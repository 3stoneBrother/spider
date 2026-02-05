# Spider - 浏览器模拟爬虫工具

[English](README.md)

一个基于 Go 语言和 chromedp 开发的网页爬虫工具，可以模拟真实浏览器加载网页，执行 JavaScript 代码，并抓取所有动态加载的资源。

## 功能特性

- 模拟真实浏览器加载网页
- 执行 JavaScript 动态加载的内容
- 抓取所有网络请求的资源
- **自动提取 Source Maps 中的源代码文件**
- 按域名和路径组织文件树结构
- 页面自动滚动以触发懒加载资源
- 生成详细的抓取报告
- 支持 Cookie、自定义 Headers、代理
- 支持批量 URL 并发爬取

## 系统要求

1. **Go 1.19+** - 编译和运行程序
2. **Chrome 或 Chromium 浏览器** - chromedp 需要使用

### 安装 Chrome/Chromium

**macOS:**
```bash
brew install --cask google-chrome
# 或
brew install --cask chromium
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get update
sudo apt-get install chromium-browser
```

**Windows:**
从官网下载安装: https://www.google.com/chrome/

## 安装

```bash
# 克隆仓库
git clone https://github.com/3stoneBrother/spider.git
cd spider

# 编译
go build -o spider ./cmd/spider

# 或使用 Make
make build
```

## 使用方法

```
spider -url <目标URL> [选项]
spider -file <URL文件> [选项]
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-url` | 目标网页URL（与 -file 二选一） | - |
| `-file` | URL文件路径，每行一个URL（与 -url 二选一） | - |
| `-output` | 输出目录 | `./output` |
| `-timeout` | 爬取超时时间，单位秒 | `30` |
| `-cookie` | Cookie字符串，格式: `"key1=value1; key2=value2"` | - |
| `-header` | 自定义Header，格式: `"Key:Value"`（可多次使用） | - |
| `-proxy` | HTTP/SOCKS5代理地址，如 `"http://127.0.0.1:8080"` | - |
| `-ua` | 自定义 User-Agent | - |
| `-concurrency` | 并发数，批量爬取时生效 | `1` |
| `-headless` | 无头模式 | `true` |
| `-help` | 显示帮助信息 | - |

### 使用示例

```bash
# 基本使用
spider -url https://example.com

# 使用 Cookie
spider -url https://example.com -cookie "session=abc123; token=xyz"

# 自定义 Header
spider -url https://example.com -header "Authorization:Bearer token" -header "X-Custom:value"

# 使用代理
spider -url https://example.com -proxy http://127.0.0.1:8080

# 批量爬取
spider -file urls.txt -concurrency 3

# 可视化模式（调试）
spider -url https://example.com -headless=false

# 自定义输出目录和超时时间
spider -url https://example.com -output ./mysite -timeout 60
```

## 输出结构

```
output/
├── example.com/
│   ├── index.html
│   ├── index.html.meta
│   ├── css/
│   │   ├── style.css
│   │   └── style.css.meta
│   ├── js/
│   │   ├── app.js
│   │   └── app.js.meta
│   └── images/
│       └── logo.png
├── cdn.example.com/
│   └── library.js
└── report.txt
```

## Source Maps 支持

爬虫会自动检测 JavaScript 和 CSS 文件中的 Source Map 引用（`//# sourceMappingURL=...`）：

1. 下载对应的 `.map` 文件
2. 解析 Source Map JSON 格式
3. 提取所有原始源代码文件（包括 React 组件、模块等）
4. 保存完整的源代码文件树结构

Source Maps 输出示例：
```
output/
└── example.com/
    ├── src/
    │   ├── components/
    │   ├── modules/
    │   └── store/
    ├── static/
    │   ├── js/
    │   └── css/
    └── assets/
```

## 工作原理

1. **启动 Headless Chrome** - 使用 chromedp 启动无头浏览器
2. **页面加载与滚动** - 自动滚动页面以触发懒加载资源
3. **拦截网络请求** - 监听所有网络事件（EventResponseReceived）
4. **获取响应内容** - 通过 Chrome DevTools Protocol 获取响应体
5. **提取 Source Maps** - 自动检测并下载 .map 文件，提取原始源代码
6. **保存资源** - 按域名和路径结构保存到本地文件系统
7. **生成报告** - 统计并生成详细的抓取报告

## 项目结构

```
spider/
├── cmd/
│   └── spider/
│       └── main.go          # 主程序入口
├── internal/
│   ├── crawler/
│   │   ├── crawler.go       # 爬虫核心逻辑
│   │   └── config.go        # 配置定义
│   ├── storage/
│   │   └── storage.go       # 文件存储管理
│   └── sourcemap/
│       └── sourcemap.go     # Source Map 处理
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 注意事项

1. 确保系统中已安装 Chrome 或 Chromium 浏览器
2. 某些网站可能有反爬虫机制，请合理使用
3. 大型网站可能需要较长的加载时间，建议适当增加 timeout 值
4. 遵守目标网站的 robots.txt 和使用条款

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！
