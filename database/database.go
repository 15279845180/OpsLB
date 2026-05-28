package database

import (
	"OpsLB/config"
	"OpsLB/models"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"time"
)

var DB *gorm.DB

func InitDB(cfg *config.MySQLConfig) error {
	dsn := cfg.DSN()
	
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // 只打印警告和错误,不打印SQL
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %v", err)
	}

	// 配置连接池（高并发SIP场景必需）
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(200)       // 最大打开连接数（高并发场景提升到200）
	sqlDB.SetMaxIdleConns(50)        // 最大空闲连接数（保持50个常驻连接）
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // 连接最大生命周期
	log.Println("Database connection pool configured: max_open=200, max_idle=50")

	// 注意：表已手动创建，不需要自动迁移
	// err = DB.AutoMigrate(...)

	log.Println("Database connected successfully")
	
	// 初始化默认管理员账号
	initDefaultAdmin()
	
	return nil
}

func initDefaultAdmin() {
	var user models.User
	result := DB.Where("username = ?", "admin").First(&user)
	
	if result.Error != nil {
		// admin不存在，创建默认管理员
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Failed to hash password: %v", err)
			return
		}
		
		admin := &models.User{
			Username: "admin",
			Password: string(hashedPassword),
			Nickname: "管理员",
			Status:   1,
		}
		DB.Create(admin)
		log.Println("✅ 默认管理员创建成功: admin/admin123")
	} else {
		// admin已存在，重置密码为admin123（仅开发/测试环境）
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Failed to hash password: %v", err)
			return
		}
		
		// 更新密码和状态
		DB.Model(&user).Updates(map[string]interface{}{
			"password": string(hashedPassword),
			"status":   1, // 确保启用
		})
		log.Println("✅ 默认管理员密码已重置: admin/admin123")
	}
}
