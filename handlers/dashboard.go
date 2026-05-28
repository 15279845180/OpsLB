package handlers

import (
	"OpsLB/cache"
	"OpsLB/database"
	"OpsLB/models"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// 获取仪表盘统计数据
func GetDashboardStats(c *gin.Context) {
	// 今日开始时间
	today := time.Now().Truncate(24 * time.Hour)
	
	// 今日总呼叫数
	var totalCalls int64
	database.DB.Model(&models.CallRecord{}).
		Where("invite_time >= ?", today).
		Count(&totalCalls)
	
	// 今日接通数
	var successCalls int64
	database.DB.Model(&models.CallRecord{}).
		Where("invite_time >= ? AND status_code = ?", today, 200).
		Count(&successCalls)
	
	// 接通率
	var answerRate float64
	if totalCalls > 0 {
		answerRate = float64(successCalls) / float64(totalCalls) * 100
	}
	
	// 入局网关并发统计
	inboundGateways := make([]models.InboundGateway, 0)
	database.DB.Where("status = ?", 1).Find(&inboundGateways)
	
	var totalInboundCalls int
	for i := range inboundGateways {
		calls, _ := cache.GetInboundCalls(inboundGateways[i].ID)
		inboundGateways[i].CurrentCalls = int(calls)
		totalInboundCalls += int(calls)
	}
	
	// 出局网关并发统计
	outboundGateways := make([]models.OutboundGateway, 0)
	database.DB.Where("status = ?", 1).Find(&outboundGateways)
	
	var totalOutboundCalls int
	for i := range outboundGateways {
		calls, _ := cache.GetGatewayCalls(outboundGateways[i].ID)
		outboundGateways[i].CurrentCalls = int(calls)
		totalOutboundCalls += int(calls)
	}
	
	// 失败呼叫 TOP 5 (只查今天的表,避免性能问题)
	type FailedCall struct {
		OutboundGatewayIP string
		StatusCode        int
		StatusText        string
		Count             int
	}
	
	failedCalls := make([]FailedCall, 0)
	
	// 只查询今天的话单表
	todayTableName := fmt.Sprintf("cdr_call_%s", time.Now().Format("20060102"))
	
	// 检查表是否存在
	var tableCount int64
	database.DB.Raw(
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		todayTableName,
	).Scan(&tableCount)
	
	if tableCount > 0 {
		database.DB.Table(todayTableName).
			Select("outbound_gateway_ip, status_code, status_text, COUNT(*) as count").
			Where("invite_time >= ? AND status_code >= 400", today).
			Group("outbound_gateway_ip, status_code, status_text").
			Order("count DESC").
			Limit(5).
			Scan(&failedCalls)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"total_calls":        totalCalls,
		"success_calls":      successCalls,
		"answer_rate":        answerRate,
		"total_inbound_calls":  totalInboundCalls,
		"total_outbound_calls": totalOutboundCalls,
		"inbound_gateways":   inboundGateways,
		"outbound_gateways":  outboundGateways,
		"failed_calls_top":   failedCalls,
	})
}
