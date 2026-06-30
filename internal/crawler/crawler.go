package crawler

import (
	"context"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const maxBodyBytes = 100 * 1024 * 1024 // 100 MB per resource

// Resource 表示一个网络资源
type Resource struct {
	URL          string
	Method       string
	StatusCode   int
	MimeType     string
	Content      []byte
	Headers      map[string]string
	ResponseTime time.Time
}

// Spider 爬虫结构
type Spider struct {
	resources   map[string]*Resource
	mu          sync.Mutex
	wg          sync.WaitGroup
	config      *Config
	httpClient  *http.Client
	lastCapture time.Time // 最后一次成功抓取资源的时间，用于空闲检测
}

// New 创建新的爬虫实例
func New(config *Config) *Spider {
	if config == nil {
		config = DefaultConfig()
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.Proxy != "" {
		if proxyURL, err := url.Parse(config.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &Spider{
		resources: make(map[string]*Resource),
		config:    config,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// buildAllocatorOptions 构建 Chrome 启动参数（供 Crawl 和 Pool 共用）
func buildAllocatorOptions(config *Config) []chromedp.ExecAllocatorOption {
	base := make([]chromedp.ExecAllocatorOption, len(chromedp.DefaultExecAllocatorOptions))
	copy(base, chromedp.DefaultExecAllocatorOptions[:])
	opts := append(base,
		chromedp.Flag("headless", config.Headless),
		chromedp.Flag("disable-gpu", true),
		// 保留 Chrome 渲染器沙箱，不使用 --no-sandbox
	)
	if config.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(config.ChromePath))
	}
	if config.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(config.Proxy))
	}
	if config.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(config.UserAgent))
	}
	return opts
}

// Crawl 单 URL 模式：自行启动/销毁 Chrome 进程。
// 浏览器热身时间不计入页面爬取超时。
func (s *Spider) Crawl(targetURL string) error {
	if err := validateURL(targetURL); err != nil {
		return err
	}

	opts := buildAllocatorOptions(s.config)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// 热身：在同一 tab context 上启动浏览器并建立连接；
	// 不使用子 timeout context，避免 cancel() 污染 chromedp 内部 session。
	startAt := time.Now()
	if err := chromedp.Run(ctx); err != nil {
		return fmt.Errorf("chrome failed to start: %w", err)
	}
	log.Printf("浏览器启动耗时 %.1fs", time.Since(startAt).Seconds())

	// 超时从浏览器就绪后开始，不含启动时间。
	// 注意：对同一变量重赋值；原 chromedp context 仍在树中，超时是其子节点。
	ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	return s.crawlInTab(ctx, targetURL)
}

// CrawlInContext 批量模式：在浏览器池提供的 allocCtx 中开新 Tab 爬取，不关闭 Chrome 进程。
// Tab 的超时独立计算，不含浏览器进程的启动时间。
func (s *Spider) CrawlInContext(allocCtx context.Context, targetURL string) error {
	if err := validateURL(targetURL); err != nil {
		return err
	}

	// 在现有 Chrome 进程中创建新 Tab（chromedp 懒创建，真正 Run 时才 open tab）
	tabCtx, tabCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer tabCancel()

	// 超时仅覆盖 Tab 生命周期
	ctx, cancel := context.WithTimeout(tabCtx, s.config.Timeout)
	defer cancel()

	return s.crawlInTab(ctx, targetURL)
}

// crawlInTab 在已有上下文（含超时）中执行完整爬取流程。
// 调用方（Crawl / CrawlInContext）负责设置超时，此函数不再重复创建。
func (s *Spider) crawlInTab(ctx context.Context, targetURL string) error {
	// 初始化 lastCapture 基线
	s.mu.Lock()
	s.lastCapture = time.Now()
	s.mu.Unlock()

	// 监听网络响应事件
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			go s.handleResponse(ctx, ev)
		}
	})

	actions := []chromedp.Action{network.Enable()}

	if len(s.config.Headers) > 0 {
		headers := make(map[string]any)
		for k, v := range s.config.Headers {
			headers[k] = v
		}
		actions = append(actions, network.SetExtraHTTPHeaders(network.Headers(headers)))
	}

	if s.config.Cookies != "" {
		if cookies := s.parseCookies(targetURL, s.config.Cookies); len(cookies) > 0 {
			actions = append(actions, network.SetCookies(cookies))
		}
	}

	// 导航：等待 DOM 就绪替代固定 Sleep
	actions = append(actions,
		chromedp.Navigate(targetURL),
		chromedp.ActionFunc(waitForReadyState),
	)

	if err := chromedp.Run(ctx, actions...); err != nil {
		return fmt.Errorf("failed to crawl %s: %w", targetURL, err)
	}

	// 滚动触发懒加载：每步独立容错
	s.scrollPage(ctx)

	// 网络空闲检测（替代固定 Sleep）
	s.waitForIdle()

	// 等待所有资源下载 goroutine 完成
	s.wg.Wait()

	return nil
}

