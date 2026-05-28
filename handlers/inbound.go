package handlers

import (
	"OpsLB/cache"
	"OpsLB/database"
	"OpsLB/models"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

// 同步单个网关到 cfg_inbound_ips 表
func syncGatewayToPermissions(gateway models.InboundGateway) error {
	// 1. 删除该网关的旧IP记录（通过 context_info 字段标记网关ID）
	result := database.DB.Exec("DELETE FROM cfg_inbound_ips WHERE context_info = ?", fmt.Sprintf("gateway_%d", gateway.ID))
	if result.Error != nil {
		return fmt.Errorf("failed to delete old IPs: %v", result.Error)
	}
	
	// 2. 如果网关被禁用，不插入新IP
	if gateway.Status != 1 {
		return nil
	}
	
	// 3. 插入新IP（入局网关使用默认优先级 0）
	ips := strings.Split(gateway.IPs, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		
		if err := database.DB.Exec(
			"INSERT INTO cfg_inbound_ips (ip, mask, port, proto, priority, context_info) VALUES (?, 32, 0, 'any', 0, ?)",
			ip, fmt.Sprintf("gateway_%d", gateway.ID),
		).Error; err != nil {
			return fmt.Errorf("failed to insert IP %s: %v", ip, err)
		}
	}
	
	return nil
}

// 刷新OpenSIPS白名单（调用 MI API）
func reloadOpenSIPS() error {
	jsonBody := `{"jsonrpc":"2.0","method":"address_reload","id":1}`
	resp, err := http.Post("http://127.0.0.1:8889/mi", "application/json", strings.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to call MI API: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("reload failed with status: %d", resp.StatusCode)
	}
	return nil
}

// 获取所有入局网关
func GetInboundGateways(c *gin.Context) {
	var gateways []models.InboundGateway
	if err := database.DB.Find(&gateways).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取实时并发数
	for i := range gateways {
		calls, _ := cache.GetInboundCalls(gateways[i].ID)
		gateways[i].CurrentCalls = int(calls)
	}

	c.JSON(http.StatusOK, gateways)
}

// 获取单个入局网关
func GetInboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.InboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	calls, _ := cache.GetInboundCalls(gateway.ID)
	gateway.CurrentCalls = int(calls)

	c.JSON(http.StatusOK, gateway)
}

// 创建入局网关
func CreateInboundGateway(c *gin.Context) {
	var gateway models.InboundGateway
	if err := c.ShouldBindJSON(&gateway); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 IP 列表
	if gateway.IPs == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP列表不能为空"})
		return
	}

	if err := database.DB.Create(&gateway).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 同步到 cfg_inbound_ips 表
	if err := syncGatewayToPermissions(gateway); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "同步白名单失败: " + err.Error()})
		return
	}

	// 刷新 OpenSIPS 内存
	go reloadOpenSIPS()

	c.JSON(http.StatusCreated, gateway)
}

// 更新入局网关
func UpdateInboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.InboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	if err := c.ShouldBindJSON(&gateway); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 IP 列表
	if gateway.IPs == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP列表不能为空"})
		return
	}

	gateway.ID = uint(id)
	if err := database.DB.Save(&gateway).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 同步到 cfg_inbound_ips 表（先删除旧的，再插入新的）
	if err := syncGatewayToPermissions(gateway); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "同步白名单失败: " + err.Error()})
		return
	}

	// 刷新 OpenSIPS 内存
	go reloadOpenSIPS()

	c.JSON(http.StatusOK, gateway)
}

// 删除入局网关
func DeleteInboundGateway(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var gateway models.InboundGateway
	if err := database.DB.First(&gateway, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	// 硬删除（从数据库彻底删除）
	database.DB.Unscoped().Delete(&gateway)

	// 从 cfg_inbound_ips 表删除对应的IP
	database.DB.Exec("DELETE FROM cfg_inbound_ips WHERE context_info = ?", fmt.Sprintf("gateway_%d", id))

	// 刷新 OpenSIPS 内存
	go reloadOpenSIPS()

	c.JSON(http.StatusOK, gin.H{"message": "Gateway deleted successfully"})
}

// 手动刷新OpenSIPS白名单
func ReloadInboundGateways(c *gin.Context) {
	if err := reloadOpenSIPS(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Reload successful"})
}
