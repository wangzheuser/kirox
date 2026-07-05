package task

import (
	"log"
	"strings"
	"sync"

	"reg_go/internal/data"
	"reg_go/internal/kirorsync"
	"reg_go/internal/storage"
)

const kiroRSAutoSyncQueueBuffer = 1024

type kiroRSAutoSyncJob struct {
	outDir  string
	apiURL  string
	apiKey  string
	account map[string]interface{}
}

type kiroRSAutoSyncQueue struct {
	once    sync.Once
	jobs    chan kiroRSAutoSyncJob
	process func(kiroRSAutoSyncJob)
}

var defaultKiroRSAutoSyncQueue = newKiroRSAutoSyncQueue(kiroRSAutoSyncQueueBuffer, processKiroRSAutoSyncJob)

func newKiroRSAutoSyncQueue(buffer int, process func(kiroRSAutoSyncJob)) *kiroRSAutoSyncQueue {
	if buffer <= 0 {
		buffer = 1
	}
	if process == nil {
		process = processKiroRSAutoSyncJob
	}
	return &kiroRSAutoSyncQueue{
		jobs:    make(chan kiroRSAutoSyncJob, buffer),
		process: process,
	}
}

func (q *kiroRSAutoSyncQueue) enqueue(job kiroRSAutoSyncJob) {
	if q == nil {
		return
	}
	q.once.Do(func() {
		go q.worker()
	})
	select {
	case q.jobs <- job:
	default:
		log.Printf("[Kiro] kiro.rs 自动同步队列繁忙，后台等待入队: %s", kiroRSSyncJobEmail(job))
		go func() {
			q.jobs <- job
		}()
	}
}

func (q *kiroRSAutoSyncQueue) worker() {
	for job := range q.jobs {
		q.process(job)
	}
}

func enqueueKiroRSAutoSyncResult(outDir string, result map[string]interface{}) {
	if result == nil || result["status"] != "success" {
		return
	}
	email, _ := result["email"].(string)
	email = strings.TrimSpace(email)
	if email == "" {
		log.Printf("[Kiro] kiro.rs 自动同步跳过：成功结果缺少邮箱")
		return
	}
	if !storage.GetKiroRSAutoSync() {
		return
	}
	apiURL := storage.GetKiroRSAPIURL()
	apiKey := storage.GetKiroRSAPIKey()
	if strings.TrimSpace(apiURL) == "" {
		log.Printf("[Kiro] kiro.rs 自动同步跳过 %s：未配置 API 地址", email)
		return
	}
	if strings.TrimSpace(apiKey) == "" {
		log.Printf("[Kiro] kiro.rs 自动同步跳过 %s：未配置 API Key", email)
		return
	}

	accounts, err := data.LoadAccounts(outDir)
	if err != nil {
		log.Printf("[Kiro] kiro.rs 自动同步读取账号文件失败 %s: %v", email, err)
		return
	}
	selected, missing := selectAccountsByEmail(accounts, []string{email})
	if len(selected) == 0 {
		if len(missing) > 0 {
			log.Printf("[Kiro] kiro.rs 自动同步跳过：账号尚未落盘 %s", strings.Join(missing, ", "))
		} else {
			log.Printf("[Kiro] kiro.rs 自动同步跳过：未找到账号 %s", email)
		}
		return
	}

	defaultKiroRSAutoSyncQueue.enqueue(kiroRSAutoSyncJob{
		outDir:  outDir,
		apiURL:  apiURL,
		apiKey:  apiKey,
		account: cloneKiroRSSyncAccount(selected[0]),
	})
	log.Printf("[Kiro] kiro.rs 自动同步已入队: %s", email)
}

func processKiroRSAutoSyncJob(job kiroRSAutoSyncJob) {
	email := kiroRSSyncJobEmail(job)
	if strings.TrimSpace(job.outDir) == "" || strings.TrimSpace(job.apiURL) == "" || strings.TrimSpace(job.apiKey) == "" || job.account == nil {
		log.Printf("[Kiro] kiro.rs 自动同步跳过：任务参数不完整 %s", email)
		return
	}

	log.Printf("[Kiro] 开始自动同步账号到 kiro.rs: %s", email)
	syncResult := kirorsync.SyncAccountsBlocking(job.apiURL, job.apiKey, []map[string]interface{}{job.account})
	if syncResult.Error != "" {
		log.Printf("[Kiro] kiro.rs 自动同步未执行 %s: %s", email, syncResult.Error)
		emitKiroRSSyncResult(syncResult)
		return
	}

	updated, removedRejected, _, applyErr := applyKiroRSSyncResult(job.outDir, syncResult)
	if applyErr != nil {
		log.Printf("[Kiro] kiro.rs 自动同步本地状态更新失败 %s: %v", email, applyErr)
	} else if updated > 0 {
		log.Printf("[Kiro] kiro.rs 自动同步状态已更新: %s", email)
	}
	if removedRejected > 0 {
		log.Printf("[Kiro] kiro.rs 自动同步已删除本地永久失效账号: %s", email)
	}
	log.Printf("[Kiro] kiro.rs 自动同步完成 %s: 成功 %d / 失败 %d", email, syncResult.Success, syncResult.Failed)
	emitKiroRSSyncResult(syncResult)
}

func emitKiroRSSyncResult(result kirorsync.SyncResult) {
	if Manager.OnSyncResult != nil {
		Manager.OnSyncResult(result)
	}
}

func kiroRSSyncJobEmail(job kiroRSAutoSyncJob) string {
	if job.account == nil {
		return "-"
	}
	email, _ := job.account["email"].(string)
	if email = strings.TrimSpace(email); email != "" {
		return email
	}
	return "-"
}

func cloneKiroRSSyncAccount(account map[string]interface{}) map[string]interface{} {
	if account == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(account))
	for key, value := range account {
		cloned[key] = value
	}
	return cloned
}
