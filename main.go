package main

import (
	"OpsLB/cache"
	"OpsLB/config"
	"OpsLB/database"
	"OpsLB/handlers"
	"OpsLB/models"
	"OpsLB/router"
	"log"
	"time"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()

	// 初始化数据库
	if err := database.InitDB(&cfg.MySQL); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 初始化Redis
	if err := cache.InitRedis(&cfg.Redis); err != nil {
		log.Fatalf("Failed to initialize redis: %v", err)
	}

	// 初始化网关内存缓存（启动时预热）
	if err := cache.InitGatewayCache(); err != nil {
		log.Fatalf("Failed to initialize gateway cache: %v", err)
	}

	// 初始化话单异步写盘队列
	cache.InitCallRecordQueue()

	// 启动时确保今天的话单表存在
	if err := models.EnsureTodayTableExists(database.DB); err != nil {
		log.Printf("⚠️  检查话单表失败: %v", err)
	} else {
		log.Println("✅ 话单表检查完成")
	}

	// 启动定时任务：每天23:50预创建明天的话单表
	go startPreCreateTableJob()

	// 启动WebSocket实时推送服务
	go handlers.StartDashboardPusher()

	// 启动定时兜底扫描任务：每60秒检测超时未释放的通话
	go startAutoReleaseTask()

	// 设置路由
	r := router.SetupRouter(cfg)

	// 启动服务
	addr := ":" + cfg.Server.Port
	log.Printf("Server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// startAutoReleaseTask 定时扫描超时通话并自动释放
func startAutoReleaseTask() {
	log.Println("[AUTO-RELEASE] Starting timeout call scanner (interval=60s, timeout=300s)")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		callIDs, err := cache.ScanTimeoutCalls(300 * time.Second)
		if err != nil {
			log.Printf("[AUTO-RELEASE] Scan error: %v", err)
			continue
		}

		if len(callIDs) == 0 {
			continue
		}

		log.Printf("[AUTO-RELEASE] Found %d timeout calls, releasing...", len(callIDs))
		for _, callID := range callIDs {
			released, inboundReleased, outboundReleased, err := cache.ReleaseCall(callID)
			if err != nil {
				log.Printf("[AUTO-RELEASE] Release error: call_id=%s, err=%v", callID, err)
				continue
			}
			if released {
				log.Printf("[AUTO-RELEASE] Auto-released timeout call: call_id=%s, inbound=%v, outbound=%v",
					callID, inboundReleased, outboundReleased)
			}
		}
	}
}

// startPreCreateTableJob 定时预创建话单表任务
func startPreCreateTableJob() {
	for {
		// 计算下一个23:50的时间
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 23, 50, 0, 0, now.Location())
		
		// 如果今天已经过了23:50，就设置为明天
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		
		// 等待到23:50
		duration := next.Sub(now)
		log.Printf("⏰ 下次预创建话单表时间: %s (等待 %v)", next.Format("2006-01-02 15:04:05"), duration)
		
		time.Sleep(duration)
		
		// 执行预创建
		log.Println("🔨 开始预创建明天的话单表...")
		if err := models.PreCreateTomorrowTable(database.DB); err != nil {
			log.Printf("❌ 预创建话单表失败: %v", err)
		} else {
			log.Println("✅ 预创建话单表任务完成")
		}
		
		// 等待1分钟再进入下一轮循环(避免重复执行)
		time.Sleep(1 * time.Minute)
	}
}
