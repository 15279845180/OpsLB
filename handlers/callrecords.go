package handlers

import (
	"OpsLB/database"
	"OpsLB/models"
	"encoding/csv"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// isNumeric 判断是否为纯数字
func isNumeric(s string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}

// buildStatusKeywordFilter 根据中文关键字构建状态码过滤条件
func buildStatusKeywordFilter(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	
	// 内部错误关键字
	if strings.Contains(keyword, "超频") || strings.Contains(keyword, "cps") {
		return "status_code = -1"
	}
	if strings.Contains(keyword, "无可用") || strings.Contains(keyword, "no gateway") {
		return "status_code = -2"
	}
	if strings.Contains(keyword, "超并发") || strings.Contains(keyword, "concurrent") {
		return "status_code = -3"
	}
	
	// 呼叫中（status_code=0 或 NULL）
	if strings.Contains(keyword, "呼叫中") {
		return "(status_code = 0 OR status_code IS NULL)"
	}
	
	// 通话中（status_code=200 且 end_time 为空）
	if strings.Contains(keyword, "通话中") {
		return "status_code = 200 AND (end_time IS NULL OR end_time = '0000-00-00 00:00:00')"
	}
	
	// 成功（status_code=200）
	if strings.Contains(keyword, "成功") {
		return "status_code = 200"
	}
	
	// 常见 SIP 错误关键字
	if strings.Contains(keyword, "无应答") || strings.Contains(keyword, "timeout") {
		return "status_code IN (408, 480, 487)"
	}
	if strings.Contains(keyword, "用户忙") || strings.Contains(keyword, "busy") {
		return "status_code = 486"
	}
	if strings.Contains(keyword, "服务器") || strings.Contains(keyword, "server") {
		return "status_code >= 500 AND status_code < 600"
	}
	if strings.Contains(keyword, "拒绝") || strings.Contains(keyword, "decline") {
		return "status_code = 603"
	}
	
	// 默认：尝试匹配 status_code（兜底）
	if num, err := strconv.Atoi(keyword); err == nil {
		return fmt.Sprintf("status_code = %d", num)
	}
	
	return "status_code != status_code" // 永不匹配
}

// 获取呼叫记录列表
func GetCallRecords(c *gin.Context) {
	// 获取筛选参数
	callerNumber := c.Query("caller_number")
	calledNumber := c.Query("called_number")
	statusCode := c.Query("status_code")
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	
	// 获取所有话单表名
	var tableNames []string
	database.DB.Raw(`
		SELECT TABLE_NAME FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME LIKE 'cdr_call_%'
		ORDER BY TABLE_NAME DESC
	`).Scan(&tableNames)
	
	if len(tableNames) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total":     0,
			"page":      page,
			"page_size": pageSize,
			"data":      []interface{}{},
		})
		return
	}
	
	// 合并所有表的数据（最多查询最近7天的表）
	var allRecords []models.CallRecord
	maxTables := 7
	if len(tableNames) < maxTables {
		maxTables = len(tableNames)
	}
	
	for i := 0; i < maxTables; i++ {
		tableName := tableNames[i]
		query := database.DB.Table(tableName)
		
		// 应用筛选条件
		if callerNumber != "" {
			query = query.Where("caller_number LIKE ?", "%"+callerNumber+"%")
		}
		if calledNumber != "" {
			query = query.Where("called_number LIKE ?", "%"+calledNumber+"%")
		}
		if statusCode != "" {
			// 支持自定义输入查询：
			// 1. 纯数字：精确匹配status_code
			// 2. 中文关键字：根据关键字匹配状态描述
			// 3. success/failed：原有逻辑
			if statusCode == "success" {
				query = query.Where("status_code = 200")
			} else if statusCode == "failed" {
				query = query.Where("status_code != 200 AND status_code >= 0")
			} else if isNumeric(statusCode) {
				// 纯数字，精确匹配状态码
				query = query.Where("status_code = ?", statusCode)
			} else {
				// 中文关键字，匹配内部错误或常见状态描述
				query = query.Where(buildStatusKeywordFilter(statusCode))
			}
		}
		if startTime != "" {
			query = query.Where("invite_time >= ?", startTime)
		}
		if endTime != "" {
			query = query.Where("invite_time <= ?", endTime)
		}
		
		var records []models.CallRecord
		if err := query.Order("invite_time DESC").Find(&records).Error; err != nil {
			// 表可能损坏，跳过
			continue
		}
		allRecords = append(allRecords, records...)
	}
	
	// 按时间排序（降序）
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].InviteTime.After(allRecords[j].InviteTime)
	})

	// 总数
	total := int64(len(allRecords))
	
	// 分页
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > len(allRecords) {
		end = len(allRecords)
	}
	
	// 确保始终返回数组，而不是null
	paginatedRecords := make([]models.CallRecord, 0)
	if offset < len(allRecords) {
		paginatedRecords = allRecords[offset:end]
	}
	
	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      paginatedRecords,
	})
}

