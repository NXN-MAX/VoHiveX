![VoHiveX 個人模組管理與測試平台](images/vohivex-banner.png)

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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | 繁體中文 | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

**原始專案：** [iniwex5](https://github.com/iniwex5) 開發的 [VoHive](https://github.com/iniwex5/vohive)<br>
**VoHiveX 作者：** [NXN-MAX](https://github.com/NXN-MAX) · **版本：** 2.1.4

VoHiveX 是從 VoHive 衍生的個人模組管理與測試平台。專案加入 DJI 4G 模組相容性、排程簡訊、Mihomo 訂閱與節點管理、eSIM 啟用碼辨識、簡訊封存、訊息推送，以及由 Go 閘道支援的響應式 Vue 介面。

## 主要功能

### DJI 4G 模組相容性

- 支援使用 USB ID `2ca3:4006` 的 DJI 第一代 4G 模組。
- 保留 DJI 原始 USB ID，無需重新刷機、改寫 USB ID 或永久修改主機驅動程式。
- 執行時將裝置配對至 Linux 現有的 `option` 與 `qmi_wwan` 驅動程式，並向容器提供 AT 串列與 QMI 介面。
- 第二代模組可透過韌體與主機核心提供的相容 QMI 或 MBIM 通道使用。

### 排程簡訊

- 可在指定日期時間傳送一次，或按天、時、分、秒重複傳送。
- 支援新增、修改、刪除、開始、暫停任務，並檢視下次執行時間與執行記錄。
- 新增或修改後的任務保持暫停，需手動開始；錯過的執行不會集中補發。
- 執行與投遞結果可透過已啟用的 Telegram、Feishu、QQ、Bark、Email、Pushplus 與 Webhook 管道推送。

### 訂閱與節點管理

- 在 VoHiveX 容器中執行 Mihomo，並提供固定的內建 SOCKS5 位址 `127.0.0.1:17890`。
- 支援多個 HTTPS 訂閱、Clash YAML 與常見代理分享連結。
- 提供可摺疊訂閱清單、節點選擇、延遲測試、VoWiFi 國家規則及本機出站代理執行個體。
- 代理 QR 圖片僅在瀏覽器中辨識，完成後立即丟棄圖片資料。

### eSIM 啟用碼辨識

- 支援辨識 QR 圖片、剪貼簿圖片、上傳的 JPG/JPEG/PNG/WebP 檔案，或直接輸入 `LPA:1` 連結。
- 將 SM-DP+ 位址、Matching ID 與選用確認碼解析並填入下載表單。
- 辨識只會填入表單。使用者檢查內容並按下**開始下載**前，不會寫入 Profile。
- 支援 eUICC 資訊、已安裝 Profile、備註、切換與刪除，實際功能取決於模組與卡片能力。

### 裝置與訊息管理

- 提供裝置探索、無線狀態、AT 與 USSD 終端、SIM/eSIM 資訊、卡片策略、VoWiFi 狀態及即時日誌。
- 三欄簡訊中心支援對話搜尋、未讀狀態、投遞狀態、批次操作，以及 CSV/TXT/HTML/XML 匯入與匯出。
- 本機 Swagger UI 位於 `/api/docs`，健康檢查位於 `/healthz`，Prometheus 相容指標位於 `/metrics`。

## 支援的架構與模組通道

| 元件 | 支援目標 |
| --- | --- |
| 完整 Docker 執行環境 | Linux `amd64`、`arm64`/`aarch64`、`armv7` |
| 獨立 Go 閘道 | Linux `amd64`、`arm64`/`aarch64`、`armv7`、`386` |
| 模組傳輸 | AT 串列，以及 Qualcomm 相容的 QMI 或 MBIM 網路 |
| 主機核心介面 | `option`、`qmi_wwan`、USB 串列、QMI 控制裝置或相容 MBIM 通道 |
| DJI 第一代 | USB ID `2ca3:4006`，無需修改 USB ID |
| DJI 第二代 | 相容 QMI/MBIM 配置；可用功能取決於韌體與 USB 組合 |

完整容器僅為具有相容內建模組核心的架構發布。`386` 下載只包含 Go 閘道，必須連線至另行提供的相容模組核心。

## Docker 安裝

映像發布於：

- `maxnxxn/vohivex:2.1.4`
- `ghcr.io/nxn-max/vohivex:2.1.4`

兩個儲存庫均提供多架構的 `2.1.4`、`v2.1.4` 與 `latest` 標籤，Docker 會自動選擇適合主機的映像。

預設值：

- 網頁連接埠：`7575`
- 使用者名稱：`admin`
- 密碼：`admin`

```sh
cp .env.example .env
docker compose pull
docker compose up -d
```

開啟 `http://<主機位址>:7575`。更新時會保留現有的 `config`、`data`、`logs` 與 `driver-state` 目錄。首次登入後請修改預設密碼。

容器會偵測主機目前執行的核心並重用既有模組驅動程式；不會安裝核心套件或取代主機核心。

## 專案畫面

以下畫面均由本機展示 API 產生。裝置識別碼、IP 位址、電話號碼、簡訊、訂閱、節點、eSIM 資料與任務均為虛構範例。

| 儀表板 | 訂閱與節點 |
| --- | --- |
| ![使用虛構模組資料的 VoHiveX 儀表板](images/dashboard.png) | ![使用虛構節點的 VoHiveX 代理訂閱](images/proxy.png) |

| eSIM QR 或連結辨識 | 簡訊中心 |
| --- | --- |
| ![使用範例資料的 VoHiveX eSIM 啟用碼辨識](images/esim.png) | ![使用虛構對話的 VoHiveX 簡訊中心](images/sms.png) |

### 排程任務

![使用虛構收件號碼與內容的 VoHiveX 排程任務](images/tasks.png)

## 發布與文件

- [GitHub Releases](https://github.com/NXN-MAX/VoHiveX/releases)
- [Docker Hub](https://hub.docker.com/r/maxnxxn/vohivex)
- [驅動程式與部署說明](../app/README.md)
- [排程簡訊說明](../app/scheduler/README.md)
- [Mihomo 與訂閱說明](../app/proxy/README.md)
- [第三方聲明](../app/proxy/THIRD-PARTY.md)

## 合理使用與授權

> [!CAUTION]
> **嚴禁商業用途。VoHiveX 僅用於對您合法控制的裝置與電話號碼進行個人研究、學習及測試。**

請勿將 VoHiveX 用於驗證碼收集、號碼出租、未經請求或大量傳送簡訊、詐騙、非法代理服務，或任何違反當地法律與電信業者條款的活動。

原始專案及第三方元件保留各自授權。VoHiveX 新增內容採用[個人非商業授權](../LICENSE)，此授權並非 OSI 核准的開放原始碼授權。
