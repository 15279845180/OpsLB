package handlers

import (
	"OpsLB/cache"
	"OpsLB/database"
	"OpsLB/models"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ==================== 一体化分配接口（V2.5 接口合并） ====================

// 合并请求：一次请求完成 入局授权 + 选路 + 出局授权
type AllocateRequest struct {
	CallID    string `json:"call_id" binding:"required"`
	Caller    string `json:"caller"`
	Called    string `json:"called"`
	InboundIP string `json:"inbound_ip" binding:"required"`
}

// 合并响应
type AllocateResponse struct {
	Allowed           bool   `json:"allowed"`
	InboundID         uint   `json:"inbound_id"`
	GatewayID         uint   `json:"gateway_id"`
	GatewayIP         string `json:"gateway_ip"`
	GatewayPort       string `json:"gateway_port"`
	Protocol          string `json:"protocol"`
	ErrorCode         int    `json:"error_code"`
	Reason            string `json:"reason,omitempty"`
	CurrentConcurrent int64  `json:"current_concurrent"`
}

// Allocate 一体化分配：入局授权 + 选路 + 出局授权（一次HTTP完成三件事）
func Allocate(c *gin.Context) {
	var req AllocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. 从内存缓存查找入局网关（微秒级）
	inboundGW, exists := cache.GetInboundGatewayByIP(req.InboundIP)
	if !exists {
		log.Printf("[ALLOCATE] IP not authorized: %s, call_id=%s", req.InboundIP, req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -4, Reason: "IP not authorized"})
		return
	}

	// 2. Redis 原子申请入局并发
	inboundStatus, _, err := cache.AllocateInboundCall(req.CallID, inboundGW.ID, inboundGW.MaxConcurrent)
	if err != nil {
		log.Printf("[ALLOCATE] Redis inbound error: %v, call_id=%s", err, req.CallID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error"})
		return
	}

	isNewCall := inboundStatus == "SUCCESS"

	switch inboundStatus {
	case "FULL":
		log.Printf("[ALLOCATE] Inbound FULL: id=%d, max=%d, call_id=%s", inboundGW.ID, inboundGW.MaxConcurrent, req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -3, Reason: "Inbound concurrent full"})
		return
	case "REJECT":
		log.Printf("[ALLOCATE] Ghost INVITE rejected: call_id=%s", req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -5, Reason: "Call already released"})
		return
	}
	// SUCCESS 或 DUPLICATE 都继续走选路

	// 3. 从内存缓存选路（微秒级，Go内做优先级>权重>CPS筛选）
	priorityGroups, priorities := cache.GetOutboundGateways()
	if len(priorities) == 0 {
		// 选路失败 → 回滚入局并发
		cache.ReleaseCall(req.CallID)
		log.Printf("[ALLOCATE] No gateways available, call_id=%s", req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -2, Reason: "No available gateways"})
		return
	}

	var selected *cache.OutboundGatewayInfo
	var lastErrorType string
	for _, priority := range priorities {
		group := priorityGroups[priority]
		availableGateways, errType := filterAvailableGatewaysFromCache(group)
		if len(availableGateways) > 0 {
			selected = selectByWeightFromCache(availableGateways)
			break
		}
		if errType != "" {
			lastErrorType = errType
		}
	}

	if selected == nil {
		// 选路失败 → 回滚入局并发
		cache.ReleaseCall(req.CallID)
		errCode := -3
		if lastErrorType == "all_cps" {
			errCode = -1
		}
		log.Printf("[ALLOCATE] No gateway available (err=%s), call_id=%s", lastErrorType, req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: errCode, Reason: "All gateways at capacity"})
		return
	}

	// 4. Redis 原子申请出局并发
	outboundStatus, outboundCurrent, err := cache.AllocateOutboundCall(req.CallID, selected.ID, selected.MaxConcurrent)
	if err != nil {
		// 出局分配异常 → 回滚入局并发
		cache.ReleaseCall(req.CallID)
		log.Printf("[ALLOCATE] Redis outbound error: %v, call_id=%s", err, req.CallID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error"})
		return
	}

	switch outboundStatus {
	case "NOT_FOUND", "INVALID":
		// 入局已分配但call记录异常 → 回滚
		cache.ReleaseCall(req.CallID)
		log.Printf("[ALLOCATE] Outbound status=%s, call_id=%s", outboundStatus, req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -5, Reason: "Call record mismatch"})
		return
	case "FULL":
		// 出局满 → 回滚入局并发
		cache.ReleaseCall(req.CallID)
		log.Printf("[ALLOCATE] Outbound FULL: id=%d, max=%d, call_id=%s", selected.ID, selected.MaxConcurrent, req.CallID)
		c.JSON(http.StatusOK, AllocateResponse{Allowed: false, ErrorCode: -3, Reason: "Outbound concurrent full"})
		return
	}

	// 5. 新通话：异步创建话单
	if isNewCall {
		record := models.CallRecord{
			CallID:       req.CallID,
			InboundIP:    req.InboundIP,
			CallerNumber: req.Caller,
			CalledNumber: req.Called,
			InviteTime:   time.Now(),
		}
		cache.EnqueueCallRecord(record)
	}

	log.Printf("[ALLOCATE] SUCCESS call_id=%s inbound=%d gateway=%d(%s:%s) duplicate=%v",
		req.CallID, inboundGW.ID, selected.ID, selected.IP, strconv.Itoa(selected.Port), !isNewCall)

	c.JSON(http.StatusOK, AllocateResponse{
		Allowed:           true,
		InboundID:         inboundGW.ID,
		GatewayID:         selected.ID,
		GatewayIP:         selected.IP,
		GatewayPort:       strconv.Itoa(selected.Port),
		Protocol:          selected.Protocol,
		CurrentConcurrent: outboundCurrent,
	})
}

