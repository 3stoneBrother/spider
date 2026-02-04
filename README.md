# Spider - 浏览器模拟爬虫工具

一个基于 Go 语言和 chromedp 开发的网页爬虫工具，可以模拟真实浏览器加载网页，执行 JavaScript 代码，并抓取所有动态加载的资源。

## 功能特性

- ✅ 模拟真实浏览器行为
- ✅ 执行 JavaScript 动态加载的内容
- ✅ 拦截并保存所有网络请求的资源
- ✅ **自动提取 Source Maps 中的源代码文件**
- ✅ 按域名和路径组织文件树结构（类似 Chrome DevTools）
- ✅ 页面自动滚动以触发懒加载资源
- ✅ 生成详细的抓取报告
- ✅ 支持自定义超时时间
- ✅ 保存资源元数据信息

## 系统要求

### 必需组件

1. **Go 1.19+** - 编译和运行程序
2. **Chrome 或 Chromium 浏览器** - chromedp 需要使用

### 安装 Chrome/Chromium

#### macOS
```bash
# 使用 Homebrew 安装
brew install --cask google-chrome

# 或安装 Chromium
brew install --cask chromium
```

#### Linux (Ubuntu/Debian)
```bash
# 安装 Chromium
sudo apt-get update
sudo apt-get install chromium-browser

# 或安装 Chrome
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo dpkg -i google-chrome-stable_current_amd64.deb
```

#### Windows
从官网下载安装: https://www.google.com/chrome/

## 项目结构

```
spider/
├── cmd/
│   └── spider/
│       └── main.go          # 主程序入口
├── internal/
│   ├── crawler/
│   │   └── crawler.go       # 爬虫核心逻辑
│   ├── storage/
│   │   └── storage.go       # 文件存储管理
│   └── sourcemap/
│       └── sourcemap.go     # Source Map 处理
├── go.mod
├── go.sum
└── README.md
```

## 安装和构建

### 使用 Make（推荐）

```bash
# 查看可用命令
make help

# 安装依赖
make deps

# 编译程序
make build

# 清理
make clean
```

### 手动构建

```bash
# 克隆或下载项目
cd spider

# 下载依赖
go mod tidy

# 编译程序
go build -o spider ./cmd/spider

# 运行
./spider -help
```

## 使用方法

### 基本用法

```bash
# 爬取指定网页
./spider -url https://example.com

# 指定输出目录
./spider -url https://example.com -output ./mysite

# 设置超时时间（秒）
./spider -url https://example.com -timeout 60
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| -url | 目标网页URL（必需） | - |
| -output | 输出目录 | ./output |
| -timeout | 爬取超时时间（秒） | 30 |
| -help | 显示帮助信息 | - |

### 使用示例

```bash
# 爬取一个简单网页
./spider -url https://example.com

# 爬取一个复杂的单页应用
./spider -url https://react-app.com -timeout 60

# 爬取并保存到指定目录
./spider -url https://mysite.com -output ./downloaded/mysite
```

## 输出结构

抓取完成后，资源会按以下结构保存：

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
│       ├── logo.png
│       └── logo.png.meta
├── cdn.example.com/
│   └── library.js
└── report.txt
```

- 每个资源文件旁边会有一个 `.meta` 文件，包含该资源的元数据
- `report.txt` 包含完整的抓取报告

## 工作原理

1. **启动 Headless Chrome**: 使用 chromedp 启动无头浏览器
2. **页面加载与滚动**: 自动滚动页面以触发懒加载资源
3. **拦截网络请求**: 监听所有网络事件（EventResponseReceived）
4. **获取响应内容**: 通过 Chrome DevTools Protocol 获取响应体
5. **提取 Source Maps**: 自动检测并下载 .map 文件，提取原始源代码
6. **保存资源**: 按域名和路径结构保存到本地文件系统
7. **生成报告**: 统计并生成详细的抓取报告

## Source Maps 支持

爬虫会自动检测 JavaScript 和 CSS 文件中的 Source Map 引用（`//# sourceMappingURL=...`），并：

1. 下载对应的 `.map` 文件
2. 解析 Source Map JSON 格式
3. 提取所有原始源代码文件（包括 React 组件、模块等）
4. 保存完整的源代码文件树结构（src/, components/, modules/ 等）

这样你可以获得：
- 完整的源代码文件（.js, .jsx, .ts, .tsx 等）
- 样式源文件（.scss, .sass, .less 等）
- 完整的项目目录结构

示例输出结构：
```
output/
└── example.com/
    ├── src/
    │   ├── components/
    │   ├── modules/
    │   ├── store/
    │   └── routes/
    ├── static/
    │   ├── js/
    │   └── css/
    └── assets/
```

## 注意事项

1. 确保系统中已安装 Chrome 或 Chromium 浏览器
2. 某些网站可能有反爬虫机制，请合理使用
3. 大型网站可能需要较长的加载时间，建议适当增加 timeout 值
4. 程序会等待额外时间以确保异步资源加载完成
5. 遵守目标网站的 robots.txt 和使用条款

## 技术栈

- **Go**: 主要编程语言
- **chromedp**: Chrome DevTools Protocol 的 Go 实现
- **net/http**: HTTP 客户端（备用下载）

## 常见问题

### Q: 提示 "chrome failed to start"
A: 请确保系统中已安装 Chrome 或 Chromium 浏览器。

### Q: 抓取的资源不完整
A: 可以尝试增加 timeout 值，给予网页更多加载时间。

### Q: 某些资源下载失败
A: 这是正常现象，某些资源可能因为跨域、权限等原因无法获取。程序会继续处理其他资源。

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！
