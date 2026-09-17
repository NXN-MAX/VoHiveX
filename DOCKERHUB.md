# VoHiveX

原作者：[iniwex5](https://github.com/iniwex5) · VoHiveX 作者：[NXN-MAX](https://github.com/NXN-MAX)

个人模组管理与测试平台：设备监控、短信收发、定时短信、消息推送、eSIM、AT / USSD 终端、VoWiFi 及内置 Mihomo 代理管理。包含原厂 USB ID `2ca3:4006` 的 QMI 模组适配，无需修改 USB ID；功能可用性取决于设备、驱动与运营商支持。

## 默认访问

| 项目 | 默认值 |
| --- | --- |
| 网页端口 | `7575` |
| 用户名 | `admin` |
| 密码 | `admin` |

仅新安装生成默认账号，升级保留已有配置和密码。首次登录后请修改密码。

## 镜像与架构

- Docker Hub：`maxnxxn/vohivex:2.1.0`
- GHCR：`ghcr.io/nxn-max/vohivex:2.1.0`
- 通用标签：`2.1.0`、`v2.1.0`、`latest`，自动匹配 AMD64、ARM64、ARMv7。
- 独立标签：`2.1.0-amd64`、`2.1.0-arm64`、`2.1.0-aarch64`、`2.1.0-armv7`。

```sh
docker pull maxnxxn/vohivex:2.1.0
```

从 [GitHub 仓库](https://github.com/NXN-MAX/VoHiveX) 获取 `docker-compose.yml` 与 `.env.example`。将 Compose 的 `image` 改为 `maxnxxn/vohivex:2.1.0`，复制 `.env.example` 为 `.env`，填写监听地址 `VOHIVE_BIND_IP`（内核版本自动识别，无需填写），然后执行 `docker compose pull && docker compose up -d`。

需按 [部署说明](https://github.com/NXN-MAX/VoHiveX/blob/main/DEPLOY.md) 挂载配置、数据和设备；保留 `config`、`data`、`logs`、`driver-state`，不要将用户数据加入镜像。ARM 架构已通过模拟运行测试，USB 模组仍需实机验证。

## GitHub 自动构建与发布

GitHub Actions 在提交到 `main`、推送匹配版本标签或手动运行时构建三种架构。全部启动测试通过后发布 GHCR，再将同一提交的镜像同步到 Docker Hub。PR 仅测试，不发布。

一次性在 GitHub 仓库 Settings → Secrets and variables → Actions 中配置：

- Variables：`DOCKERHUB_USERNAME` = `maxnxxn`。
- Secrets：`DOCKERHUB_TOKEN` = Docker Hub 的 Read & Write 发布令牌（不需要 Delete 权限）。

未设置用户名变量时跳过 Docker Hub 发布；启用后令牌缺失或过期将明确使 Docker Hub 任务失败。令牌只保存在 GitHub 加密密钥中，不写入源码或镜像。无需付费 Docker Hub 自动构建服务，也无需另行授权 Docker Hub 读取 GitHub 源码。

## 使用限制

仅供个人自有设备和合法持有个人号码的内部测试。严禁商业用途、非本人个人号码接入及任何违法违规用途。许可和第三方权利说明见 [项目 LICENSE](https://github.com/NXN-MAX/VoHiveX/blob/main/LICENSE)。
