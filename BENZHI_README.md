# BENZHI_README

基于 Go 实现的舞台机械安全启用核验台 Web 项目，一款后端服务，舞台机械安全启用核验台提供演出前核验、风险整改、复核冻结、启用许可签发和审计追溯的一体化浏览器工作台。

## 项目说明
- 项目：benzhi-project-9a79febb-d1ff-4975-8457-92e8e150b15f
- 项目用途：舞台机械安全启用核验台提供演出前核验、风险整改、复核冻结、启用许可签发和审计追溯的一体化浏览器工作台。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/stageguard -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-9a79febb-d1ff-4975-8457-92e8e150b15f-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-9a79febb-d1ff-4975-8457-92e8e150b15f-arm64 linux/arm64
docker run -it benzhi-project-9a79febb-d1ff-4975-8457-92e8e150b15f-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/stageguard -selfcheck -addr=127.0.0.1:19081`
