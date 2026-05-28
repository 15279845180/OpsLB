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

// 获取所有出局网关
func GetOutboundGateways(c *gin.Context) {
	var gateways []models.OutboundGateway
	if err := database.DB.Find(&gateways).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取实时并发数
	for i := range gateways {
		calls, _ := cache.GetGatewayCalls(gateways[i].ID)
		gateways[i].CurrentCalls = int(calls)
	}

	c.JSON(http.StatusOK, gateways)
}

// 获取单个出局网关
func GetOutboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.OutboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	calls, _ := cache.GetGatewayCalls(gateway.ID)
	gateway.CurrentCalls = int(calls)

	c.JSON(http.StatusOK, gateway)
}

// 创建出局网关
func CreateOutboundGateway(c *gin.Context) {
	var gateway models.OutboundGateway
	if err := c.ShouldBindJSON(&gateway); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证权重范围
	if gateway.Weight < 1 || gateway.Weight > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "权重必须在 1-100 之间"})
		return
	}

	if err := database.DB.Create(&gateway).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gateway)
}

// 更新出局网关
func UpdateOutboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.OutboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	if err := c.ShouldBindJSON(&gateway); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证权重范围
	if gateway.Weight < 1 || gateway.Weight > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "权重必须在 1-100 之间"})
		return
	}

	gateway.ID = uint(id)
	if err := database.DB.Save(&gateway).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gateway)
}

// 删除出局网关
func DeleteOutboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.OutboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	database.DB.Unscoped().Delete(&gateway)
	c.JSON(http.StatusOK, gin.H{"message": "Gateway deleted successfully"})
}

// 批量更新网关状态
func BatchUpdateGatewayStatus(c *gin.Context) {
	var req struct {
		IDs    []uint `json:"ids" binding:"required"`
		Status int    `json:"status" binding:"required"` // 1启用 0禁用 2暂停
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.OutboundGateway{}).
		Where("id IN ?", req.IDs).
		Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated successfully"})
}

// 过滤可用网关（检查并发和CPS）- 缓存版本
// 返回值: (可用网关列表, 错误类型)
// 错误类型: "", "cps", "concurrent", "all_cps", "all_concurrent"
func filterAvailableGatewaysFromCache(gateways []*cache.OutboundGatewayInfo) ([]*cache.OutboundGatewayInfo, string) {
	var available []*cache.OutboundGatewayInfo
	cpsBlocked := 0
	concurrentBlocked := 0

	for _, gw := range gateways {
		// 检查并发
		calls, _ := cache.GetGatewayCalls(gw.ID)
		if int(calls) >= gw.MaxConcurrent {
			concurrentBlocked++
			continue
		}

		// 检查CPS（只有设置了才检查）
		if gw.MaxCPS > 0 {
			ok, _ := cache.CheckAndIncCPS(gw.ID, gw.MaxCPS)
			if !ok {
				cpsBlocked++
				continue
			}
		}

		available = append(available, gw)
	}

	// 判断错误类型
	if len(available) == 0 {
		if cpsBlocked > 0 && concurrentBlocked == 0 {
			return available, "all_cps" // 全部被CPS限制
		}
		if concurrentBlocked > 0 && cpsBlocked == 0 {
			return available, "all_concurrent" // 全部被并发限制
		}
		if cpsBlocked > 0 && concurrentBlocked > 0 {
			return available, "mixed" // 混合限制
		}
	}

	return available, ""
}

// 按权重选择网关（加权轮询算法）- 缓存版本
func selectByWeightFromCache(gateways []*cache.OutboundGatewayInfo) *cache.OutboundGatewayInfo {
	if len(gateways) == 1 {
		return gateways[0]
	}

	// 获取当前优先级的轮询索引
	priority := gateways[0].Priority
	currentIndex, _ := cache.GetGatewayRRIndex(priority)

	// 计算总权重
	totalWeight := 0
	for _, gw := range gateways {
		totalWeight += gw.Weight
	}

	// 加权轮询：根据权重分配
	// 例如：网关A权重=3，网关B权重=2
	// 则序列为：A, A, A, B, B, A, A, A, B, B...
	targetIndex := currentIndex % int64(totalWeight)

	// 根据targetIndex找到对应的网关
	var cumulativeWeight int64
	for _, gw := range gateways {
		cumulativeWeight += int64(gw.Weight)
		if targetIndex < cumulativeWeight {
			// 递增索引，下次选择下一个
			cache.IncrGatewayRRIndex(priority)
			return gw
		}
	}

	// 兜底：重置索引并返回第一个
	cache.ResetGatewayRRIndex(priority)
	return gateways[0]
}

