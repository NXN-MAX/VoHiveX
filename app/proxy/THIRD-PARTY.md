# Third-Party Components and Licenses

When updating or distributing these components, retain their licenses, copyright notices, NOTICE files, and source integrity records. Do not replace a component's license with the VoHiveX usage terms.

| Component | Version / source | License and local files |
| --- | --- | --- |
| Mihomo | [v1.19.31](https://github.com/MetaCubeX/mihomo/tree/v1.19.31) · [corresponding source archive](https://github.com/MetaCubeX/mihomo/archive/refs/tags/v1.19.31.tar.gz) | GPL-3.0; `app/proxy/vendor/LICENSE.mihomo`; download and binary hashes are recorded in `app/proxy/vendor/manifest.json` |
| Antdv Next | [1.5.4](https://github.com/antdv-next/antdv-next) | MIT; frontend dependency lock in `web/package-lock.json` |
| jsQR | [1.4.0](https://github.com/cozmo/jsQR/tree/1.4.0) | Apache-2.0; Copyright (c) 2017 Cosmo Wolfe; `app/proxy/vendor/LICENSE.jsQR`; dependency lock in `web/package-lock.json` |
| Swagger UI | [5.32.15](https://github.com/swagger-api/swagger-ui/tree/v5.32.15) | Apache-2.0; license, NOTICE, and package integrity records in `app/docs/vendor/swagger-ui-5.32.15/` |
| Remix Icon | [pinned source commit](https://github.com/Remix-Design/RemixIcon/tree/9fb7967c0a4c09910161192bde99efd3df09f5eb) | Remix Icon License v1.0; Copyright (c) 2017–2026 Remix Design; SVG files and license in `web/public/icons/` and `web/public/remixicon-LICENSE.txt` |
| Jost Italic | [Google Fonts](https://github.com/google/fonts/tree/main/ofl/jost) · [upstream project](https://github.com/indestructible-type/Jost) | SIL Open Font License 1.1; Copyright 2020 The Jost Project Authors; `app/branding/OFL.txt` and `app/branding/manifest.json` |

Mihomo runs as a separate subprocess. Frontend components, fonts, and icons are included in built assets and must be distributed with their corresponding license files. The Swagger UI license and NOTICE are provided with the `/api/docs` assets. The Remix Icon and Jost licenses are available at `/assets/remixicon-LICENSE.txt` and `/assets/Jost-OFL.txt`, respectively.
