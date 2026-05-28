package models

import (
	"fmt"
	"gorm.io/gorm"
	"log"
	"time"
)

// 用户表
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Username  string         `gorm:"column:username;uniqueIndex;size:50;not null" json:"username"`
	Password  string         `gorm:"column:password;size:255;not null" json:"-"`
	Nickname  string         `gorm:"column:nickname;size:50" json:"nickname"`
	Status    int            `gorm:"column:status;default:1" json:"status"` // 1启用 0禁用
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (User) TableName() string {
	return "e_users"
}

// 入局网关
type InboundGateway struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	Name          string         `gorm:"column:name;size:100;not null" json:"name"`
	IPs           string         `gorm:"column:ips;type:text;not null" json:"ips"`
	MaxConcurrent int            `gorm:"column:max_concurrent;default:100" json:"max_concurrent"`
	CurrentCalls  int            `gorm:"-" json:"current_calls"`
	Status        int            `gorm:"column:status;default:1" json:"status"`
	Description   string         `gorm:"column:description;size:255" json:"description"`
	CreatedAt     time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (InboundGateway) TableName() string {
	return "e_inbound_gateways"
}

// 出局网关
type OutboundGateway struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	Name            string         `gorm:"column:name;size:100;not null" json:"name"`
	IP              string         `gorm:"column:ip;size:50;not null" json:"ip"`
	Port            int            `gorm:"column:port;default:5060" json:"port"`
	Protocol        string         `gorm:"column:protocol;default:'udp';size:10" json:"protocol"`
	Priority        int            `gorm:"column:priority;default:1" json:"priority"`
	Weight          int            `gorm:"column:weight;default:1" json:"weight"`
	MaxConcurrent   int            `gorm:"column:max_concurrent;default:100" json:"max_concurrent"`
	MaxCPS          int            `gorm:"column:max_cps;default:10" json:"max_cps"`
	Status          int            `gorm:"column:status;default:1" json:"status"`
	CurrentCalls    int            `gorm:"-" json:"current_calls"`
	Description     string         `gorm:"column:description;size:255" json:"description"`
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (OutboundGateway) TableName() string {
	return "e_outbound_gateways"
}

// 系统配置
type SystemConfig struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Key       string    `gorm:"column:config_key;uniqueIndex;size:50;not null" json:"key"`
	Value     string    `gorm:"column:config_value;size:500" json:"value"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SystemConfig) TableName() string {
	return "cfg_system"
}

// 呼叫记录（按日期分表）
type CallRecord struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	CallID            string    `gorm:"column:call_id;index" json:"call_id"`
	InboundIP         string    `gorm:"column:inbound_ip;index" json:"inbound_ip"`
	OutboundGatewayID uint      `gorm:"column:outbound_gateway_id;index" json:"outbound_gateway_id"`
	OutboundGatewayIP string    `gorm:"column:outbound_gateway_ip;index" json:"outbound_gateway_ip"`
	CallerNumber      string    `gorm:"column:caller_number;index" json:"caller_number"`
	CalledNumber      string    `gorm:"column:called_number;index" json:"called_number"`
	InviteTime        time.Time `gorm:"column:invite_time;index" json:"invite_time"`
	AnswerTime        time.Time `gorm:"column:answer_time" json:"answer_time"`
	EndTime           time.Time `gorm:"column:end_time" json:"end_time"`
	StatusCode        int       `gorm:"column:status_code" json:"status_code"`
	StatusText        string    `gorm:"column:status_text;size:50" json:"status_text"`
	TalkDuration      int       `gorm:"column:talk_duration" json:"talk_duration"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

func (CallRecord) TableName() string {
	// 按日期分表：cdr_call_20260418
	return "cdr_call_" + time.Now().Format("20060102")
}

// createCDRTable 创建话单表的公共函数
func createCDRTable(db *gorm.DB, tableName, dateStr string) error {
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			call_id VARCHAR(128) NOT NULL COMMENT 'SIP Call-ID',
			inbound_ip VARCHAR(50) NOT NULL COMMENT '入局IP',
			outbound_gateway_id INT UNSIGNED NOT NULL COMMENT '出局网关ID',
			outbound_gateway_ip VARCHAR(50) NOT NULL COMMENT '出局网关IP',
			caller_number VARCHAR(50) DEFAULT NULL COMMENT '主叫号码',
			called_number VARCHAR(50) DEFAULT NULL COMMENT '被叫号码',
			invite_time DATETIME NOT NULL COMMENT 'INVITE时间',
			answer_time DATETIME DEFAULT NULL COMMENT '接通时间',
			end_time DATETIME DEFAULT NULL COMMENT '结束时间',
			status_code INT NOT NULL DEFAULT 0 COMMENT 'SIP状态码',
			status_text VARCHAR(50) DEFAULT NULL COMMENT '状态描述',
			talk_duration INT NOT NULL DEFAULT 0 COMMENT '通话时长(秒)',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			INDEX idx_call_id (call_id),
			INDEX idx_inbound_ip (inbound_ip),
			INDEX idx_outbound_gateway_id (outbound_gateway_id),
			INDEX idx_outbound_gateway_ip (outbound_gateway_ip),
			INDEX idx_caller_number (caller_number),
			INDEX idx_called_number (called_number),
			INDEX idx_invite_time (invite_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='话单表-%s'
	`, tableName, dateStr)

	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("创建表失败 %s: %v", tableName, err)
	}

	// 注册到 OpenSIPS version 表（如果 permissions 模块需要）
	db.Exec(`INSERT IGNORE INTO version (table_name, table_version) VALUES (?, 1)`, tableName)
	return nil
}

// PreCreateTomorrowTable 预创建明天的话单表(定时任务调用)
func PreCreateTomorrowTable(db *gorm.DB) error {
	tomorrow := time.Now().AddDate(0, 0, 1)
	tableName := "cdr_call_" + tomorrow.Format("20060102")
	dateStr := tomorrow.Format("2006-01-02")

	// 检查表是否已存在
	var count int64
	err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		tableName,
	).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("检查表失败: %v", err)
	}
	if count > 0 {
		log.Printf("✅ 话单表已存在: %s", tableName)
		return nil
	}

	if err := createCDRTable(db, tableName, dateStr); err != nil {
		return err
	}
	log.Printf("✅ 预创建话单表成功: %s (日期: %s)", tableName, dateStr)
	return nil
}

// EnsureTodayTableExists 确保今天的话单表存在(启动时检查)
func EnsureTodayTableExists(db *gorm.DB) error {
	today := time.Now()
	tableName := "cdr_call_" + today.Format("20060102")
	dateStr := today.Format("2006-01-02")

	var count int64
	err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		tableName,
	).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("检查表失败: %v", err)
	}
	if count > 0 {
		return nil
	}

	if err := createCDRTable(db, tableName, dateStr); err != nil {
		return err
	}
	log.Printf("✅ 启动时创建话单表: %s", tableName)
	return nil
}

// 操作日志
type OperationLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	UserID    uint      `gorm:"column:user_id" json:"user_id"`
	Username  string    `gorm:"column:username;size:50" json:"username"`
	Action    string    `gorm:"column:action;size:50" json:"action"`
	Content   string    `gorm:"column:content;size:500" json:"content"`
	IP        string    `gorm:"column:ip;size:50" json:"ip"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (OperationLog) TableName() string {
	return "log_operations"
}