// scrollPage 分步滚动页面触发懒加载，每步独立容错不影响后续步骤
func (s *Spider) scrollPage(ctx context.Context) {
	steps := []struct {
		frac  float64
		delay time.Duration
	}{
		{0.25, 800 * time.Millisecond},
		{0.50, 800 * time.Millisecond},
		{0.75, 800 * time.Millisecond},
		{1.00, 1000 * time.Millisecond},
		{0.00, 500 * time.Millisecond},
	}

	log.Println("滚动页面以触发懒加载资源...")
	for _, step := range steps {
		if ctx.Err() != nil {
			log.Printf("警告: 页面上下文已结束，跳过剩余滚动步骤")
			break
		}

		// JS 加 try-catch：兼容 document.body 为 null 的异常页面
		js := fmt.Sprintf(`(function(){
			try {
				var el = document.scrollingElement || document.body || document.documentElement;
				if (el) window.scrollTo(0, Math.round(el.scrollHeight * %.2f));
			} catch(e) {}
		})()`, step.frac)

		if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
			log.Printf("警告: 滚动 %.0f%% 时出错（跳过此步）: %v", step.frac*100, err)
		}

		// 等待期间同时监听 ctx 取消，避免超时后还在 sleep
		timer := time.NewTimer(step.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// waitForReadyState 轮询 document.readyState 直到 complete 或最多 10s
func waitForReadyState(ctx context.Context) error {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timeout:
			return nil
		case <-ticker.C:
			var state string
			if err := chromedp.Evaluate(`document.readyState`, &state).Do(ctx); err != nil {
				return nil
			}
			if state == "complete" {
				return nil
			}
		}
	}
}

// waitForIdle 等待网络空闲：连续 2s 无新资源，或达到 IdleTimeout 上限
func (s *Spider) waitForIdle() {
	const idleThreshold = 2 * time.Second
	deadline := time.Now().Add(s.config.IdleTimeout)
	log.Println("等待网络空闲...")
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		s.mu.Lock()
		since := time.Since(s.lastCapture)
		s.mu.Unlock()
		if since >= idleThreshold {
			log.Printf("网络已空闲 %.1f 秒，继续处理", since.Seconds())
			return
		}
	}
	log.Printf("已等待 %v 达到上限，强制继续", s.config.IdleTimeout)
}

// handleResponse 处理网络响应事件
func (s *Spider) handleResponse(ctx context.Context, ev *network.EventResponseReceived) {
	resp := ev.Response
	requestID := ev.RequestID

	resource := &Resource{
		URL:          resp.URL,
		StatusCode:   int(resp.Status),
		MimeType:     resp.MimeType,
		Headers:      make(map[string]string),
		ResponseTime: time.Now(),
	}
	for k, v := range resp.Headers {
		if str, ok := v.(string); ok {
			resource.Headers[k] = str
		}
	}

	// 占位写入：check + insert 在同一把锁内，消除 TOCTOU 竞态
	s.mu.Lock()
	if _, exists := s.resources[resp.URL]; exists {
		s.mu.Unlock()
		return
	}
	s.resources[resp.URL] = resource
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var body []byte
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				body, err = network.GetResponseBody(requestID).Do(ctx)
				return err
			}),
		)
		if err != nil {
			// 304 Not Modified 等情况，使用配置好的 HTTP 客户端重试
			body = s.downloadResource(resource.URL)
		}

		s.mu.Lock()
		resource.Content = body
		s.lastCapture = time.Now() // 更新空闲检测基线
		s.mu.Unlock()

		log.Printf("Captured: %s [%s] - %d bytes", resource.URL, resource.MimeType, len(body))
	}()
}

// downloadResource 直接下载资源（备用），继承代理、Cookie、Headers 配置
func (s *Spider) downloadResource(targetURL string) []byte {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil
	}
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}
	if s.config.Cookies != "" {
		req.Header.Set("Cookie", s.config.Cookies)
	}
	if s.config.UserAgent != "" {
		req.Header.Set("User-Agent", s.config.UserAgent)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil
	}
	return body
}

// GetResources 返回所有已抓取资源的副本（线程安全）
func (s *Spider) GetResources() map[string]*Resource {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]*Resource, len(s.resources))
	maps.Copy(result, s.resources)
	return result
}

// parseCookies 解析 Cookie 字符串
func (s *Spider) parseCookies(targetURL, cookieStr string) []*network.CookieParam {
	var cookies []*network.CookieParam
	u, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("警告: 无法解析 URL %s: %v", targetURL, err)
		return cookies
	}
	domain := u.Hostname()

	for pair := range strings.SplitSeq(cookieStr, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx == -1 {
			continue
		}
		name := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if name == "" {
			continue
		}
		cookies = append(cookies, &network.CookieParam{
			Name:   name,
			Value:  value,
			Domain: domain,
		})
	}
	return cookies
}

// validateURL 校验 URL scheme，防止 file:// / javascript: 等传入浏览器
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		scheme := ""
		if u != nil {
			scheme = u.Scheme
		}
		return fmt.Errorf("不支持的 URL scheme %q：仅允许 http 和 https", scheme)
	}
	return nil
}
