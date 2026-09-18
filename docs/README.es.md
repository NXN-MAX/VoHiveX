![VoHiveX, plataforma personal de gestión y pruebas de módems](images/vohivex-banner.png)

<p align="center">
  <a href="https://github.com/iniwex5/vohive">VoHive</a> ·
  <a href="https://go.dev/">Go</a> ·
  <a href="https://github.com/MetaCubeX/mihomo">Mihomo</a> ·
  <a href="https://github.com/vuejs/core">Vue 3</a> ·
  <a href="https://github.com/vitejs/vite">Vite</a> ·
  <a href="https://github.com/vuejs/pinia">Pinia</a> ·
  <a href="https://github.com/antdv-next/antdv-next">Antdv Next</a> ·
  <a href="https://github.com/Remix-Design/RemixIcon">Remix Icon</a>
</p>

# VoHiveX

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | Español | [日本語](README.ja.md)

**Proyecto original:** [VoHive](https://github.com/iniwex5/vohive), de [iniwex5](https://github.com/iniwex5)<br>
**Autor de VoHiveX:** [NXN-MAX](https://github.com/NXN-MAX) · **Versión:** 2.1.4

VoHiveX es una plataforma personal de gestión y pruebas de módems derivada de VoHive. Añade compatibilidad con módulos DJI 4G, SMS programados, gestión de suscripciones y nodos Mihomo, reconocimiento de códigos de activación eSIM, archivo de SMS, notificaciones y una interfaz Vue adaptable respaldada por una puerta de enlace en Go.

## Funciones principales

### Compatibilidad con módulos DJI 4G

- Admite módulos DJI 4G de primera generación con USB ID `2ca3:4006`.
- Conserva el USB ID original de DJI. No requiere reflashear, reescribir el USB ID ni modificar permanentemente los controladores del host.
- Asocia el dispositivo en tiempo de ejecución con los controladores Linux `option` y `qmi_wwan` existentes, y expone al contenedor las interfaces serie AT y QMI.
- Los módulos de segunda generación pueden utilizar rutas QMI o MBIM compatibles proporcionadas por el firmware y el kernel del host.

### SMS programados

- Envío único en una fecha y hora concretas o repetición por intervalos de días, horas, minutos y segundos.
- Permite crear, editar, eliminar, iniciar y pausar tareas, además de consultar la próxima ejecución y el historial.
- Las tareas nuevas o modificadas permanecen pausadas hasta que se inician manualmente. Las ejecuciones omitidas no se envían juntas.
- Los resultados de ejecución y entrega pueden enviarse por los canales habilitados de Telegram, Feishu, QQ, Bark, Email, Pushplus y Webhook.

### Gestión de suscripciones y nodos

- Ejecuta Mihomo dentro del contenedor VoHiveX y ofrece un endpoint SOCKS5 integrado fijo en `127.0.0.1:17890`.
- Admite varias suscripciones HTTPS, Clash YAML y enlaces comunes para compartir proxies.
- Incluye listas de suscripciones plegables, selección de nodos, pruebas de latencia, reglas VoWiFi por país e instancias de proxy de salida local.
- Las imágenes QR del proxy se decodifican en el navegador y se descartan inmediatamente después del reconocimiento.

### Reconocimiento de códigos de activación eSIM

- Reconoce imágenes QR, imágenes del portapapeles, archivos JPG/JPEG/PNG/WebP subidos y enlaces `LPA:1` introducidos directamente.
- Extrae la dirección SM-DP+, el Matching ID y el código de confirmación opcional en el formulario de descarga.
- El reconocimiento solo rellena el formulario. No se escribe ningún Profile hasta que el usuario revisa los valores y pulsa **Iniciar descarga**.
- Incluye información eUICC, Profile instalados, notas, cambio y eliminación cuando el módulo y la tarjeta lo permiten.

### Gestión de dispositivos y mensajes

- Ofrece detección de dispositivos, estado de radio, terminales AT y USSD, información SIM/eSIM, políticas de tarjeta, estado VoWiFi y registros en directo.
- El centro de SMS de tres columnas incluye búsqueda de conversaciones, estado de lectura, estado de entrega, acciones múltiples e importación/exportación CSV/TXT/HTML/XML.
- Swagger UI local está en `/api/docs`, la comprobación de estado en `/healthz` y las métricas compatibles con Prometheus en `/metrics`.

## Arquitecturas y rutas de módem compatibles

| Componente | Destinos compatibles |
| --- | --- |
| Entorno Docker completo | Linux `amd64`, `arm64`/`aarch64` y `armv7` |
| Puerta de enlace Go independiente | Linux `amd64`, `arm64`/`aarch64`, `armv7` y `386` |
| Transporte del módem | Serie AT con red QMI o MBIM compatible con Qualcomm |
| Interfaces del kernel del host | `option`, `qmi_wwan`, serie USB, dispositivo de control QMI o ruta MBIM compatible |
| DJI de primera generación | USB ID `2ca3:4006`, sin cambiar el USB ID |
| DJI de segunda generación | Diseños QMI/MBIM compatibles; funciones según el firmware y la composición USB |

El contenedor completo solo se publica para arquitecturas que disponen de un núcleo de módem integrado compatible. La descarga `386` contiene únicamente la puerta de enlace Go y debe conectarse a un núcleo de módem compatible proporcionado por separado.

## Instalación con Docker

Las imágenes se publican en:

- `maxnxxn/vohivex:2.1.4`
- `ghcr.io/nxn-max/vohivex:2.1.4`

Ambos repositorios ofrecen las etiquetas multiarquitectura `2.1.4`, `v2.1.4` y `latest`. Docker selecciona automáticamente la imagen adecuada para el host.

Valores predeterminados:

- Puerto web: `7575`
- Usuario: `admin`
- Contraseña: `admin`

```sh
cp .env.example .env
docker compose pull
docker compose up -d
```

Abre `http://<direccion-del-host>:7575`. Los directorios existentes `config`, `data`, `logs` y `driver-state` se conservan durante las actualizaciones. Cambia la contraseña predeterminada tras el primer inicio de sesión.

El contenedor detecta el kernel activo del host y reutiliza sus controladores de módem. No instala paquetes del kernel ni sustituye el kernel del host.

## Capturas de pantalla

Todas las capturas siguientes proceden de la API de demostración local. Los identificadores de dispositivo, direcciones IP, números de teléfono, mensajes, suscripciones, nodos, datos eSIM y tareas son ejemplos ficticios.

| Panel | Suscripciones y nodos |
| --- | --- |
| ![Panel de VoHiveX con datos ficticios del módem](images/dashboard.png) | ![Suscripciones proxy de VoHiveX con nodos ficticios](images/proxy.png) |

| Reconocimiento eSIM por QR o enlace | Centro de SMS |
| --- | --- |
| ![Reconocimiento de activación eSIM de VoHiveX con datos de ejemplo](images/esim.png) | ![Centro de SMS de VoHiveX con conversaciones ficticias](images/sms.png) |

### Tareas programadas

![Tareas programadas de VoHiveX con destinatarios y contenido ficticios](images/tasks.png)

## Versiones y documentación

- [GitHub Releases](https://github.com/NXN-MAX/VoHiveX/releases)
- [Docker Hub](https://hub.docker.com/r/maxnxxn/vohivex)
- [Controladores y despliegue](../app/README.md)
- [SMS programados](../app/scheduler/README.md)
- [Mihomo y suscripciones](../app/proxy/README.md)
- [Avisos de terceros](../app/proxy/THIRD-PARTY.md)

## Uso responsable y licencia

> [!CAUTION]
> **El uso comercial está estrictamente prohibido. VoHiveX está destinado únicamente a investigación, aprendizaje y pruebas personales con dispositivos y números de teléfono que controles legalmente.**

No utilices VoHiveX para recopilar códigos de verificación, alquilar números, enviar mensajes no solicitados o masivos, cometer fraude, prestar servicios proxy ilícitos ni realizar actividades contrarias a la legislación local o a las condiciones del operador.

El proyecto original y los componentes de terceros conservan sus respectivas licencias. Las adiciones de VoHiveX utilizan la [Licencia personal no comercial](../LICENSE), que no es una licencia de código abierto aprobada por la OSI.
