# 第三方组件与许可

更新或分发组件时，保留许可证、版权声明、NOTICE 和来源校验记录。不要将组件许可改为本项目的使用条款。

| 组件 | 版本 / 来源 | 许可与本地文件 |
| --- | --- | --- |
| Mihomo | [v1.19.31](https://github.com/MetaCubeX/mihomo/tree/v1.19.31) · [对应源代码归档](https://github.com/MetaCubeX/mihomo/archive/refs/tags/v1.19.31.tar.gz) | GPL-3.0；`app/proxy/vendor/LICENSE.mihomo`；下载与程序哈希见 `app/proxy/vendor/manifest.json` |
| Antdv Next | [1.5.4](https://github.com/antdv-next/antdv-next) | MIT；前端依赖锁定见 `web/package-lock.json` |
| jsQR | [1.4.0](https://github.com/cozmo/jsQR/tree/v1.4.0) | Apache-2.0；Copyright (c) 2017 Cosmo Wolfe；`app/proxy/vendor/LICENSE.jsQR`；依赖锁定见 `web/package-lock.json` |
| Swagger UI | [5.32.15](https://github.com/swagger-api/swagger-ui/tree/v5.32.15) | Apache-2.0；许可、NOTICE 和包校验记录见 `app/docs/vendor/swagger-ui-5.32.15/` |
| Remix Icon | [固定源码提交](https://github.com/Remix-Design/RemixIcon/tree/9fb7967c0a4c09910161192bde99efd3df09f5eb) | Remix Icon License v1.0；Copyright (c) 2017–2026 Remix Design；SVG 与许可见 `web/public/icons/` 和 `web/public/remixicon-LICENSE.txt` |
| Jost Italic | [Google Fonts](https://github.com/google/fonts/tree/main/ofl/jost) · [上游项目](https://github.com/indestructible-type/Jost) | SIL Open Font License 1.1；Copyright 2020 The Jost Project Authors；`app/branding/OFL.txt` 与 `app/branding/manifest.json` |

Mihomo 作为独立子进程运行。前端组件、字体及图标随构建资源提供；分发时应同时提供相应许可文件。Swagger UI 的许可证与 NOTICE 随 `/api/docs` 资源提供，Remix Icon 和 Jost 的许可分别提供于 `/assets/remixicon-LICENSE.txt` 和 `/assets/Jost-OFL.txt`。
