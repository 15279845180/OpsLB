package cache

import (
	"OpsLB/database"
	"OpsLB/models"
	"log"
	"strings"
	"sync"
	"time"
)

// GatewayCache 网关内存缓存结构
type GatewayCache struct {
	mu sync.RWMutex

	// 入局网关缓存：IP -> 网关信息
	inboundByIP map[string]*InboundGatewayInfo

	// 出局网关缓存：按优先级分组
	outboundByPriority map[int][]*OutboundGatewayInfo
	outboundPriorities []int

	// 出局网关ID索引：ID -> 网关信息（用于AllocateOutbound快速查询）
	outboundByID map[uint]*OutboundGatewayInfo

	// 缓存更新时间
	lastUpdated time.Time
}

// InboundGatewayInfo 入局网关缓存信息
type InboundGatewayInfo struct {
	ID            uint
	Name          string
	MaxConcurrent int
	IPs           map[string]bool // 精确匹配IP集合
}

// OutboundGatewayInfo 出局网关缓存信息
type OutboundGatewayInfo struct {
	ID            uint
	Name          string
	IP            string
	Port          int
	Protocol      string
	Priority      int
	Weight        int
	MaxConcurrent int
	MaxCPS        int
}

var gatewayCache = &GatewayCache{
	inboundByIP:      make(map[string]*InboundGatewayInfo),
	outboundByPriority: make(map[int][]*OutboundGatewayInfo),
	outboundByID:     make(map[uint]*OutboundGatewayInfo),
}

// InitGatewayCache 初始化网关缓存（启动时调用）
func InitGatewayCache() error {
	log.Println("[GATEWAY-CACHE] Initializing gateway cache...")
	if err := refreshGatewayCache(); err != nil {
		return err
	}

	// 启动定时刷新任务（每30秒刷新一次）
	go startCacheRefreshJob()

	log.Println("[GATEWAY-CACHE] Gateway cache initialized successfully")
	return nil
}

// refreshGatewayCache 刷新网关缓存
func refreshGatewayCache() error {
	gatewayCache.mu.Lock()
	defer gatewayCache.mu.Unlock()

	// 1. 刷新入局网关
	inboundGateways := make(map[string]*InboundGatewayInfo)
	var inbounds []models.InboundGateway
	if err := database.DB.Where("status = ?", 1).Find(&inbounds).Error; err != nil {
		return err
	}

	for _, gw := range inbounds {
		info := &InboundGatewayInfo{
			ID:            gw.ID,
			Name:          gw.Name,
			MaxConcurrent: gw.MaxConcurrent,
			IPs:           make(map[string]bool),
		}

		// 解析IP列表（精确匹配）
		ips := strings.Split(gw.IPs, "\n")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				info.IPs[ip] = true
			}
		}

		// 建立 IP -> 网关 映射
		for ip := range info.IPs {
			inboundGateways[ip] = info
		}
	}

	gatewayCache.inboundByIP = inboundGateways

	// 2. 刷新出局网关
	outboundByPriority := make(map[int][]*OutboundGatewayInfo)
	var priorities []int
	var outbounds []models.OutboundGateway
	if err := database.DB.Where("status = ?", 1).Order("priority ASC").Find(&outbounds).Error; err != nil {
		return err
	}

	for _, gw := range outbounds {
		info := &OutboundGatewayInfo{
			ID:            gw.ID,
			Name:          gw.Name,
			IP:            gw.IP,
			Port:          gw.Port,
			Protocol:      gw.Protocol,
			Priority:      gw.Priority,
			Weight:        gw.Weight,
			MaxConcurrent: gw.MaxConcurrent,
			MaxCPS:        gw.MaxCPS,
		}

		outboundByPriority[gw.Priority] = append(outboundByPriority[gw.Priority], info)
		gatewayCache.outboundByID[gw.ID] = info  // 建立ID索引

		// 收集优先级列表
		found := false
		for _, p := range priorities {
			if p == gw.Priority {
				found = true
				break
			}
		}
		if !found {
			priorities = append(priorities, gw.Priority)
		}
	}

	gatewayCache.outboundByPriority = outboundByPriority
	gatewayCache.outboundPriorities = priorities
	gatewayCache.lastUpdated = time.Now()

	log.Printf("[GATEWAY-CACHE] Cache refreshed: %d inbounds, %d outbounds, %d priorities",
		len(inboundGateways), len(outbounds), len(priorities))

	return nil
}

// startCacheRefreshJob 定时刷新缓存任务
func startCacheRefreshJob() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := refreshGatewayCache(); err != nil {
			log.Printf("[GATEWAY-CACHE] Refresh error: %v", err)
		}
	}
}

// GetInboundGatewayByIP 从内存缓存获取入局网关（微秒级）
func GetInboundGatewayByIP(ip string) (*InboundGatewayInfo, bool) {
	gatewayCache.mu.RLock()
	defer gatewayCache.mu.RUnlock()

	gw, exists := gatewayCache.inboundByIP[ip]
	return gw, exists
}

// GetOutboundGateways 从内存缓存获取出局网关（按优先级分组）
func GetOutboundGateways() (map[int][]*OutboundGatewayInfo, []int) {
	gatewayCache.mu.RLock()
	defer gatewayCache.mu.RUnlock()

	// 返回副本，避免外部修改
	priorityCopy := make(map[int][]*OutboundGatewayInfo)
	for p, gws := range gatewayCache.outboundByPriority {
		priorityCopy[p] = gws
	}

	return priorityCopy, gatewayCache.outboundPriorities
}

// GetOutboundGatewayByID 从内存缓存根据ID获取出局网关（微秒级）
func GetOutboundGatewayByID(id uint) (*OutboundGatewayInfo, bool) {
	gatewayCache.mu.RLock()
	defer gatewayCache.mu.RUnlock()

	gw, exists := gatewayCache.outboundByID[id]
	return gw, exists
}

// GetCacheStats 获取缓存统计信息
func GetCacheStats() map[string]interface{} {
	gatewayCache.mu.RLock()
	defer gatewayCache.mu.RUnlock()

	inboundCount := len(gatewayCache.inboundByIP)
	outboundCount := 0
	for _, gws := range gatewayCache.outboundByPriority {
		outboundCount += len(gws)
	}

	return map[string]interface{}{
		"inbound_count":  inboundCount,
		"outbound_count": outboundCount,
		"last_updated":   gatewayCache.lastUpdated,
	}
}
