![VoHiveX 個人向けモデム管理・テストプラットフォーム](images/vohivex-banner.png)

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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | 日本語

**原プロジェクト：** [iniwex5](https://github.com/iniwex5) による [VoHive](https://github.com/iniwex5/vohive)<br>
**VoHiveX 作者：** [NXN-MAX](https://github.com/NXN-MAX) · **バージョン：** 2.1.4

VoHiveX は VoHive から派生した、個人向けのモデム管理・テストプラットフォームです。DJI 4G モジュール対応、SMS スケジューリング、Mihomo の購読・ノード管理、eSIM アクティベーションコード認識、SMS アーカイブ、通知機能、Go ゲートウェイを使用したレスポンシブな Vue UI を追加しています。

## 主な機能

### DJI 4G モジュール対応

- USB ID `2ca3:4006` を持つ DJI 第1世代 4G モジュールに対応します。
- DJI の元の USB ID を維持し、再フラッシュ、USB ID の書き換え、ホストドライバーの恒久的変更は不要です。
- 実行時に既存の Linux `option` / `qmi_wwan` ドライバーへデバイスを関連付け、AT シリアルおよび QMI インターフェースをコンテナへ公開します。
- 第2世代モジュールは、ファームウェアとホストカーネルが提供する互換 QMI または MBIM 経路を利用できます。

### SMS スケジューリング

- 指定日時に1回送信、または日・時・分・秒の間隔で繰り返し送信できます。
- タスクの作成、編集、削除、開始、一時停止、次回実行時刻と履歴の確認に対応します。
- 新規または編集済みタスクは手動で開始するまで一時停止されます。実行できなかった分をまとめて送信することはありません。
- 実行結果と配信結果を、有効な Telegram、Feishu、QQ、Bark、Email、Pushplus、Webhook へ通知できます。

### 購読とノード管理

- VoHiveX コンテナ内で Mihomo を実行し、固定の内蔵 SOCKS5 エンドポイント `127.0.0.1:17890` を提供します。
- 複数の HTTPS 購読、Clash YAML、一般的なプロキシ共有リンクに対応します。
- 折りたたみ可能な購読一覧、ノード選択、遅延テスト、VoWiFi 国別ルール、ローカル送信プロキシを提供します。
- プロキシ QR 画像はブラウザー内だけで読み取り、認識後すぐに破棄します。

### eSIM アクティベーションコード認識

- QR 画像、クリップボード画像、アップロードした JPG/JPEG/PNG/WebP、または直接入力した `LPA:1` リンクを認識します。
- SM-DP+ アドレス、Matching ID、任意の確認コードを解析し、ダウンロードフォームへ入力します。
- 認識処理はフォームを入力するだけです。内容を確認して**ダウンロード開始**を押すまで Profile は書き込まれません。
- モジュールとカードが対応する範囲で、eUICC 情報、インストール済み Profile、メモ、切り替え、削除を利用できます。

### デバイスとメッセージ管理

- デバイス検出、無線状態、AT / USSD ターミナル、SIM/eSIM 情報、カードポリシー、VoWiFi 状態、リアルタイムログを提供します。
- 3列の SMS センターは、会話検索、未読状態、配信状態、複数選択操作、CSV/TXT/HTML/XML のインポートとエクスポートに対応します。
- ローカル Swagger UI は `/api/docs`、ヘルスチェックは `/healthz`、Prometheus 互換メトリクスは `/metrics` です。

## 対応アーキテクチャとモデム経路

| コンポーネント | 対応対象 |
| --- | --- |
| 完全な Docker ランタイム | Linux `amd64`、`arm64`/`aarch64`、`armv7` |
| 単体 Go ゲートウェイ | Linux `amd64`、`arm64`/`aarch64`、`armv7`、`386` |
| モデム通信 | AT シリアルと Qualcomm 互換 QMI または MBIM ネットワーク |
| ホストカーネルインターフェース | `option`、`qmi_wwan`、USB シリアル、QMI 制御デバイス、または互換 MBIM 経路 |
| DJI 第1世代 | USB ID `2ca3:4006`。USB ID の変更不要 |
| DJI 第2世代 | 互換 QMI/MBIM 構成。利用可能な機能はファームウェアと USB 構成に依存 |

完全なコンテナは、互換性のある組み込みモデムコアを用意できるアーキテクチャ向けにのみ公開されます。`386` のダウンロードには Go ゲートウェイだけが含まれ、別途提供する互換モデムコアへの接続が必要です。

## Docker インストール

イメージの公開先：

- `maxnxxn/vohivex:2.1.4`
- `ghcr.io/nxn-max/vohivex:2.1.4`

どちらもマルチアーキテクチャの `2.1.4`、`v2.1.4`、`latest` タグを提供し、Docker がホストに合うイメージを自動選択します。

既定値：

- Web ポート：`7575`
- ユーザー名：`admin`
- パスワード：`admin`

```sh
cp .env.example .env
docker compose pull
docker compose up -d
```

`http://<ホスト>:7575` を開きます。更新時も既存の `config`、`data`、`logs`、`driver-state` ディレクトリは保持されます。初回ログイン後に既定パスワードを変更してください。

コンテナは実行中のホストカーネルを検出し、既存のモデムドライバーを再利用します。カーネルパッケージをインストールしたり、ホストカーネルを置き換えたりしません。

## スクリーンショット

以下の画像はすべてローカルのデモ API から生成されています。デバイス ID、IP アドレス、電話番号、メッセージ、購読、ノード、eSIM データ、タスクは架空の例です。

| ダッシュボード | 購読とノード |
| --- | --- |
| ![架空のモデムデータを使用した VoHiveX ダッシュボード](images/dashboard.png) | ![架空のノードを使用した VoHiveX プロキシ購読](images/proxy.png) |

| eSIM QR / リンク認識 | SMS センター |
| --- | --- |
| ![サンプルデータを使用した VoHiveX eSIM 認識](images/esim.png) | ![架空の会話を使用した VoHiveX SMS センター](images/sms.png) |

### スケジュールタスク

![架空の宛先と本文を使用した VoHiveX スケジュールタスク](images/tasks.png)

## リリースとドキュメント

- [GitHub Releases](https://github.com/NXN-MAX/VoHiveX/releases)
- [Docker Hub](https://hub.docker.com/r/maxnxxn/vohivex)
- [ドライバーと導入手順](../app/README.md)
- [SMS スケジューリング](../app/scheduler/README.md)
- [Mihomo と購読](../app/proxy/README.md)
- [サードパーティ通知](../app/proxy/THIRD-PARTY.md)

## 責任ある利用とライセンス

> [!CAUTION]
> **商用利用は厳禁です。VoHiveX は、利用者が合法的に管理するデバイスと電話番号を用いた個人的な研究、学習、テストに限って使用できます。**

認証コードの収集、電話番号の貸し出し、迷惑または大量の SMS、詐欺、違法なプロキシサービス、現地法または通信事業者の規約に反する活動には使用しないでください。

原プロジェクトとサードパーティコンポーネントには各ライセンスが適用されます。VoHiveX の追加部分には [Personal Non-Commercial License](../LICENSE) が適用されます。このライセンスは OSI 承認のオープンソースライセンスではありません。
