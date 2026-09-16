# 构建与部署

以下命令从项目根目录执行。Compose 项目标识为 `vohivex`（Compose 要求小写），容器名称为 `VoHiveX`。目标平台为 Linux amd64，使用 `Dockerfile.vohivex` 和 `docker-compose.single.yml`。

## 环境要求

- 前端构建机：Python 3、Node.js/npm；仅重新生成二进制补丁时需要 UPX。
- 运行主机：Docker Compose，以及与当前内核匹配的 `option`、`qmi_wwan` 和依赖模块。
- 模组接口：适配脚本为 `2ca3:4006` 注册运行时匹配，MI_02 用于 AT，MI_04 用于 QMI，不修改设备 USB ID。其他设备使用内核原有匹配表。
- 仅容器镜像安装依赖，不更新宿主机系统包。当前 Compose 未授予 `NET_ADMIN`，不提供移动数据拨号权限。

## 构建

```sh
npm ci --prefix app/build-tools
node app/build-tools/node_modules/terser/bin/terser app/frontend/Settings.js --module --compress --mangle -o app/frontend/Settings.min.js
python3 app/scheduler/build-assets.py
```

完整部署包的程序文件位于 `release/`。从 Git 获取的源码不包含生成文件，首次构建应先执行 `python3 app/prepare-build.py`。需要单独重建补丁时，准备上游原始文件并执行：

```sh
python3 app/patch-release.py --original /path/to/original-linux-amd64
```

`patch-release.py` 仅适用于 `patch-manifest.json` 指定且 SHA256 校验通过的上游二进制。不要对其他版本套用偏移补丁。修改前端后更新 `build-assets.py` 中的资源版本，重新生成资源。

## 配置与启动

1. 准备 `config/config.yaml`，设置独立的登录密码；首次使用可将设备列表设为 `devices: []`。
2. 在 `.env` 中设置 `VOHIVE_BIND_IP` 和 `DRIVER_KERNEL`。后者必须与宿主机 `uname -r` 一致，内核升级后需重新检查驱动兼容性。
3. 构建并启动容器：

```sh
docker compose -f docker-compose.single.yml up -d --build
```

浏览器访问 `http://<主机地址>:7575`。默认只绑定 `127.0.0.1`；容器内网关使用 `7576`，转发到原服务 `7575`。在设备管理中添加已发现的模块。

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

- `VoHiveX-deploy.zip`：包含生成的前端、Linux amd64 程序、Mihomo 与 Docker 构建输入，可直接构建镜像。
- `VoHiveX-single.zip`：在部署包基础上包含前端构建脚本、字体、图标、文档和截图；解压后可重新构建前端和再次打包。

两种包均保留 `Dockerfile`、`Dockerfile.vohivex`、`docker-compose.yml` 和 `docker-compose.single.yml`，附带 `SHA256SUMS`。不包含用户配置、短信数据、日志、订阅、依赖缓存或开发归档。项目目录名称可自行更改。

```sh
python3 app/verify-build.py
```

此命令检查本地镜像输入和二进制校验值，不代替实际的 Docker 构建。从 Git 拉取的源码不包含生成的二进制与前端资源。可在具备 Python 3、Node.js/npm、UPX 的构建机执行 `python3 app/prepare-build.py`：脚本下载固定版本上游文件并验证 SHA256，生成定制二进制和前端，准备 Mihomo。脚本不会安装系统包。GitHub Actions 在独立 CI 环境完成这些步骤并验证镜像构建，不自动推送镜像到外部仓库。

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

## 内核版本配置

复制 `.env.example` 为 `.env`，在实际部署的 Linux 主机执行 `uname -r`，将结果填写到 `DRIVER_KERNEL`。此项不能为空：Compose 会在启动前检查；驱动入口还会再次核对运行中的内核版本，不一致时停止加载。不要填写构建机的内核版本；宿主机更新内核后应重新核对驱动兼容性。
