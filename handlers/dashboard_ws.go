package handlers

import (
	"OpsLB/cache"
	"OpsLB/database"
	"OpsLB/models"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/golang-jwt/jwt/v5"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebSocket升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 生产环境需要验证Origin
	},
}

// 客户端管理
type ClientManager struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan DashboardMessage
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.RWMutex
}

type DashboardMessage struct {
	Type         string       `json:"type"`
	InboundCalls int          `json:"inbound_calls,omitempty"`
	OutboundCalls int         `json:"outbound_calls,omitempty"`
	TotalCalls   int64        `json:"total_calls,omitempty"`
	SuccessCalls int64        `json:"success_calls,omitempty"`
	AnswerRate   float64      `json:"answer_rate,omitempty"`
	FailedCalls  []FailedCall `json:"failed_calls,omitempty"`
}

type FailedCall struct {
	OutboundGatewayIP string `json:"outbound_gateway_ip"`
	StatusCode        int    `json:"status_code"`
	StatusText        string `json:"status_text"`
	Count             int    `json:"count"`
}

var manager = &ClientManager{
	clients:    make(map[*websocket.Conn]bool),
	broadcast:  make(chan DashboardMessage),
	register:   make(chan *websocket.Conn),
	unregister: make(chan *websocket.Conn),
}

// 启动消息处理器
func init() {
	go manager.handleMessages()
}

// 处理客户端注册/注销和消息广播
func (m *ClientManager) handleMessages() {
	for {
		select {
		case conn := <-m.register:
			m.mutex.Lock()
			m.clients[conn] = true
			m.mutex.Unlock()
			log.Printf("[WebSocket] 客户端连接, 当前连接数: %d", len(m.clients))

		case conn := <-m.unregister:
			m.mutex.Lock()
			if _, ok := m.clients[conn]; ok {
				conn.Close()
				delete(m.clients, conn)
				log.Printf("[WebSocket] 客户端断开, 当前连接数: %d", len(m.clients))
			}
			m.mutex.Unlock()

		case message := <-m.broadcast:
			m.mutex.RLock()
			for client := range m.clients {
				err := client.WriteJSON(message)
				if err != nil {
					log.Printf("[WebSocket] 发送消息失败: %v", err)
					client.Close()
					delete(m.clients, client)
				}
			}
			m.mutex.RUnlock()
		}
	}
}

// WebSocket连接处理
func DashboardWebSocket(c *gin.Context) {
	// 验证token(从URL参数或Header获取)
	tokenString := c.Query("token")
	if tokenString == "" {
		// 尝试从Header获取
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}
	}
	
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
		return
	}
	
	// 验证JWT
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(c.GetString("config")), nil
	})
	
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}
	
	// 升级HTTP到WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v", err)
		return
	}

	// 注册客户端
	manager.register <- conn

	// 发送初始数据
	go sendInitialData(conn)

	// 保持连接,接收心跳
	go func() {
		defer func() {
			manager.unregister <- conn
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("[WebSocket] 连接错误: %v", err)
				}
				break
			}
		}
	}()
}

// 发送初始数据
func sendInitialData(conn *websocket.Conn) {
	msg := DashboardMessage{Type: "initial"}

	// 获取并发数
	msg.InboundCalls = getTotalInboundCalls()
	msg.OutboundCalls = getTotalOutboundCalls()

	// 获取今日统计
	today := time.Now().Truncate(24 * time.Hour)
	database.DB.Model(&models.CallRecord{}).
		Where("invite_time >= ?", today).
		Count(&msg.TotalCalls)

	database.DB.Model(&models.CallRecord{}).
		Where("invite_time >= ? AND status_code = ?", today, 200).
		Count(&msg.SuccessCalls)

	if msg.TotalCalls > 0 {
		msg.AnswerRate = float64(msg.SuccessCalls) / float64(msg.TotalCalls) * 100
	}

	conn.WriteJSON(msg)
}

// 获取总入局并发
func getTotalInboundCalls() int {
	var total int
	inboundGateways := make([]models.InboundGateway, 0)
	database.DB.Where("status = ?", 1).Find(&inboundGateways)

	for _, gw := range inboundGateways {
		calls, _ := cache.GetInboundCalls(gw.ID)
		total += int(calls)
	}
	return total
}

// 获取总出局并发
func getTotalOutboundCalls() int {
	var total int
	outboundGateways := make([]models.OutboundGateway, 0)
	database.DB.Where("status = ?", 1).Find(&outboundGateways)

	for _, gw := range outboundGateways {
		calls, _ := cache.GetGatewayCalls(gw.ID)
		total += int(calls)
	}
	return total
}

// 启动定时推送任务
func StartDashboardPusher() {
	// 每2秒推送实时并发数（仅当存在客户端连接时）
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// 无客户端时跳过查询,避免空转浪费资源
			manager.mutex.RLock()
			clientCount := len(manager.clients)
			manager.mutex.RUnlock()
			if clientCount == 0 {
				continue
			}

			msg := DashboardMessage{
				Type:          "realtime",
				InboundCalls:  getTotalInboundCalls(),
				OutboundCalls: getTotalOutboundCalls(),
			}
			manager.broadcast <- msg
		}
	}()

	// 每60秒推送今日统计（仅当存在客户端连接时）
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			manager.mutex.RLock()
			clientCount := len(manager.clients)
			manager.mutex.RUnlock()
			if clientCount == 0 {
				continue
			}

			msg := DashboardMessage{Type: "stats"}

			today := time.Now().Truncate(24 * time.Hour)
			database.DB.Model(&models.CallRecord{}).
				Where("invite_time >= ?", today).
				Count(&msg.TotalCalls)

			database.DB.Model(&models.CallRecord{}).
				Where("invite_time >= ? AND status_code = ?", today, 200).
				Count(&msg.SuccessCalls)

			if msg.TotalCalls > 0 {
				msg.AnswerRate = float64(msg.SuccessCalls) / float64(msg.TotalCalls) * 100
			}

			manager.broadcast <- msg
		}
	}()

	log.Println("✅ 仪表盘WebSocket推送服务已启动")
}

// 获取失败呼叫TOP5
func getFailedCallsTop() []FailedCall {
	failedCalls := make([]FailedCall, 0)
	today := time.Now().Truncate(24 * time.Hour)
	todayTableName := fmt.Sprintf("cdr_call_%s", time.Now().Format("20060102"))

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

	return failedCalls
}
