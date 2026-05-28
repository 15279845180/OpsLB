package cache

import (
	"OpsLB/database"
	"OpsLB/models"
	"log"
	"time"
)

const (
	// 话单队列缓冲区大小
	callRecordQueueSize = 10000

	// 批量写入配置
	batchSize       = 100          // 每批最多100条
	batchTimeout    = 500 * time.Millisecond // 超时500ms强制写入
)

// CallRecordQueue 话单异步写盘队列
var CallRecordQueue chan models.CallRecord

// InitCallRecordQueue 初始化话单异步写盘队列
func InitCallRecordQueue() {
	CallRecordQueue = make(chan models.CallRecord, callRecordQueueSize)

	// 启动后台消费者（5个goroutine批量写入）
	for i := 0; i < 5; i++ {
		go callRecordWriter(i)
	}

	log.Println("[CALL-RECORD-QUEUE] Async call record queue initialized with 5 writers")
}

// callRecordWriter 话单写入消费者
func callRecordWriter(workerID int) {
	log.Printf("[CALL-RECORD-WRITER-%d] Started", workerID)

	var batch []models.CallRecord
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}

		// 批量写入数据库
		if err := database.DB.CreateInBatches(batch, batchSize).Error; err != nil {
			log.Printf("[CALL-RECORD-WRITER-%d] ❌ Batch insert failed: %v, count=%d", workerID, err, len(batch))
		} else {
			log.Printf("[CALL-RECORD-WRITER-%d] ✅ Batch inserted: count=%d", workerID, len(batch))
		}

		// 清空batch
		batch = batch[:0]
	}

	for {
		select {
		case record, ok := <-CallRecordQueue:
			if !ok {
				// 通道关闭，写入剩余数据
				flushBatch()
				log.Printf("[CALL-RECORD-WRITER-%d] Channel closed, exiting", workerID)
				return
			}

			batch = append(batch, record)

			// 达到批次大小，立即写入
			if len(batch) >= batchSize {
				flushBatch()
			}

		case <-ticker.C:
			// 超时强制写入
			flushBatch()
		}
	}
}

// EnqueueCallRecord 将话单加入异步队列（非阻塞）
func EnqueueCallRecord(record models.CallRecord) {
	select {
	case CallRecordQueue <- record:
		// 成功入队
	default:
		// 队列满了，记录日志但不阻塞
		log.Printf("[CALL-RECORD-QUEUE] ⚠️  Queue full, dropping record: call_id=%s", record.CallID)
	}
}
