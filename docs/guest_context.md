# ゲストコンテキスト 入出力整理

ドメイン定義: guest_domain.md
デプロイ単位: guest_service_spec.md（独立マイクロサービス）

設計方針: 上流からの push を受けて自 DB に読み取りモデルを保持し、ゲストへはアクセス時 pull で返す
ゲスト用ページはコンポーネント群で構成され、各コンポーネントが Guest Service に集約されたコンポーネント API から個別に取得する（URL＝設置箇所×種類がページ構成を決める）
誘導提案は表示しない（提案はスタッフ向け。ゲストへの誘導情報はスタッフのゲスト向けメッセージのみ）
案内マップは、グラフ、案内マップレイアウト、混雑状況の3つから組み立てる。ゲストに渡すグラフは案内マップの描画に使う要素と属性に限る
ゲスト向け表示の組み立て（表示名の付与・文言化）は Guest Service が担う（push は ID と数値のみ）

## ドメインイベント↔入出力 対応

| ドメインイベント | 対応 |
|---|---|
| RotatingQrRefreshed | 将来導入（QR の動的化＝偽造対策。当面は設置箇所×種類ごとの固定 URL。計上は設置箇所単位） |
| GuestPageOpened | ゲスト → Guest Service（HTTP。設置箇所・種類ごとの発行 URL の QR から。ページ構成を返す） |
| GuestCountedViaQr | Guest Service → Observation: ReportMeasurements（source=QR、ページ表示時に設置箇所単位で1回、バッチ） |
| GuestSnapshotServed | 各コンポーネント API が読み取りモデルから返却（ページ表示の単位で発生） |
| SignageUpdated | 同じ読み取りモデルから描画（専用要件は将来判断） |
| GuestViewUpdated | 観測が `guest-status` トピックへ publish（PubSub。混雑・行列のみ）→ Guest Service が購読 |
| GuestMessageUpdated | Operation が `guest-messages` トピックへ publish（PubSub。スタッフの手動配信）→ Guest Service が購読 |

## 他コンテキストとの接点

| 接点 | 対応 |
|---|---|
| 行列状況・混雑（並び先案内含む） | 観測が publish（`guest-status`。PubSub 経由）。復旧時のみ GetGuestSnapshot を pull（観測コンテキスト） |
| ゲスト向けスタッフメッセージ（誘導情報の唯一の伝達経路） | Operation が publish（`guest-messages`。PubSub 経由）。復旧時のみ ListStaffMessages を pull（スタッフ間コミュニケーションコンテキスト） |
| 誘導提案 | 届かない（提案はスタッフ向け。スタッフが手動でメッセージ化） |
| 表示名（ポイント・ルート） | グラフ編集から pull（GetDisplayNames）して複製を保持。正本はグラフ編集（グラフ編集コンテキスト） |
| 案内マップ（グラフと案内マップレイアウト） | GetGuideMap で、案内マップレイアウトが参照するポイントとグループの ID とラベル、案内マップレイアウト、QR 設置箇所から決めた現在地を返す。グラフの正本はグラフ編集。案内マップレイアウトの正本もグラフ編集（グラフ版に含まれる）。GetGuideMapLayout で現在の版の分を pull して複製を保持 |
| 混雑回避経路 | ゲスト用ページが GetCrowd の混雑状況から計算する案内。誘導提案は使わない |
| QR 設置箇所 | グラフ編集から pull（GetQrLocations）して発行 URL（設置箇所×種類）を生成。正本はグラフ編集 |
| 天候 | 外部天気 API（HTTP、順応者）。周期取得して自 DB に保持。イベント所在地は Guest Service の設定 |
| QR 由来の人数計上 | Observation: ReportMeasurements（source=QR） |
