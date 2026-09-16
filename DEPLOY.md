# VoHiveX 部署包

支持 Linux amd64。部署包包含定制二进制、已生成的前端、Mihomo 和 Docker 构建文件，不需要在部署主机安装 Node.js 或重新构建前端。

## 检查与启动

1. 解压后进入 `VoHiveX` 目录，执行 `sha256sum -c SHA256SUMS` 核对文件完整性；目录可自行重命名。
2. 宿主机应已提供与当前内核匹配的 `option`、`qmi_wwan` 及依赖模块，并已安装 Docker Compose。
3. 准备 `config/config.yaml`，设置独立的登录密码。在 `.env` 中设置 `VOHIVE_BIND_IP` 和 `DRIVER_KERNEL`；内核版本应与宿主机 `uname -r` 的结果一致。
4. 执行以下命令：

```sh
docker compose config --quiet
docker compose build
docker compose up -d
```

浏览器访问 `http://<主机地址>:7575`。默认仅绑定 `127.0.0.1`，局域网访问需设置主机绑定地址。

`Dockerfile` 与 `Dockerfile.vohivex` 内容一致；两份 Compose 文件也一致。可选执行 `python3 app/verify-build.py` 检查构建输入；这不代替实际构建镜像和设备验证。

## 更新

停止容器并备份原部署目录中的 `.env`、`config`、`data`、`logs`、`driver-state` 及部署配置。更新时保留这些文件；发布包不包含个人配置和数据。重新构建并启动后，检查容器健康状态及设备在线状态。

## 完整构建包

`VoHiveX-single.zip` 额外包含前端构建脚本、字体、图标与文档。在安装 Python 3、Node.js/npm 的开发机中，可执行：

```sh
npm ci --prefix app/build-tools
python3 app/scheduler/build-assets.py
python3 app/package-single.py
```

输出位于 `dist/`。部署包 `VoHiveX-deploy.zip` 只提供镜像构建输入，不提供重新生成前端和再次打包的工具。

## 内核版本配置

复制 `.env.example` 为 `.env`，在实际部署的 Linux 主机执行 `uname -r`，将结果填写到 `DRIVER_KERNEL`。此项不能为空：Compose 会在启动前检查；驱动入口还会再次核对运行中的内核版本，不一致时停止加载。不要填写构建机的内核版本；宿主机更新内核后应重新核对驱动兼容性。
