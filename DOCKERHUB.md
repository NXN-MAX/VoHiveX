# VoHiveX

个人模组管理与测试平台，包含设备管理、短信、eSIM/eUICC、VoWiFi、定时任务和 Mihomo 代理管理。

**支持架构：Linux amd64。**

> 仅供个人自有设备和合法持有号码的内部测试。严禁商业用途、非本人个人号码接入及任何违法违规用途。

## 构建与启动

1. 解压完整的 VoHiveX 发布包，准备 `config/config.yaml`，设置独立登录密码。
2. 在 `.env` 中配置 `VOHIVE_BIND_IP` 和与宿主机一致的 `DRIVER_KERNEL`。
3. 从项目根目录执行：

```sh
docker compose -f docker-compose.single.yml up -d --build
```

构建使用 `Dockerfile.vohivex`、`release/` 下的定制版二进制及 `app/` 中的程序资源。不要用旧上游镜像代替当前定制构建。

## 访问与数据

浏览器访问 `http://<主机地址>:7575`，使用配置文件中设置的账号登录。默认监听地址为 `127.0.0.1`。

升级前停止容器并备份 `.env`、`config`、`data`、`logs` 与 `driver-state`，更新程序时保留这些目录。

## 内核版本配置

复制 `.env.example` 为 `.env`，在实际部署的 Linux 主机执行 `uname -r`，将结果填写到 `DRIVER_KERNEL`。此项不能为空：Compose 会在启动前检查；驱动入口还会再次核对运行中的内核版本，不一致时停止加载。不要填写构建机的内核版本；宿主机更新内核后应重新核对驱动兼容性。