// 导出呼叫记录为 CSV
func ExportCallRecords(c *gin.Context) {
	// 获取筛选参数（同 GetCallRecords）
	callerNumber := c.Query("caller_number")
	calledNumber := c.Query("called_number")
	statusCode := c.Query("status_code")
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	
	// 获取所有话单表名
	var tableNames []string
	database.DB.Raw(`
		SELECT TABLE_NAME FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = DATABASE() 
		AND TABLE_NAME LIKE 'cdr_call_%'
		ORDER BY TABLE_NAME DESC
	`).Scan(&tableNames)
	
	// 确保始终返回数组
	allRecords := make([]models.CallRecord, 0)
	
	// 最多查询最近7天的表
	maxTables := 7
	if len(tableNames) < maxTables {
		maxTables = len(tableNames)
	}
	
	for i := 0; i < maxTables; i++ {
		tableName := tableNames[i]
		query := database.DB.Table(tableName)
		
		if callerNumber != "" {
			query = query.Where("caller_number LIKE ?", "%"+callerNumber+"%")
		}
		if calledNumber != "" {
			query = query.Where("called_number LIKE ?", "%"+calledNumber+"%")
		}
		if statusCode != "" {
			// 支持自定义输入查询（同 GetCallRecords）
			if statusCode == "success" {
				query = query.Where("status_code = 200")
			} else if statusCode == "failed" {
				query = query.Where("status_code != 200 AND status_code >= 0")
			} else if isNumeric(statusCode) {
				query = query.Where("status_code = ?", statusCode)
			} else {
				query = query.Where(buildStatusKeywordFilter(statusCode))
			}
		}
		if startTime != "" {
			query = query.Where("invite_time >= ?", startTime)
		}
		if endTime != "" {
			query = query.Where("invite_time <= ?", endTime)
		}
		
		var records []models.CallRecord
		if err := query.Order("invite_time DESC").Find(&records).Error; err != nil {
			continue
		}
		allRecords = append(allRecords, records...)
	}
	
	// 按时间排序（降序）
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].InviteTime.After(allRecords[j].InviteTime)
	})

	// 创建 CSV
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	
	// 写入表头
	w.Write([]string{
		"呼叫ID", "主叫号码", "被叫号码", "入局IP", "出局网关IP",
		"开始时间", "接通时间", "结束时间", "状态码", "状态描述", "通话时长(秒)",
	})
	
	// 写入数据
	for _, record := range allRecords {
		answerTime := ""
		if !record.AnswerTime.IsZero() {
			answerTime = record.AnswerTime.Format("2006-01-02 15:04:05")
		}
		endTime := ""
		if !record.EndTime.IsZero() {
			endTime = record.EndTime.Format("2006-01-02 15:04:05")
		}
		
		w.Write([]string{
			record.CallID,
			record.CallerNumber,
			record.CalledNumber,
			record.InboundIP,
			record.OutboundGatewayIP,
			record.InviteTime.Format("2006-01-02 15:04:05"),
			answerTime,
			endTime,
			strconv.Itoa(record.StatusCode),
			record.StatusText,
			strconv.Itoa(record.TalkDuration),
		})
	}
	
	// 设置响应头
	filename := fmt.Sprintf("call_records_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv")
}
