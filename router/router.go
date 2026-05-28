package router

import (
	"OpsLB/config"
	"OpsLB/handlers"
	"OpsLB/middleware"
	"github.com/gin-gonic/gin"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	// 静态文件服务
	r.StaticFile("/", "./static/index.html")
	r.Static("/css", "./static/css")
	r.Static("/js", "./static/js")
	r.Static("/pages", "./static/pages")

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 注入配置
	r.Use(func(c *gin.Context) {
		c.Set("config", cfg.JWT.Secret)
		c.Next()
	})

	// 公开API
	public := r.Group("/api")
	{
		public.POST("/login", handlers.Login)
	}

	// 需要认证的API
	authorized := r.Group("/api")
	authorized.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
	{
		// 用户相关
		authorized.POST("/change-password", handlers.ChangePassword)

		// 入局网关管理
		authorized.GET("/inbound", handlers.GetInboundGateways)
		authorized.GET("/inbound/:id", handlers.GetInboundGateway)
		authorized.POST("/inbound", handlers.CreateInboundGateway)
		authorized.PUT("/inbound/:id", handlers.UpdateInboundGateway)
		authorized.DELETE("/inbound/:id", handlers.DeleteInboundGateway)
		authorized.POST("/inbound/reload", handlers.ReloadInboundGateways)

		// 出局网关管理
		authorized.GET("/outbound", handlers.GetOutboundGateways)
		authorized.GET("/outbound/:id", handlers.GetOutboundGateway)
		authorized.POST("/outbound", handlers.CreateOutboundGateway)
		authorized.PUT("/outbound/:id", handlers.UpdateOutboundGateway)
		authorized.DELETE("/outbound/:id", handlers.DeleteOutboundGateway)
		authorized.POST("/outbound/batch-status", handlers.BatchUpdateGatewayStatus)

		// 呼叫记录
		authorized.GET("/call-records", handlers.GetCallRecords)
		authorized.GET("/call-records/export", handlers.ExportCallRecords)

		// 系统设置
		authorized.GET("/settings", handlers.GetSystemConfig)
		authorized.PUT("/settings", handlers.UpdateSystemConfig)

		// 仪表盘统计
		authorized.GET("/dashboard/stats", handlers.GetDashboardStats)
	}
	
	// WebSocket不需要JWT中间件(自己在handler里验证)
	r.GET("/api/dashboard/ws", handlers.DashboardWebSocket)

	// OpenSIPS内部调用API（不需要JWT认证，但需要IP白名单保护）

	// 申请-授权模式接口（V2.5 接口合并）
	call := r.Group("/api/call")
	{
		call.POST("/allocate", handlers.Allocate)   // 一体化分配
		call.POST("/release", handlers.ReleaseCall)  // 并发释放
	}

	// 传统事件上报接口（话单状态更新）
	route := r.Group("/api/route")
	{
		route.POST("/event", handlers.ReportSIPEvent)
	}

	return r
}
