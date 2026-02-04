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
	resources map[string]*Resource
	mu        sync.Mutex
	config    *Config
}

// New 创建新的爬虫实例
func New(config *Config) *Spider {
	if config == nil {
		config = DefaultConfig()
	}
	return &Spider{
		resources: make(map[string]*Resource),
		config:    config,
	}
}

// Crawl 爬取指定URL的所有资源
func (s *Spider) Crawl(targetURL string) error {
	// 创建上下文
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", s.config.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)

	// 添加代理支持
	if s.config.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(s.config.Proxy))
	}

	// 自定义 User-Agent
	if s.config.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(s.config.UserAgent))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// 先启动浏览器并等待就绪（解决冷启动时 websocket url timeout 问题）
	if err := chromedp.Run(ctx); err != nil {
		return fmt.Errorf("failed to start browser: %v", err)
	}

	// 设置超时
	ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	// 启用网络事件
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			go s.handleResponse(ctx, ev)
		}
	})

	// 构建初始化 actions
	actions := []chromedp.Action{
		network.Enable(),
	}

	// 设置自定义 Headers
	if len(s.config.Headers) > 0 {
		headers := make(map[string]any)
		for k, v := range s.config.Headers {
			headers[k] = v
		}
		actions = append(actions, network.SetExtraHTTPHeaders(network.Headers(headers)))
	}

	// 设置 Cookie
	if s.config.Cookies != "" {
		cookies := s.parseCookies(targetURL, s.config.Cookies)
		if len(cookies) > 0 {
			actions = append(actions, network.SetCookies(cookies))
		}
	}

	// 导航到目标 URL
	actions = append(actions,
		chromedp.Navigate(targetURL),
		chromedp.Sleep(3*time.Second), // 等待初始加载
	)

	// 访问页面并等待加载完成
	err := chromedp.Run(ctx, actions...)

	if err != nil {
		return fmt.Errorf("failed to crawl %s: %v", targetURL, err)
	}

	// 滚动页面以触发懒加载资源
	log.Println("滚动页面以触发懒加载资源...")
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight/4)`, nil),
		chromedp.Sleep(1*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight/2)`, nil),
		chromedp.Sleep(1*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight*3/4)`, nil),
		chromedp.Sleep(1*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`window.scrollTo(0, 0)`, nil),
		chromedp.Sleep(1*time.Second),
	)

	if err != nil {
		log.Printf("警告: 滚动页面时出错: %v", err)
	}

	// 额外等待时间以确保所有异步资源都被加载
	log.Println("等待所有异步资源加载...")
	time.Sleep(5 * time.Second)

	return nil
}

// handleResponse 处理网络响应
func (s *Spider) handleResponse(ctx context.Context, ev *network.EventResponseReceived) {
	resp := ev.Response
	requestID := ev.RequestID

	// 检查是否已经抓取过此资源
	s.mu.Lock()
	if _, exists := s.resources[resp.URL]; exists {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	resource := &Resource{
		URL:          resp.URL,
		StatusCode:   int(resp.Status),
		MimeType:     resp.MimeType,
		Headers:      make(map[string]string),
		ResponseTime: time.Now(),
	}

	// 复制headers
	for k, v := range resp.Headers {
		if str, ok := v.(string); ok {
			resource.Headers[k] = str
		}
	}

	// 获取响应体
	go func() {
		var body []byte
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				body, err = network.GetResponseBody(requestID).Do(ctx)
				return err
			}),
		)

		if err != nil {
			// 某些资源可能无法获取body（如304 Not Modified）
			// 尝试直接下载
			body = s.downloadResource(resource.URL)
		}

		resource.Content = body

		// 保存资源
		s.mu.Lock()
		s.resources[resource.URL] = resource
		s.mu.Unlock()

		log.Printf("Captured: %s [%s] - %d bytes", resource.URL, resource.MimeType, len(resource.Content))
	}()
}

// downloadResource 直接下载资源（作为备用方案）
func (s *Spider) downloadResource(url string) []byte {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return body
}

// GetResources 获取所有抓取的资源
func (s *Spider) GetResources() map[string]*Resource {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 返回副本以避免并发问题
	result := make(map[string]*Resource, len(s.resources))
	maps.Copy(result, s.resources)

	return result
}

// parseCookies 解析 Cookie 字符串为 network.CookieParam 数组
func (s *Spider) parseCookies(targetURL, cookieStr string) []*network.CookieParam {
	var cookies []*network.CookieParam

	// 解析目标 URL 获取域名
	u, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("警告: 无法解析 URL %s: %v", targetURL, err)
		return cookies
	}
	domain := u.Hostname()

	// 解析 Cookie 字符串（格式: "key1=value1; key2=value2"）
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
