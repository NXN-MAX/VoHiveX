# VoHiveX Docker 镜像

官方构建位置：`ghcr.io/nxn-max/vohivex`（GitHub Container Registry）。本项目目前不向 Docker Hub 发布镜像。

支持 AMD64、ARM64、ARMv7；标签 `2.0.0`、`v2.0.0` 和 `latest` 自动选择架构，也提供 `2.0.0-amd64`、`2.0.0-arm64`、`2.0.0-armv7`。

- 默认网页端口：`7575`。
- 默认账号：`admin`。
- 默认密码：`admin`。
- 仅首次安装生成默认配置，已有账号密码不会重置。

下载 `docker-compose.yml` 与 `.env.example`，复制后者为 `.env`，填写部署主机内核版本 `DRIVER_KERNEL` 和监听地址 `VOHIVE_BIND_IP`，执行 `docker compose pull && docker compose up -d`。

详细步骤见 [部署说明](DEPLOY.md)。保留 `.env`、`config`、`data`、`logs`、`driver-state`，不要将用户数据加入镜像。

仅供个人自有设备和合法持有号码的内部测试。严禁商业用途、非本人个人号码接入及任何违法违规用途。
