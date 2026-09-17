# API 文档

## 使用

1. 登录管理界面。
2. 打开「系统设置 → 系统信息 → 查看本机接口说明」，或访问 `/api/docs`。
3. 按接口分组查看请求、响应和 Schema；需要调试时使用 Authorize 与 Try it out。

规格地址为 `/api/openapi.json`，需要 Bearer 鉴权。登录过期后应重新登录。请求仅允许当前同源服务，不使用在线规格校验或 URL 配置覆盖。

## 维护

- 页面：`app/docs/index.html`。
- Swagger UI：`app/docs/vendor/swagger-ui-5.32.15/`。
- 资源完整性记录：同目录的 `manifest.json`。
- 后台主题：`app/frontend/vohivex-theme.css`。

```sh
python3 app/scheduler/build-assets.py
```

修改资源后提高构建脚本中的资源版本，重新生成前端并构建镜像。资源由本机提供，许可证、NOTICE 和完整性记录必须随资源一并打包。
