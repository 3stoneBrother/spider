.PHONY: build clean test run help

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
BINARY_NAME=spider
BUILD_DIR=.
CMD_DIR=./cmd/spider

# 构建
build: ## 编译项目
	@echo "正在编译..."
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "编译完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 清理
clean: ## 清理编译产物和输出目录
	@echo "清理中..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -rf ./output
	@echo "清理完成"

# 安装依赖
deps: ## 安装/更新依赖
	@echo "安装依赖..."
	go mod tidy
	go mod download
	@echo "依赖安装完成"

# 运行（需要提供URL参数）
run: build ## 运行程序（示例）
	@echo "运行示例..."
	./$(BINARY_NAME) -help

# 测试
test: ## 运行测试
	go test -v ./...

# 格式化代码
fmt: ## 格式化代码
	go fmt ./...

# 代码检查
lint: ## 代码检查
	@which golangci-lint > /dev/null || (echo "请先安装 golangci-lint" && exit 1)
	golangci-lint run

# 安装
install: build ## 安装到 GOPATH/bin
	go install $(CMD_DIR)

# 帮助
help: ## 显示帮助信息
	@echo "可用命令:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
