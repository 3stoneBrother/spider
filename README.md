# Spider - Browser-Based Web Crawler

[中文文档](README_CN.md)

A web crawler tool built with Go and chromedp that simulates real browser behavior, executes JavaScript, and captures all dynamically loaded resources.

## Features

- Simulates real browser behavior
- Executes JavaScript and captures dynamically loaded content
- Intercepts and saves all network request resources
- **Automatically extracts source code from Source Maps**
- Organizes files by domain and path (like Chrome DevTools)
- Auto-scrolls pages to trigger lazy-loaded resources
- Generates detailed crawl reports
- Supports Cookie, custom Headers, and proxy
- Supports batch URL crawling with concurrency

## Requirements

1. **Go 1.19+** - For building and running
2. **Chrome or Chromium** - Required by chromedp

### Install Chrome/Chromium

**macOS:**
```bash
brew install --cask google-chrome
# or
brew install --cask chromium
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get update
sudo apt-get install chromium-browser
```

**Windows:**
Download from: https://www.google.com/chrome/

## Installation

```bash
# Clone the repository
git clone https://github.com/3stoneBrother/spider.git
cd spider

# Build
go build -o spider ./cmd/spider

# Or use Make
make build
```

## Usage

```
spider -url <target-url> [options]
spider -file <url-file> [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-url` | Target URL (mutually exclusive with -file) | - |
| `-file` | File containing URLs, one per line (mutually exclusive with -url) | - |
| `-output` | Output directory | `./output` |
| `-timeout` | Crawl timeout in seconds | `30` |
| `-cookie` | Cookie string, format: `"key1=value1; key2=value2"` | - |
| `-header` | Custom header, format: `"Key:Value"` (can be used multiple times) | - |
| `-proxy` | HTTP/SOCKS5 proxy address, e.g., `"http://127.0.0.1:8080"` | - |
| `-ua` | Custom User-Agent | - |
| `-concurrency` | Concurrency level for batch crawling | `1` |
| `-headless` | Headless mode | `true` |
| `-help` | Show help message | - |

### Examples

```bash
# Basic usage
spider -url https://example.com

# With cookies
spider -url https://example.com -cookie "session=abc123; token=xyz"

# Custom headers
spider -url https://example.com -header "Authorization:Bearer token" -header "X-Custom:value"

# Using proxy
spider -url https://example.com -proxy http://127.0.0.1:8080

# Batch crawling
spider -file urls.txt -concurrency 3

# Visual mode (for debugging)
spider -url https://example.com -headless=false

# Custom output directory and timeout
spider -url https://example.com -output ./mysite -timeout 60
```

## Output Structure

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

## Source Maps Support

The crawler automatically detects Source Map references (`//# sourceMappingURL=...`) in JavaScript and CSS files:

1. Downloads the corresponding `.map` files
2. Parses Source Map JSON format
3. Extracts all original source files (React components, modules, etc.)
4. Preserves the complete source code directory structure

Output example with Source Maps:
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

## How It Works

1. **Launch Headless Chrome** - Uses chromedp to start a headless browser
2. **Page Loading & Scrolling** - Auto-scrolls to trigger lazy-loaded resources
3. **Network Interception** - Monitors all network events (EventResponseReceived)
4. **Response Capture** - Gets response bodies via Chrome DevTools Protocol
5. **Source Map Extraction** - Detects and downloads .map files, extracts source code
6. **Resource Storage** - Saves to local filesystem organized by domain and path
7. **Report Generation** - Creates detailed crawl statistics report

## Project Structure

```
spider/
├── cmd/
│   └── spider/
│       └── main.go          # Entry point
├── internal/
│   ├── crawler/
│   │   ├── crawler.go       # Core crawler logic
│   │   └── config.go        # Configuration
│   ├── storage/
│   │   └── storage.go       # File storage management
│   └── sourcemap/
│       └── sourcemap.go     # Source Map processing
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Notes

1. Ensure Chrome or Chromium is installed on your system
2. Some websites may have anti-crawler mechanisms, use responsibly
3. Large websites may require longer timeout values
4. Respect target website's robots.txt and terms of service

## License

MIT License

## Contributing

Issues and Pull Requests are welcome!
