package cache

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"strconv"
	"strings"
	"time"
)

// AllocateInboundCall 原子申请入局并发
// 返回值: status ("SUCCESS"|"DUPLICATE"|"REJECT"|"FULL"), currentConcurrent, error
func AllocateInboundCall(callID string, inboundID uint, maxConcurrent int) (string, int64, error) {
	key := fmt.Sprintf("call_alloc:%s", callID)
	counterKey := fmt.Sprintf("inbound:%d:calls", inboundID)

	luaScript := `
		local key = KEYS[1]
		local counterKey = KEYS[2]
		local inbound_id = ARGV[1]
		local max_concurrent = tonumber(ARGV[2])

		local current_status = redis.call('HGET', key, 'status')
		if current_status then
			if current_status == 'pending' or current_status == 'active' then
				return {"DUPLICATE", redis.call('GET', counterKey) or '0'}
			else
				return {"REJECT", '0'}
			end
		end

		local current = tonumber(redis.call('GET', counterKey) or '0')
		if current >= max_concurrent then
			return {"FULL", tostring(current)}
		end

		redis.call('HSET', key, 'status', 'active', 'inbound_id', inbound_id, 'created_at', redis.call('TIME')[1])
		redis.call('EXPIRE', key, 86400)
		redis.call('INCR', counterKey)

		return {"SUCCESS", tostring(current + 1)}
	`

	result, err := Redis.Eval(Ctx, luaScript, []string{key, counterKey}, inboundID, maxConcurrent).Result()
	if err != nil {
		return "", 0, err
	}

	values := result.([]interface{})
	status := values[0].(string)
	currentStr := values[1].(string)
	current, _ := strconv.ParseInt(currentStr, 10, 64)

	return status, current, nil
}

// AllocateOutboundCall 原子申请出局并发
// 返回值: status ("SUCCESS"|"NOT_FOUND"|"INVALID"|"FULL"), currentConcurrent, error
func AllocateOutboundCall(callID string, gatewayID uint, maxConcurrent int) (string, int64, error) {
	key := fmt.Sprintf("call_alloc:%s", callID)
	counterKey := fmt.Sprintf("gateway:%d:calls", gatewayID)

	luaScript := `
		local key = KEYS[1]
		local counterKey = KEYS[2]
		local gateway_id = ARGV[1]
		local max_concurrent = tonumber(ARGV[2])

		local current_status = redis.call('HGET', key, 'status')
		if not current_status then
			return {"NOT_FOUND", '0'}
		end
		if current_status ~= 'active' then
			return {"INVALID", '0'}
		end

		local current = tonumber(redis.call('GET', counterKey) or '0')
		if current >= max_concurrent then
			return {"FULL", tostring(current)}
		end

		redis.call('HSET', key, 'gateway_id', gateway_id)
		redis.call('INCR', counterKey)

		return {"SUCCESS", tostring(current + 1)}
	`

	result, err := Redis.Eval(Ctx, luaScript, []string{key, counterKey}, gatewayID, maxConcurrent).Result()
	if err != nil {
		return "", 0, err
	}

	values := result.([]interface{})
	status := values[0].(string)
	currentStr := values[1].(string)
	current, _ := strconv.ParseInt(currentStr, 10, 64)

	return status, current, nil
}

// ReleaseCall 安全释放并发
// 返回值: released, inboundReleased, outboundReleased, error
func ReleaseCall(callID string) (bool, bool, bool, error) {
	key := fmt.Sprintf("call_alloc:%s", callID)

	result, err := Redis.HGetAll(Ctx, key).Result()
	if err != nil {
		return false, false, false, err
	}

	if len(result) == 0 {
		return false, false, false, nil
	}

	status := result["status"]
	if status == "released" {
		return false, false, false, nil
	}

	inboundIDStr := result["inbound_id"]
	gatewayIDStr := result["gateway_id"]

	inboundReleased := false
	outboundReleased := false

	if inboundIDStr != "" && inboundIDStr != "0" {
		var inboundID uint
		fmt.Sscanf(inboundIDStr, "%d", &inboundID)
		if inboundID > 0 {
			DecrInboundCalls(inboundID)
			inboundReleased = true
		}
	}

	if gatewayIDStr != "" && gatewayIDStr != "0" {
		var gatewayID uint
		fmt.Sscanf(gatewayIDStr, "%d", &gatewayID)
		if gatewayID > 0 {
			DecrGatewayCalls(gatewayID)
			outboundReleased = true
		}
	}

	// 更新状态为已释放
	Redis.HSet(Ctx, key, "status", "released", "released_at", time.Now().Unix())

	return true, inboundReleased, outboundReleased, nil
}

// GetCallInfo 获取通话Redis记录
func GetCallInfo(callID string) (map[string]string, error) {
	key := fmt.Sprintf("call_alloc:%s", callID)
	return Redis.HGetAll(Ctx, key).Result()
}

// ScanTimeoutCalls 扫描超时未释放的活跃通话
// timeout: 通话最大存活时间（如5分钟）
// 优化: 使用Redis Pipeline减少网络往返,使用HScan替代全量Scan
func ScanTimeoutCalls(timeout time.Duration) ([]string, error) {
	var callIDs []string
	var cursor uint64
	now := time.Now().Unix()
	cutoff := now - int64(timeout.Seconds())

	for {
		// HScan遍历hash key,每次批量处理
		keys, nextCursor, err := Redis.Scan(Ctx, cursor, "call_alloc:*", 50).Result()
		if err != nil {
			return nil, err
		}

		if len(keys) > 0 {
			// Pipeline批量获取status和created_at
			pipe := Redis.Pipeline()
			statusCmds := make([]*redis.StringCmd, len(keys))
			createdCmds := make([]*redis.StringCmd, len(keys))

			for i, key := range keys {
				statusCmds[i] = pipe.HGet(Ctx, key, "status")
				createdCmds[i] = pipe.HGet(Ctx, key, "created_at")
			}

			_, err := pipe.Exec(Ctx)
			if err != nil {
				return nil, err
			}

			for i, key := range keys {
				if statusCmds[i].Val() != "active" {
					continue
				}
				createdAt, _ := strconv.ParseInt(createdCmds[i].Val(), 10, 64)
				if createdAt > 0 && createdAt < cutoff {
					callID := strings.TrimPrefix(key, "call_alloc:")
					callIDs = append(callIDs, callID)
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return callIDs, nil
}
