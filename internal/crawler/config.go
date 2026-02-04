package crawler

import "time"

// Config 爬虫配置
type Config struct {
	Timeout     time.Duration     // 爬取超时时间
	Cookies     string            // Cookie 字符串，格式: "key1=value1; key2=value2"
	Headers     map[string]string // 自定义请求头
	Proxy       string            // 代理地址，如 "http://127.0.0.1:8080"
	UserAgent   string            // 自定义 User-Agent
	Headless    bool              // 是否无头模式
	Concurrency int               // 并发数（批量爬取时）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Timeout:     30 * time.Second,
		Headers:     make(map[string]string),
		Headless:    true,
		Concurrency: 1,
	}
}
