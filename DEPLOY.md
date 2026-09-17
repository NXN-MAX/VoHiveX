# VoHiveX 部署

支持 Linux AMD64、ARM64/AArch64、ARMv7。推荐直接拉取 `ghcr.io/nxn-max/vohivex:2.1.0`；Docker 自动选择对应架构。另提供 386 Go 网关二进制，但不包含 386 调制解调器核心。

## 默认访问

- 网页端口：`7575`（容器端口也为 `7575`）。
- 默认账号：`admin`。
- 默认密码：`admin`。

只在配置不存在时创建默认账号密码，已有配置不会重置。首次登录后修改密码。

## 拉取与启动

1. 宿主机应已提供与当前内核匹配的 `option`、`qmi_wwan` 及依赖模块，并已安装 Docker Compose。
2. 将 `docker-compose.yml` 与 `.env.example` 放在同一目录，复制 `.env.example` 为 `.env`。
3. 内核版本自动识别，无需手动配置。未加载的驱动从当前内核对应目录加载；目录缺失或加载失败时停止初始化并记录错误。
4. 设置 `VOHIVE_BIND_IP`。默认 `127.0.0.1` 仅允许本机访问，局域网访问需填写主机的局域网地址。
5. 执行：

```sh
docker compose config --quiet
docker compose pull
docker compose up -d
```

访问 `http://<主机地址>:7575`。初始化自动生成最小配置，无需手动准备账号密码。

## 更新与数据

更新前停止容器并备份 `.env`、`config`、`data`、`logs`、`driver-state` 及部署配置。更新不覆盖用户数据。旧配置只将核心服务端口迁移为 `127.0.0.1:7576`，账号密码和其他设置保留；旧 Compose 的网页映射及健康检查需要同步改为容器端口 `7575`。

## 从源码或部署包构建

源码构建机需要 Go 1.24+、Python 3、Node.js/npm、UPX；运行 `python3 app/prepare-build.py` 后执行 `docker build -t vohivex:2.1.0 .`。完整部署包已包含三种架构程序与生成的前端，可直接构建，无需在部署主机安装前端依赖。

发布包附带 `SHA256SUMS`，解压后先执行 `sha256sum -c SHA256SUMS`。`python3 app/package-single.py` 生成 `dist/VoHiveX-deploy.zip` 与包含构建源码的 `dist/VoHiveX-single.zip`。

Compose 默认从 GHCR 拉取已发布镜像。本地构建镜像如需用于部署，应将 Compose 的 `image` 改为本地标签，并将 `pull_policy` 改为 `never`。
