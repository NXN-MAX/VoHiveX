# 构建与部署

以下命令从项目根目录执行。Compose 项目标识为 `vohivex`（Compose 要求小写），容器名称为 `VoHiveX`。目标平台为 Linux AMD64、ARM64、ARMv7，使用 `Dockerfile.vohivex` 和 `docker-compose.single.yml`。

## 环境要求

- 前端构建机：Python 3、Node.js/npm；仅重新生成二进制补丁时需要 UPX。
- 运行主机：Docker Compose，以及与当前内核匹配的 `option`、`qmi_wwan` 和依赖模块。
- 模组接口：适配脚本为 `2ca3:4006` 注册运行时匹配，MI_02 用于 AT，MI_04 用于 QMI，不修改设备 USB ID。其他设备使用内核原有匹配表。
- 仅容器镜像安装依赖，不更新宿主机系统包。当前 Compose 未授予 `NET_ADMIN`，不提供移动数据拨号权限。

## 构建

镜像由 GitHub Actions 按 `linux/amd64`、`linux/arm64`、`linux/arm/v7` 分别构建，运行启动测试后发布到 `ghcr.io/nxn-max/vohivex`。拉取 `2.1.3` 或 `latest` 时会自动选择架构。

从源码首次构建时，在构建机准备 Go 1.24+、Python 3、Node.js/npm 和 UPX，然后运行：

```sh
python3 app/prepare-build.py
python3 app/verify-build.py
docker build -f Dockerfile.vohivex -t vohivex:2.1.3 .
```

可用 `--arch amd64`、`--arch arm64` 或 `--arch armv7` 仅准备所需架构。前端由 `web/` 中的 Vue 3 + Vite + Pinia + Antdv Next 工程统一构建；三种架构使用独立的版本锁定清单与 SHA256，不对其他上游版本套用偏移补丁。`patch-release.py --arch <架构> --original /path/to/original` 可单独重建对应补丁。

## 配置与启动

1. 复制 `.env.example` 为 `.env`，设置 `VOHIVE_BIND_IP`；内核版本自动识别。
2. 执行：

```sh
docker compose -f docker-compose.single.yml pull
docker compose -f docker-compose.single.yml up -d
```

默认网页端口为 `7575`，默认账号和密码均为 `admin`。首次启动会创建 `config/config.yaml`；已有账号、密码、设备和代理配置保留。旧配置的 `server.port` 自动迁移为 `127.0.0.1:7576`，只供容器内网关访问；对外端口和健康检查均使用 `7575`。

默认只绑定主机 `127.0.0.1`；局域网访问应在 `.env` 中配置主机的局域网地址。内核升级后重新核对驱动兼容性并更新 `.env`。

## 运行检查

```sh
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh inspect
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh health
docker compose -f docker-compose.single.yml logs vohive
```

健康检查涵盖驱动绑定、设备节点及网页服务；设备实际在线状态需在界面确认。任一必要进程退出时，由 Docker 重启策略恢复容器。

## 更新与备份

更新前停止容器并备份 `.env`、`config`、`data`、`logs`、`driver-state` 及当前部署配置。替换程序文件后重新构建，不覆盖持久化目录。

```sh
python3 app/package-single.py
```

发布包输出到 `dist/`，可用 `--output-dir /path/to/output` 指定其他目录：

- `VoHiveX-deploy.zip`：包含生成的前端、三种架构的 Linux 程序、Mihomo 与 Docker 构建输入，可直接构建镜像。
- `VoHiveX-single.zip`：在部署包基础上包含前端构建脚本、字体、图标、文档和截图；解压后可重新构建前端和再次打包。

两种包均保留 `Dockerfile`、`Dockerfile.vohivex`、`docker-compose.yml` 和 `docker-compose.single.yml`，附带 `SHA256SUMS`。不包含用户配置、短信数据、日志、订阅、依赖缓存或开发归档。项目目录名称可自行更改。

```sh
python3 app/verify-build.py
```

此命令检查本地镜像输入和二进制校验值，不代替实际的 Docker 构建。从 Git 拉取的源码不包含生成的二进制与前端资源。可在具备 Python 3、Node.js/npm、UPX 的构建机执行 `python3 app/prepare-build.py`：脚本下载固定版本上游文件并验证 SHA256，生成定制二进制和前端，准备 Mihomo。脚本不会安装系统包。GitHub Actions 在独立 CI 环境完成这些步骤、逐架构启动测试并发布到 GitHub Container Registry；PR 不发布镜像。

## 停止与驱动撤销

```sh
docker compose -f docker-compose.single.yml stop vohive
docker compose -f docker-compose.single.yml run --rm --no-deps --entrypoint /bin/sh vohive /opt/vohivex/driver.sh cleanup
docker compose -f docker-compose.single.yml down
```

停止容器不会立即解绑设备。`cleanup` 只撤销本项目登记的匹配和绑定，并尝试安全卸载其加载的模块；正在被其他设备使用的模块不会强制卸载。若 `option1` 不支持 `remove_id`，动态匹配需安全卸载模块或重启宿主机后清除。撤销完成前保留 `driver-state`。

## 模块说明

- [定时短信](scheduler/README.md)
- [代理管理](proxy/README.md)
- [API 文档](docs/README.md)
- [品牌资源](branding/README.md)
- [第三方组件与许可](proxy/THIRD-PARTY.md)

## 自动识别内核

容器启动时自动识别宿主机当前运行的内核，无需填写或指定内核版本。已加载的驱动直接复用；未加载时从挂载的 `/lib/modules/<当前内核版本>` 加载 `option`、`qmi_wwan` 及依赖。目录缺失或加载失败会报告错误并停止初始化，不安装驱动包、不替换内核。升级宿主机内核后，需确保宿主机提供对应驱动。
