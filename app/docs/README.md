# API Documentation

## Use

1. Sign in to the management interface.
2. Open **System Settings → System Information → View local API documentation**, or visit `/api/docs`.
3. Browse requests, responses, and schemas by API group. Use **Authorize** and **Try it out** for testing.

The specification is available at `/api/openapi.json` and requires Bearer authentication. Sign in again after the session expires. Requests are restricted to the current same-origin service; online specification validation and URL-based configuration overrides are disabled.

## Maintenance

- Page: `app/docs/index.html`.
- Swagger UI: `app/docs/vendor/swagger-ui-5.32.15/`.
- Asset integrity manifest: `manifest.json` in the same directory.
- Management frontend source and theme: `web/src/`.

```sh
python3 app/scheduler/build-assets.py
```

Regenerate the frontend and rebuild the image after source changes. Assets are served locally, and the licenses, NOTICE files, and integrity manifests must be distributed with them.
