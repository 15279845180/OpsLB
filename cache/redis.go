package cache

import (
	"OpsLB/config"
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

var Redis *redis.Client
var Ctx = context.Background()  // 导出供 Handler 使用

func InitRedis(cfg *config.RedisConfig) error {
	Redis = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     500,             // 连接池大小（高并发场景提升到500）
		MinIdleConns: 50,              // 最小空闲连接（保持50个常驻连接）
		PoolTimeout:  2 * time.Second, // 获取连接超时
		ReadTimeout:  2 * time.Second, // 读超时
		WriteTimeout: 2 * time.Second, // 写超时
		MaxRetries:   3,               // 最大重试次数
	})

	_, err := Redis.Ping(Ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect redis: %v", err)
	}

	log.Printf("Redis connection pool configured: pool_size=500, min_idle=50")
	return nil
}

// 并发计数相关
func IncrGatewayCalls(gatewayID uint) error {
	key := fmt.Sprintf("gateway:%d:calls", gatewayID)
	
	// 原子操作: Incr + 刷新TTL
	err := Redis.Incr(Ctx, key).Err()
	if err != nil {
		return err
	}
	
	// 设置或刷新TTL(30分钟)
	Redis.Expire(Ctx, key, 30*time.Minute)
	return nil
}

func DecrGatewayCalls(gatewayID uint) error {
	key := fmt.Sprintf("gateway:%d:calls", gatewayID)
	
	// 幂等性保护：使用 Lua 脚本原子性检查并减少
	// 防止 CANCEL 触发两次 call-end 导致并发计数器变负数
	luaScript := `
		local key = KEYS[1]
		local current = tonumber(redis.call('GET', key) or '0')
		if current > 0 then
			local new_val = redis.call('DECR', key)
			if new_val <= 0 then
				redis.call('DEL', key)
				return 0
			end
			return new_val
		end
		return 0  -- 已经是0，不重复减少（幂等）
	`
	
	_, err := Redis.Eval(Ctx, luaScript, []string{key}).Result()
	return err
}

func GetGatewayCalls(gatewayID uint) (int64, error) {
	key := fmt.Sprintf("gateway:%d:calls", gatewayID)
	val, err := Redis.Get(Ctx, key).Int64()
	if err != nil {
		// key不存在时返回0
		return 0, nil
	}
	return val, nil
}

// CPS限流相关
func CheckAndIncCPS(gatewayID uint, maxCPS int) (bool, error) {
	key := fmt.Sprintf("gateway:%d:cps:%s", gatewayID, time.Now().Format("20060102150405"))
	
	pipe := Redis.Pipeline()
	incr := pipe.Incr(Ctx, key)
	pipe.Expire(Ctx, key, 2*time.Second)
	
	_, err := pipe.Exec(Ctx)
	if err != nil {
		return false, err
	}
	
	return incr.Val() <= int64(maxCPS), nil
}

// 加权轮询相关
func GetGatewayRRIndex(priority int) (int64, error) {
	key := fmt.Sprintf("gateway:rr:%d", priority)
	return Redis.Get(Ctx, key).Int64()
}

func IncrGatewayRRIndex(priority int) error {
	key := fmt.Sprintf("gateway:rr:%d", priority)
	return Redis.Incr(Ctx, key).Err()
}

func ResetGatewayRRIndex(priority int) error {
	key := fmt.Sprintf("gateway:rr:%d", priority)
	return Redis.Set(Ctx, key, 0, 0).Err()
}

// 入局网关并发计数相关
func IncrInboundCalls(gatewayID uint) error {
	key := fmt.Sprintf("inbound:%d:calls", gatewayID)
	
	// 原子操作: Incr + 刷新TTL
	err := Redis.Incr(Ctx, key).Err()
	if err != nil {
		return err
	}
	
	// 设置或刷新TTL(30分钟)
	Redis.Expire(Ctx, key, 30*time.Minute)
	return nil
}

func DecrInboundCalls(gatewayID uint) error {
	key := fmt.Sprintf("inbound:%d:calls", gatewayID)
	
	// 幂等性保护：使用 Lua 脚本原子性检查并减少
	// 防止 CANCEL 触发两次 call-end 导致并发计数器变负数
	luaScript := `
		local key = KEYS[1]
		local current = tonumber(redis.call('GET', key) or '0')
		if current > 0 then
			local new_val = redis.call('DECR', key)
			if new_val <= 0 then
				redis.call('DEL', key)
				return 0
			end
			return new_val
		end
		return 0  -- 已经是0，不重复减少（幂等）
	`
	
	_, err := Redis.Eval(Ctx, luaScript, []string{key}).Result()
	return err
}

func GetInboundCalls(gatewayID uint) (int64, error) {
	key := fmt.Sprintf("inbound:%d:calls", gatewayID)
	val, err := Redis.Get(Ctx, key).Int64()
	if err != nil {
		// key不存在时返回0
		return 0, nil
	}
	return val, nil
}