// ============================================================
// 统一事件接口 - OpenSIPS 只需调用这一个接口上报所有事件
// 事件类型:
//
//	invite  - INVITE 进来，创建话单 + 增加并发
//	answer  - 200 OK，更新话单接通时间
//	bye     - BYE 挂断，结束话单 + 释放并发
//	cancel  - CANCEL 取消，释放并发 + 更新话单
//	failed  - 失败（4xx/5xx/6xx），更新话单状态 + 释放并发
//
// ============================================================
func ReportSIPEvent(c *gin.Context) {
	var req struct {
		Type       string `json:"type" binding:"required"` // invite/answer/bye/cancel/failed
		CallID     string `json:"call_id" binding:"required"`
		InboundIP  string `json:"inbound_ip"`    // invite 时传入
		CallerNum  string `json:"caller_number"` // invite 时传入
		CalledNum  string `json:"called_number"` // invite 时传入
		GatewayID  uint   `json:"gateway_id"`    // 出局网关ID
		InboundID  uint   `json:"inbound_id"`    // 入局网关ID
		StatusCode int    `json:"status_code"`   // answer/failed 时传入
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.Type {
	case "invite":
		// 【申请-授权模式】话单创建已迁移到 /api/call/allocate/inbound
		// 此处不再重复创建话单，仅记录日志
		// 注意：如果OpenSIPS同时调用两个接口，会导致话单重复创建
		log.Printf("[EVENT-invite] call_id=%s inbound=%d (call record managed by allocate/inbound)", req.CallID, req.InboundID)

	case "answer":
		// 200 OK：更新话单接通时间
		updates := map[string]interface{}{
			"status_code": 200,
			"answer_time": time.Now(),
		}
		result := database.DB.Model(&models.CallRecord{}).Where("call_id = ?", req.CallID).Updates(updates)
		if result.RowsAffected > 0 {
			log.Printf("[EVENT-answer] ✅ Updated: call_id=%s, status=200", req.CallID)
		} else {
			log.Printf("[EVENT-answer] ⚠️  Record not found: call_id=%s", req.CallID)
		}

	case "bye":
		// 【申请-授权模式】BYE只更新话单，并发释放由 /api/call/release 统一处理
		now := time.Now()
		var record models.CallRecord
		if err := database.DB.Where("call_id = ?", req.CallID).First(&record).Error; err == nil {
			log.Printf("[EVENT-bye] 📝 Record found: call_id=%s, status_code=%d, answer_time=%v, end_time=%v",
				req.CallID, record.StatusCode, record.AnswerTime, record.EndTime)

			if record.EndTime.IsZero() {
				updates := map[string]interface{}{"end_time": now, "talk_duration": 0}
				if !record.AnswerTime.IsZero() {
					updates["talk_duration"] = int(now.Sub(record.AnswerTime).Seconds())
				}
				database.DB.Model(&models.CallRecord{}).Where("call_id = ?", req.CallID).Updates(updates)
				log.Printf("[EVENT-bye] ✅ Record ended: talk_duration=%ds", updates["talk_duration"])
			} else {
				log.Printf("[EVENT-bye] ⚠️  Already ended, skipped")
			}
		} else {
			log.Printf("[EVENT-bye] ❌ Record not found: call_id=%s", req.CallID)
		}

		// 【已移除】出局/入局并发释放逻辑 → 由 /api/call/release 统一处理
		log.Printf("[EVENT-bye] call_id=%s gw=%d inbound=%d (concurrent managed by /api/call/release)", req.CallID, req.GatewayID, req.InboundID)

	case "cancel":
		// 【申请-授权模式】CANCEL只更新话单，并发释放由 /api/call/release 统一处理
		log.Printf("[EVENT-cancel] 📥 Received: call_id=%s, gw=%d, inbound=%d", req.CallID, req.GatewayID, req.InboundID)

		// 【已移除】出局/入局并发释放逻辑 → 由 /api/call/release 统一处理

		// 更新话单状态为487（Request Terminated）
		now := time.Now()
		result := database.DB.Model(&models.CallRecord{}).Where("call_id = ?", req.CallID).Updates(map[string]interface{}{
			"status_code": 487,
			"end_time":    now,
		})
		if result.RowsAffected > 0 {
			log.Printf("[EVENT-cancel] ✅ Record updated: status=487")
		} else {
			log.Printf("[EVENT-cancel] ⚠️  Record not found, status not updated")
		}

		log.Printf("[EVENT-cancel] call_id=%s gw=%d inbound=%d (concurrent managed by /api/call/release)", req.CallID, req.GatewayID, req.InboundID)

	case "failed":
		// 【申请-授权模式】failed只更新话单，并发释放由 /api/call/release 统一处理
		now := time.Now()
		log.Printf("[EVENT-failed] 📥 Received: call_id=%s, status=%d, gw=%d, inbound=%d",
			req.CallID, req.StatusCode, req.GatewayID, req.InboundID)

		updates := map[string]interface{}{
			"status_code": req.StatusCode,
			"end_time":    now,
		}
		result := database.DB.Model(&models.CallRecord{}).Where("call_id = ?", req.CallID).Updates(updates)
		if result.RowsAffected > 0 {
			log.Printf("[EVENT-failed] ✅ Record updated: status=%d", req.StatusCode)
		} else {
			log.Printf("[EVENT-failed] ⚠️  Record not found, status not updated: call_id=%s", req.CallID)
		}

		// 【已移除】出局/入局并发释放逻辑 → 由 /api/call/release 统一处理
		log.Printf("[EVENT-failed] call_id=%s status=%d gw=%d (concurrent managed by /api/call/release)", req.CallID, req.StatusCode, req.GatewayID)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown event type: " + req.Type})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
