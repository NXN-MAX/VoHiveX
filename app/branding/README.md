# Brand Assets

| Asset | Specification |
| --- | --- |
| VoHiveX wordmark | Jost 800 Italic with the X in `#9FE870` |
| Web icon | Black VoX on a `#9FE870` rounded-square background |
| `vohivex-logo.png` | 1024 × 1024 |
| `vohivex-icon.png` | 512 × 512 |
| `vohivex-favicon.ico` | 16 / 24 / 32 / 48 / 64 / 128 / 256 px |
| `docs/images/vohivex-banner.png` | 1440 × 360, rounded green background with the X in contrasting `#397A16` |

Run these commands from the repository root. Pillow must be available in the Python environment:

```sh
python3 app/branding/build-icon.py
python3 app/branding/build-banner.py
```

Retain the font file, `OFL.txt`, and `manifest.json`. When updating the font, verify the source, license, and file checksums together.
