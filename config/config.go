package config

import (
	"fmt"
	"os"
)

type Config struct {
	MySQL  MySQLConfig
	Redis  RedisConfig
	Server ServerConfig
	JWT    JWTConfig
}

type MySQLConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type ServerConfig struct {
	Port string
}

type JWTConfig struct {
	Secret string
}

func LoadConfig() *Config {
	return &Config{
		MySQL: MySQLConfig{
			Host:     getEnv("MYSQL_HOST", "localhost"),
			Port:     getEnv("MYSQL_PORT", "3306"),
			Database: getEnv("MYSQL_DB", "opslb"),
			Username: getEnv("MYSQL_USER", "OpsLB"),
			Password: getEnv("MYSQL_PASSWORD", "YsmywJkcSTCcdw2J"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", "OpsLB_2026_Redis!@#"),
			DB:       0,
		},
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "OpsLB_Secret_Key_2026"),
		},
	}
}

func (c *MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.Username, c.Password, c.Host, c.Port, c.Database)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
