package crawler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

// Pool 管理固定数量的 Chrome 浏览器进程供批量爬取复用。
// 浏览器进程数 = concurrency，爬取时每个 URL 在对应进程内开新 Tab，
// Tab 关闭但进程保留，彻底消除每 URL 冷启动开销，也限制了系统进程总量。
type Pool struct {
	available chan context.Context  // 可用的 Chrome allocCtx（进程级别）
	cancels   []context.CancelFunc // 对应的关闭函数
	config    *Config
}

// NewPool 创建并预热浏览器池。
// 全部浏览器启动完成后才返回，启动失败则关闭已启动的进程并返回错误。
func NewPool(config *Config) (*Pool, error) {
	size := config.Concurrency
	if size <= 0 {
		size = 1
	}

	// 进程预估提醒：每个 Chrome 实例约产生 5-8 个 OS 进程
	const chromeProcPerInstance = 6
	estimated := size * chromeProcPerInstance
	log.Printf("浏览器池: 预启动 %d 个 Chrome 实例，预计占用约 %d+ 个系统进程", size, estimated)
	if estimated > 60 {
		log.Printf("警告: 系统进程占用较高，请确认进程上限 (ulimit -u 或 /proc/sys/kernel/pid_max)")
	}

	p := &Pool{
		available: make(chan context.Context, size),
		cancels:   make([]context.CancelFunc, 0, size),
		config:    config,
	}

	for i := range size {
		allocCtx, cancel, err := p.launchBrowser(i+1, size)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("浏览器槽 %d/%d 启动失败: %w", i+1, size, err)
		}
		p.cancels = append(p.cancels, cancel)
		p.available <- allocCtx
	}

	log.Printf("浏览器池就绪，共 %d 个进程", size)
	return p, nil
}

// launchBrowser 启动单个 Chrome 进程并预热（开临时 Tab 验证可用后关闭 Tab）
func (p *Pool) launchBrowser(idx, total int) (context.Context, context.CancelFunc, error) {
	opts := buildAllocatorOptions(p.config)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// 预热：开临时 Tab 触发 Chrome 进程真正启动，完成后关闭 Tab 保留进程
	warmCtx, warmCancel := chromedp.NewContext(allocCtx)
	startCtx, startCancel := context.WithTimeout(warmCtx, 30*time.Second)
	defer startCancel()

	start := time.Now()
	if err := chromedp.Run(startCtx); err != nil {
		warmCancel()
		allocCancel()
		return nil, nil, fmt.Errorf("预热失败: %w", err)
	}
	warmCancel() // 关闭预热 Tab，Chrome 进程继续存活

	log.Printf("浏览器 %d/%d 已就绪 (启动耗时 %.1fs)", idx, total, time.Since(start).Seconds())
	return allocCtx, allocCancel, nil
}

// Acquire 获取一个空闲的 Chrome allocCtx，无空闲时阻塞等待
func (p *Pool) Acquire() context.Context {
	return <-p.available
}

// Release 归还 Chrome allocCtx 到池中
func (p *Pool) Release(allocCtx context.Context) {
	p.available <- allocCtx
}

// Close 关闭所有 Chrome 进程（必须在所有爬取任务完成后调用）
func (p *Pool) Close() {
	for _, cancel := range p.cancels {
		cancel()
	}
	p.cancels = nil
}
