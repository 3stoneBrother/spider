# Spider - 浏览器模拟爬虫工具

基于 Go + chromedp（Chrome DevTools Protocol）开发的网页爬虫，驱动真实 Chrome/Chromium 引擎执行 JavaScript、触发懒加载，完整抓取动态页面的所有网络资源。

## 功能特性

- 真实浏览器驱动，完整执行 JavaScript
- 网络空闲检测（替代固定延迟），静态页 2s 退出，动态页自适应等待
- 分步滚动触发懒加载，每步独立容错不中断
- 自动提取 Source Maps 还原源代码
- 批量模式：浏览器池预热，并发爬取，按 hostname 分目录输出
- 失败自动重试（指数退避）
- 支持代理、自定义 Header / Cookie / User-Agent
- URL 文件输入自动去重（大小写、默认端口、末尾斜杠规范化）

---

## 系统要求

### 1. Chrome / Chromium 浏览器

Spider 通过 CDP 协议驱动本地 Chrome 进程，**必须预先安装**。

#### macOS

系统不自带 Chrome，但通常已手动安装。若未安装：

```bash
# Google Chrome（推荐）
brew install --cask google-chrome

# 或 Chromium（开源版）
brew install --cask chromium
```

验证是否可用：

```bash
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --version
```

#### Linux 服务器（无头模式，无需显示器）

headless 模式不需要 X11 / 桌面环境，但需要一些系统库。

**Ubuntu / Debian：**

```bash
sudo apt-get update
sudo apt-get install -y chromium-browser

# Ubuntu 22.04+ 包名改为 chromium
sudo apt-get install -y chromium
```

**CentOS / RHEL / Rocky Linux：**

```bash
sudo dnf install -y chromium
# 或旧版
sudo yum install -y chromium
```

**Alpine Linux（常用于 Docker）：**

```bash
apk add --no-cache chromium
```

**Kali / Parrot Linux：**

```bash
sudo apt-get install -y chromium
```

验证安装：

```bash
chromium-browser --version
# 或
chromium --version
# 或
google-chrome --version
```

#### Linux 常见依赖缺失问题

若运行时报 `error while loading shared libraries`，补装依赖：

```bash
# Ubuntu / Debian
sudo apt-get install -y \
  libnss3 libatk1.0-0 libatk-bridge2.0-0 \
  libcups2 libdrm2 libgbm1 libgtk-3-0 \
  libasound2 libxcomposite1 libxdamage1 libxrandr2
```

#### Docker 部署

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags "-s -w" -o spider ./cmd/spider

FROM alpine:latest
RUN apk add --no-cache chromium ca-certificates
WORKDIR /app
COPY --from=builder /app/spider .
ENTRYPOINT ["./spider"]
```

```bash
docker build -t spider .
docker run --rm -v $(pwd)/output:/app/output spider \
  -url https://example.com
```

> **注意**：Docker 容器中运行 Chrome 需要加 `--shm-size=2g` 防止 OOM：
> ```bash
> docker run --rm --shm-size=2g -v $(pwd)/output:/app/output spider -url https://example.com
> ```

---

### 2. Go 工具链（仅编译时需要）

运行预编译二进制无需 Go；自行编译需要 Go 1.21+。

```bash
go version  # >= go1.21
```

---

## 安装

### 方式一：使用预编译二进制（推荐）

从 `dist/` 目录下载对应平台的二进制，直接运行：

| 平台 | 文件名 |
|------|--------|
| Linux x86_64 | `spider_linux_amd64` |
| Linux ARM64 | `spider_linux_arm64` |
| macOS Intel | `spider_darwin_amd64` |
| macOS Apple Silicon | `spider_darwin_arm64` |
| Windows x86_64 | `spider_windows_amd64.exe` |

```bash
chmod +x spider_linux_amd64
./spider_linux_amd64 -help
```

### 方式二：从源码编译

```bash
# 编译当前平台
make build

# 编译所有平台到 ./dist/（含符号裁剪 -s -w）
make build-all

# 查看所有 make 目标
make help
```

---

## 使用方法

### 单 URL 爬取

```bash
./spider -url https://example.com
./spider -url https://example.com -output ./result -timeout 60
./spider -url https://example.com -proxy http://127.0.0.1:8080
./spider -url https://example.com -cookie "session=abc; token=xyz"
./spider -url https://example.com -header "Authorization:Bearer TOKEN" -header "X-Custom:value"
```

### 批量爬取（URL 文件）

```bash
# urls.txt 每行一个 URL，支持 # 注释，自动去重
./spider -file urls.txt -concurrency 3 -timeout 40

