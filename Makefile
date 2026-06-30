.PHONY: build build-all clean test run fmt lint install help \
        build-linux-amd64 build-linux-arm64 \
        build-darwin-amd64 build-darwin-arm64 \
        build-windows-amd64

.DEFAULT_GOAL := help

BINARY_NAME  := spider
CMD_DIR      := ./cmd/spider
DIST_DIR     := ./dist

# -s: 去掉符号表  -w: 去掉 DWARF 调试信息，减小体积、防止逆向
LDFLAGS := -ldflags "-s -w"

# ─── 本机构建 ──────────────────────────────────────────────────────────────────

build: ## 编译当前平台二进制（含符号裁剪）
	@echo "编译 $(BINARY_NAME) ..."
	go build $(LDFLAGS) -o $(BINARY_NAME) $(CMD_DIR)
	@echo "完成: ./$(BINARY_NAME)"

# ─── 多平台构建 ────────────────────────────────────────────────────────────────

build-linux-amd64: ## Linux x86_64
	@mkdir -p $(DIST_DIR)
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)_linux_amd64   $(CMD_DIR)
	@echo "  ✓ $(DIST_DIR)/$(BINARY_NAME)_linux_amd64"

build-linux-arm64: ## Linux ARM64（服务器/树莓派）
	@mkdir -p $(DIST_DIR)
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)_linux_arm64   $(CMD_DIR)
	@echo "  ✓ $(DIST_DIR)/$(BINARY_NAME)_linux_arm64"

build-darwin-amd64: ## macOS Intel
	@mkdir -p $(DIST_DIR)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)_darwin_amd64  $(CMD_DIR)
	@echo "  ✓ $(DIST_DIR)/$(BINARY_NAME)_darwin_amd64"

build-darwin-arm64: ## macOS Apple Silicon（M 系列）
	@mkdir -p $(DIST_DIR)
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)_darwin_arm64  $(CMD_DIR)
	@echo "  ✓ $(DIST_DIR)/$(BINARY_NAME)_darwin_arm64"

build-windows-amd64: ## Windows x86_64
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)_windows_amd64.exe $(CMD_DIR)
	@echo "  ✓ $(DIST_DIR)/$(BINARY_NAME)_windows_amd64.exe"

build-all: ## 编译所有平台（输出到 ./dist/）
	@echo "多平台编译..."
	@$(MAKE) build-linux-amd64
	@$(MAKE) build-linux-arm64
	@$(MAKE) build-darwin-amd64
	@$(MAKE) build-darwin-arm64
	@$(MAKE) build-windows-amd64
	@echo ""
	@echo "全部完成，输出目录: $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

# ─── 工具目标 ──────────────────────────────────────────────────────────────────

clean: ## 清理编译产物和输出目录
	rm -f ./$(BINARY_NAME)
	rm -rf $(DIST_DIR) ./output
	@echo "清理完成"

deps: ## 更新依赖
	go mod tidy && go mod download
	@echo "依赖更新完成"

run: build ## 本机构建后打印帮助
	./$(BINARY_NAME) -help

test: ## 运行测试
	go test -v ./...

fmt: ## 格式化代码
	go fmt ./...

lint: ## 代码检查（需要 golangci-lint）
	@which golangci-lint > /dev/null || (echo "请先安装 golangci-lint" && exit 1)
	golangci-lint run

install: ## 安装到 GOPATH/bin
	go install $(LDFLAGS) $(CMD_DIR)

help: ## 显示帮助信息
	@echo "用法: make <目标>"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
