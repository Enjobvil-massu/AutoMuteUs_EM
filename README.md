<p align="center">
    <a href="https://automute.us/#/" alt = "Website link"><img src="assets/AutoMuteUsBanner_cropped.png" width="800"></a>
</p>
<p align="center">
    <a href="https://github.com/automuteus/automuteus/actions?query=build" alt="Build Status">
        <img src="https://github.com/automuteus/automuteus/workflows/build/badge.svg" />
    </a>
    <a href="https://github.com/automuteus/automuteus/releases/latest">
    <img alt="GitHub release" src="https://img.shields.io/github/v/release/automuteus/automuteus" >
    </a>
    <a href="https://github.com/automuteus/automuteus/graphs/contributors" alt="Contributors">
        <img src="https://img.shields.io/github/contributors/automuteus/automuteus" />
    </a>
    <a href="https://discord.gg/ZkqZSWF" alt="Discord Link">
        <img src="https://img.shields.io/discord/754465589958803548?logo=discord" />
    </a>
</p>
<p align="center">
    <a href="https://hub.docker.com/repository/docker/automuteus/automuteus" alt="Pulls">
        <img src="https://img.shields.io/docker/pulls/denverquane/amongusdiscord.svg" />
    </a>
    <a href="https://automuteus.crowdin.com/automuteus" alt="localize">
        <img alt="Localize" src="https://badges.crowdin.net/e/5eb1365b5fd16082e63cc54c33736adc/localized.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/automuteus/automuteus/v8" alt="Report Card">
        <img src="https://goreportcard.com/badge/github.com/automuteus/automuteus/v8" />
    </a>
</p>

<p align="center">
    <a href="https://add.automute.us" alt="invite">
        <img alt="Invite Link" src="https://img.shields.io/static/v1?label=bot&message=invite%20me&color=purple">
    </a>
</p>

# AutoMuteUs_EM

AutoMuteUs_EMは、公式[AutoMuteUs](https://github.com/automuteus/automuteus)を元に、日本語表示、操作画面、AmongUsCapture接続案内、障害時の安全処理などを調整したカスタム版です。

> [!IMPORTANT]
> このリポジトリは公式AutoMuteUsの運営版ではありません。
> 公式Botの招待リンクや公式サポートとは別に、自前のDocker環境で運用しています。

## 主な機能

- Among Usのゲーム状態に合わせたDiscordの自動ミュート・解除
- ロビー、タスク、会議、ゲーム終了の状態追跡
- 日本語のコマンド名、説明、案内メッセージ
- AmongUsCaptureの起動・接続リンク
- 手動接続用のホストと接続コード表示
- 18色のプレイヤーリンクボタン
- DiscordユーザーとAmong Usプレイヤーの手動リンク・解除
- Discord表示名とAmong Us名の併記
- VC退出・VC移動・ゲーム終了時のミュート解除処理
- Redis、統計DB、Capture通信エラー時の安全処理

## 必要なもの

1. Discordのボイスチャンネル
2. Among Us
3. Windows用[AmongUsCapture](https://github.com/automuteus/amonguscapture/releases/latest)
4. AutoMuteUs_EMが導入されたDiscordサーバー

AmongUsCaptureが接続されていない場合、Among Usのゲーム状態を取得できないため、自動ミュートは動作しません。

## 基本的な使い方

1. Discordの対象ボイスチャンネルへ参加します。
2. 対象テキストチャンネルで`/start`を実行します。
3. 実行者だけに表示される「AmongUsCaptureを起動・接続する」を押します。
4. 自動接続できない場合は、表示されたホストとコードをAmongUsCaptureへ手動入力します。
5. Among Usでロビーを作成または参加します。
6. 色ボタンまたは`/link`を使って、Discordユーザーとプレイヤーをリンクします。
7. ゲーム終了後は`/stop`で追跡を終了します。

## 使用できるコマンド

| コマンド | 説明 |
|---|---|
| `/help` | 使用方法とコマンド説明を表示します |
| `/start` | 現在のチャンネルでオートミュートを開始します |
| `/stop` | オートミュートとゲーム追跡を停止します |
| `/link` | DiscordユーザーをAmong Us内の色へ手動リンクします |
| `/unlink` | DiscordユーザーのAmong Usリンクを解除します |
| `/settings` | サーバーごとのミュート設定を表示・変更します |

## プレイヤーのリンク

### 色ボタン

AmongUsCaptureからプレイヤー情報を取得すると、18色のリンクボタンが表示されます。

参加者は自分が使用している色のボタンを押して、DiscordアカウントとAmong Usプレイヤーをリンクします。

### 手動リンク

色ボタンでリンクできない場合は、`/link`を使用します。

指定する項目：

- `user`：リンクするDiscordユーザー
- `color`：Among Usで使用している色

### リンク解除

間違ったユーザーや色へリンクした場合は、`/unlink`を使用します。

## Discord表示名

プレイヤー一覧では、基本的に次の順序でDiscord表示名を選択します。

1. Discordサーバー内のニックネーム
2. Discordのグローバル表示名
3. Discordアカウント名
4. DiscordユーザーID

表示例：

```text
Among Us名（Discord表示名）