# 完整示例
./spider -file urls.txt \
  -concurrency 5 \
  -timeout 40 \
  -idle-timeout 10 \
  -retry 3 \
  -output ./output
```

### 所有参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-url` | 目标 URL（与 `-file` 二选一） | — |
| `-file` | URL 文件路径，每行一个（与 `-url` 二选一） | — |
| `-output` | 输出根目录 | `./output` |
| `-timeout` | 页面爬取超时，秒（不含浏览器启动时间） | `30` |
| `-idle-timeout` | 网络空闲等待上限，秒 | `10` |
| `-retry` | 失败重试次数，指数退避 | `2` |
| `-concurrency` | 并发数，批量模式同时运行的 Chrome 进程数 | `1` |
| `-cookie` | Cookie 字符串，格式 `key=val; key2=val2` | — |
| `-header` | 自定义请求头，格式 `Key:Value`（可多次使用） | — |
| `-proxy` | 代理地址，如 `http://127.0.0.1:8080` | — |
| `-ua` | 自定义 User-Agent | — |
| `-chrome-path` | Chrome/Chromium 可执行文件路径（默认自动搜索） | — |
| `-headless` | 无头模式 | `true` |
| `-help` | 显示帮助 | — |

---

## 输出结构

### 单 URL 模式

```
./output/
├── example.com/
│   ├── index.html
│   └── _nuxt/
│       ├── app.js
│       └── vendor.js
├── cdn.example.com/
│   └── vue.min.js
└── report.txt
```

### 批量模式（`-file`）

```
./output/
├── manifest.json           ← 每个 URL 的爬取结果（成败、目录、重试次数）
├── example.com/            ← 按 hostname 独立子目录
│   ├── index.html
│   └── ...
├── example.org/
│   └── ...
└── example.org_2/          ← 重复 hostname 自动加数字后缀
    └── ...
```

`manifest.json` 示例：

```json
[
  {
    "url": "https://example.com",
    "output_dir": "./output/example.com",
    "success": true,
    "attempts": 1
  },
  {
    "url": "https://example.org",
    "output_dir": "./output/example.org",
    "success": false,
    "error": "context deadline exceeded",
    "attempts": 3
  }
]
```

### URL 文件去重规则

以下 URL 视为同一目标，只保留第一条：

```
http://example.com
http://example.com/      ← 末尾斜杠
HTTP://EXAMPLE.COM       ← 大小写
http://example.com:80    ← 默认端口
```

`http://` 与 `https://` 视为不同 URL，不合并。

---

## 工作原理

```
启动 Chrome 进程（计时，不占用爬取超时）
    │
    ▼
打开 Tab，注入 network.Enable 监听所有响应
    │
    ▼
Navigate(URL) → 等待 document.readyState = complete
    │
    ▼
分 5 步滚动页面（25% → 50% → 75% → 100% → 回顶）
    │
    ▼
网络空闲检测：连续 2s 无新资源 → 退出（最多等 idle-timeout）
    │
    ▼
等待所有资源下载 goroutine 完成（wg.Wait）
    │
    ▼
提取 Source Maps → 解析 .map → 还原源文件
    │
    ▼
保存文件 + 生成 report.txt
```

批量模式在此基础上预启动 N 个 Chrome 进程（浏览器池），每个 URL 在独立进程中开新 Tab 爬取，Tab 关闭后浏览器进程归还池中复用，避免重复冷启动。

---

## 常见问题

**Q: `chrome failed to start` / `exec: "chromium-browser": executable file not found`**

A: 未安装 Chrome/Chromium，参考上方「安装」章节。

**Q: Linux 上报 `error while loading shared libraries`**

A: 缺少系统依赖，运行：
```bash
sudo apt-get install -y libnss3 libgbm1 libgtk-3-0 libasound2
```

**Q: 抓取资源不完整**

A: 增大 `-timeout` 和 `-idle-timeout`，给动态内容更多加载时间。

**Q: Docker 容器中崩溃 / OOM**

A: 加 `--shm-size=2g`，Chrome 默认使用 `/dev/shm` 作为共享内存。

**Q: 服务器并发多少合适**

A: 每个 Chrome 实例约占 5-8 个系统进程、200-400MB 内存。`-concurrency 5` 约消耗 2GB 内存，请根据服务器配置调整。