// ==================== 释放并发 ====================

type ReleaseRequest struct {
	CallID     string `json:"call_id" binding:"required"`
	Reason     string `json:"reason"`      // "bye" | "failed" | "cancel"
	StatusCode int    `json:"status_code"` // 仅在 reason="failed" 时传入
}

type ReleaseResponse struct {
	Released         bool `json:"released"`
	InboundReleased  bool `json:"inbound_released"`
	OutboundReleased bool `json:"outbound_released"`
}

func ReleaseCall(c *gin.Context) {
	var req ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. Redis 释放并发
	released, inboundReleased, outboundReleased, err := cache.ReleaseCall(req.CallID)
	if err != nil {
		log.Printf("[RELEASE] Redis error: %v, call_id=%s", err, req.CallID)
	}

	if !released {
		// Redis中没有记录，可能是Redis key已过期
		// 由于并发计数器可能也不存在了，无法准确释放
		// 仅记录日志，不再查询数据库兜底（高并发下避免数据库雪崩）
		log.Printf("[RELEASE] Redis record not found: call_id=%s (key expired or already released)", req.CallID)
	}

	// 2. 更新话单状态（异步操作，不阻塞主流程）
	if req.Reason != "" {
		// 异步更新话单状态
		go updateCallRecordAsync(req.CallID, req.Reason, req.StatusCode)
	}

	// 3. 更新出局网关IP（如果release时有gateway_id信息）
	if req.Reason == "answer" {
		// answer事件不在这里处理，保留在event接口
	}

	log.Printf("[RELEASE] call_id=%s reason=%s released=%v inbound=%v outbound=%v",
		req.CallID, req.Reason, released, inboundReleased, outboundReleased)

	c.JSON(http.StatusOK, ReleaseResponse{
		Released:         released,
		InboundReleased:  inboundReleased,
		OutboundReleased: outboundReleased,
	})
}

// updateCallRecordAsync 异步更新话单状态（不阻塞主流程）
func updateCallRecordAsync(callID, reason string, statusCode int) {
	updates := map[string]interface{}{
		"end_time": time.Now(),
	}

	// 确定状态码
	if statusCode > 0 {
		updates["status_code"] = statusCode
	} else {
		switch reason {
		case "bye":
			updates["status_code"] = 200
		case "cancel":
			updates["status_code"] = 487
		case "failed":
			updates["status_code"] = 408
		}
	}

	// 如果有answer_time，计算通话时长
	var record models.CallRecord
	if err := database.DB.Where("call_id = ?", callID).First(&record).Error; err == nil {
		if !record.AnswerTime.IsZero() {
			updates["talk_duration"] = int(time.Now().Sub(record.AnswerTime).Seconds())
		}
	}

	result := database.DB.Model(&models.CallRecord{}).Where("call_id = ?", callID).Updates(updates)
	if result.RowsAffected > 0 {
		log.Printf("[RELEASE-ASYNC] Record updated: call_id=%s, reason=%s, status=%v",
			callID, reason, updates["status_code"])
	}
}
