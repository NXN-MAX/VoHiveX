# 品牌资源

| 资源 | 规格 |
| --- | --- |
| VoHiveX 字标 | Jost 800 Italic，X 为 `#9FE870` |
| 网页图标 | `#9FE870` 圆角方形底，黑色 VoX |
| `vohivex-logo.png` | 1024 × 1024 |
| `vohivex-icon.png` | 512 × 512 |
| `vohivex-favicon.ico` | 16 / 24 / 32 / 48 / 64 / 128 / 256 px |
| `docs/images/vohivex-banner.png` | 1440 × 360，圆角绿色底，X 为对比色 `#397A16` |

从项目根目录执行，Python 环境需提供 Pillow：

```sh
python3 app/branding/build-icon.py
python3 app/branding/build-banner.py
```

保留字体文件、`OFL.txt` 与 `manifest.json`。更新字体时同步核对来源、许可及文件校验值。
