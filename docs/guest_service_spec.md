# Guest Service 入出力仕様

package: `tolo.guest.v1`
実現するコンテキスト: guest_context.md（ドメイン定義 guest_domain.md）
役割: ゲストへ会場の状況と誘導情報を表示する独立マイクロサービス（BFF ではない）

設計方針:
上流（観測・Operation）が push した内容を自 DB のゲスト向け読み取りモデルとして保持し、ゲストアクセス時は自 DB のみで応答する
アクセスの都度の外部サービス問い合わせによるオーバーヘッドと、キャッシュ TTL によるリアルタイム性の欠落を避けるため
push は PubSub を経由する（上流が publish し本サービスが購読。at-least-once 前提で冪等に処理）
ゲスト用ページは組み換え可能なコンポーネント群で構成され、各コンポーネントが本サービスに集約されたコンポーネント API から個別に取得する
URL（設置箇所×種類）がページ構成（コンポーネントの組み合わせ）を決め、発行 URL の生成・管理とページ構成の設定は本サービスが担う
本サービスが生成するのは発行 URL のパスまでとし、ホストは管理 UI がテナントのサブドメイン名から付ける（ADR-0104）
ゲスト個人へのリアルタイム push はしない（ゲストへはアクセス時 pull のまま。不変条件を維持）
ゲスト向け表示の組み立て（表示名の付与・文言化）は本サービスが担う
観測の push は ID と数値（レベル・待ち時間等）のみで、表示名はグラフ編集から取得した複製で本サービスが付与する

## RPC 一覧

### 受ける側（PubSub 購読）

| トピック | 説明（ユビキタス言語） | publish 元 | 配信 | 関連ドメインイベント |
|---|---|---|---|---|
| `guest-status` | 観測が確定させたゲスト向け状況（混雑・行列状況。ID と数値のみ、表示名は本サービスが付与）を受信し、自 DB の最新状態を置き換える。sequence が古いものは破棄（冪等） | 観測 | at-least-once、イベント単位の順序キー | ゲスト向け状況更新された / GuestViewUpdated |
| `guest-messages` | ゲスト向けスタッフメッセージの追加・取り下げを受信し、自 DB に保持する（message_id で冪等）。ゲストへの誘導情報の唯一の伝達経路（スタッフの手動配信） | Operation | 同上 | ゲスト向けメッセージ更新された / GuestMessageUpdated |

### ゲスト向け（`GuestPageService`。未認証）

| RPC | 説明（ユビキタス言語） | 関連ドメインイベント |
|---|---|---|
| OpenGuestPage | 設置箇所ごとに発行された URL の QR からゲスト用ページを開く（トークンなし）。`event_id`、`qr_location_id`、`session_id` を受け取り、GetQrLocations で取得した QR 設置箇所からその設置箇所の種類を引いて、種類に対応するページ構成（コンポーネントの並び）を返す。ページを開くごとに設置箇所単位で1回の人数計上を行う。同じイベント・設置箇所・`session_id` で30分以内に開き直した分と、`session_id` のない要求は計上しない（ページ構成は返す）。本サービスが知らない設置箇所では既定のページ構成を返し、計上しない | GuestPageOpened／GuestCountedViaQr |
| GetCrowd、GetQueues、ListGuestMessages | コンポーネント API（混雑・行列・スタッフメッセージ）。ページを構成する各コンポーネントが個別に呼ぶ。いずれも自 DB の読み取りモデルのみで応答（外部問い合わせなし。表示名は本サービスが付与）。誘導提案は返さない | GuestSnapshotServed（ページ表示の単位で発生） |
| GetGuideMap | 案内マップのコンポーネント API。`event_id` と `qr_location_id` を受け取り、ゲストに渡すグラフ（案内マップレイアウトが参照するポイントとグループの ID と言語コード別のラベル）と案内マップレイアウトを1回の応答で返す。開いた QR 設置箇所の配置位置が、案内マップレイアウトの参照するポイントを指すときは、その `point_id` を現在地として含める。ゲスト用ページはページを開いたときに1回だけ呼ぶ。誘導提案は返さない（ADR-0110、ADR-0111） | GuestSnapshotServed（同上） |

天候のコンポーネント API は後回しにし、入出力はまだ定義しない（`GuestComponent` に天候を含めない）

設置箇所の所有はグラフ編集（QrLocation。管理 UI がグラフと同じ画面で管理）
本サービスは GetQrLocations で pull した QR 設置箇所ごとに、発行 URL を1つ生成する。パスは `/event/{event_id}/guest/{qr_location_id}` とする
種類は QR 設置箇所の属性（QrLocation の `kind` は1つ）であるため、設置箇所×種類の発行 URL は設置箇所ごとに1つになる。種類→ページ構成の対応は本サービスのイベント設定として保持し、`GuestSettingsService` で読み書きする
QR の動的化（時間で変わる偽造対策。RotatingQrRefreshed）は将来導入とし、当面は固定 URL
サイネージ（SignageUpdated）も同じ読み取りモデル・コンポーネント API から描画する

### 管理向け（`GuestSettingsService`）

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| GetGuestSettings | イベントのゲスト向け設定（種類→ページ構成）を返す | 管理 UI | `event_access` + events.read | （参照のみ） |
| UpdateGuestSettings | イベントのゲスト向け設定（種類→ページ構成）を保存する。イベントの属するテナントは、保存時に `event_access` の `tenant_id` クレームから取る（リクエストには持たない） | 管理 UI | `event_access` + events.manage | — |
| ListIssuedUrls | イベントの QR 設置箇所ごとに、設置箇所の ID・名称・種類と発行 URL のパスを返す。GetQrLocations で取得した QR 設置箇所から作る | 管理 UI | `event_access` + events.read | （参照のみ） |

### 呼び出す側

| 相手 | RPC | 目的 |
|---|---|---|
| Observation | ReportMeasurements（source=QR） | QR 由来の人数計上（バッチ送信。ゲスト応答をブロックしない）。イベントの属するテナントを `tenant_id` に入れて送る。計上数は `count_in` に入れ、`count_out` は 0 とする |
| Observation | GetGuestSnapshot | 復旧・突き合わせ（起動時・push 欠落検知時のみ。通常経路では呼ばない） |
| Operation | ListStaffMessages（audience=GUEST） | 復旧・突き合わせ（同上）。イベントの属するテナントを `tenant_id` に入れて送る |
| Graph Authoring | GetDisplayNames | ポイント・ルート表示名と、現在の公開版の `revision_id` の取得（起動時と周期、未知 ID 検知時）。表示の組み立てと、GetQrLocations に渡す版の決定に使用 |
| Graph Authoring | GetQrLocations | QR 設置箇所（名称・種類・グラフ要素参照）の取得。発行 URL（設置箇所×種類）の生成に使用（設置箇所の所有はグラフ編集）。`revision_id` には GetDisplayNames の応答の `revision_id` を入れ、イベントの属するテナントを `tenant_id` に入れて送る |
| Graph Authoring | GetGuideMapLayout | 案内マップレイアウトと、それが参照するポイントとグループのラベルの取得。GetGuideMap の読み取りモデルに使用。GetQrLocations と同じく、GetDisplayNames の応答の `revision_id` が変わったときに取り直し、イベントの属するテナントを `tenant_id` に入れて送る |
| 外部天気 API | （HTTP、順応者） | 周期取得して自 DB に保持（アクセスの都度は取得しない）。取得に使うイベント所在地は本サービスの設定として保持 |

## 参考 proto 定義

`GuestPageService` と `GuestSettingsService` の proto は、tolo-guest リポジトリの `proto/tolo/guest/v1/guest.proto` を正とし、本文書には写さない
購読するトピックのペイロードは publish する側の package が定義し（ADR-0106）、本サービスは publish する側の Go モジュールを import して復号する

| トピック | 受信エンドポイント | ペイロード | 定義 |
|---|---|---|---|
| `guest-status` | `POST /push/guest-status` | `tolo.observation.v1.GuestStatus` | Observation |
| `guest-messages` | `POST /push/guest-messages` | `tolo.operation.v1.GuestMessagesUpdate` | Operation |

push は HTTP の受信エンドポイントで受ける。受信エンドポイントの形と応答の規約は service_transport.md の「情報更新 push の受信エンドポイント」に従う
本サービスの受信エンドポイントが恒久的な失敗として扱うのは、ペイロードを復号できない場合だけである。設定を保存していないイベントへの push も、ほかのイベントと同じく読み取りモデルに反映する

## 補足

- QR 由来の人数計上には、同一セッションからの連続計上を抑えるレート制限を設ける。同じイベント・設置箇所・`session_id` の開き直しは30分間計上しない
  QR は設置箇所ごとの固定 URL（トークンなし）であり、繰り返しアクセスによる水増しを防ぐ手段が他にないため
  セッションは、ゲスト用ページ（tolo-web）が生成して端末に保持し、OpenGuestPage に付けて送る `session_id` で識別する。クライアントの IP は使わない。IP は Service Gateway を越えて本サービスまで届かず、会場の NAT の内側にいる複数のゲストを1人と数えてしまうため
  計上数は Observation の観測ウィンドウに合わせ、1分のウィンドウ（`window_start`／`window_end`）に区切って送る

- 誘導提案は表示しない。提案（方向属性・境界制御・通行制限・行列形状）はスタッフ向けであり、ゲストへの誘導情報はスタッフのゲスト向けメッセージ（`guest-messages` トピック経由）のみで届く
  並び先案内・待ち時間は「案内」（局所行列誘導の区分）として QueueStatus に含まれ表示対象
  案内マップの混雑回避経路も「案内」である。ゲスト用ページが GetCrowd の混雑状況から計算する。誘導提案は使わない（ADR-0112）
- 案内マップはグラフ、案内マップレイアウト、混雑状況の3つから組み立てる（ADR-0110）
  GetGuideMap がゲストに渡すグラフは、案内マップの描画に使う要素と属性に限る。案内マップレイアウトが参照するポイントと、面になっているグループだけを、ID と言語コード別のラベルの組で返す。ルート、ポイントの種別、ルートの端点と向き、グループへの所属と親、外部ポイント、エディタの座標と寸法は返さない。ゲスト向けの RPC は未認証で誰でも呼べるため
  ラベルは言語コード別のまま返し、表示言語の選択はゲスト用ページが行う。1回だけ取得した内容のまま、ページの中で表示言語を切り替えられるようにするため
  案内マップレイアウトは案内マップエディタで作るゲスト向けの地図の描画情報である。部品（場所・エリア・線・文字。場所のうち階段・エスカレーター・エレベーターは階移動手段）が `point_id`・`route_id`・`group_id` でグラフを参照し、面はグラフのグループとする
  現在地は、`qr_location_id` の QR 設置箇所の配置位置がポイントを指し、そのポイントを案内マップレイアウトが参照しているときだけ返す。返さないときは、ゲスト用ページが案内マップレイアウトの既定の出発地から経路を始める（ADR-0111）
  案内マップレイアウトはグラフ編集が所有し、グラフ版に含まれる。本サービスは GetGuideMapLayout で現在の版の分を取得して複製し、GetGuideMap は自 DB だけで応答する
  現在の版に案内マップレイアウトがないイベントと、本サービスが知らないイベントへの GetGuideMap は `not_found` とする
- 整合性: 所有はあくまで上流（状況＝観測、メッセージ＝Operation）で、本サービスの DB は読み取りモデル（複製）
- 配信規約: at-least-once 前提で本サービスは冪等に処理する
  状況＝sequence の新旧判定で古いものを破棄、メッセージ＝message_id による upsert／取り下げ
  配信の再試行はブローカーに委譲（送信側の再試行は publish 失敗時のみ）
  起動時と低頻度の定期の復旧 pull（GetGuestSnapshot／ListStaffMessages）による突き合わせは維持（購読漏れと、恒久的な失敗として捨てたメッセージの回復）
  ListStaffMessages の返却は有効メッセージの全量であり、返却にない保持中 message_id は取り下げ扱いにする（取り下げ操作は Operation の RevokeStaffMessage）
- DB は読み取りモデル向け（KVS／ドキュメント系）を想定し、製品選定は実装フェーズ
- 汎用のイベントバスは置かない方針を維持（PubSub は情報更新 push の3トピックに限定し、ドメインイベントの流通基盤にはしない）
- ゲスト向け表示レベル（CrowdLevel）は観測の入力用 CongestionLevel とは別に定義（ゲストに露出する表示言語のため）
- `QueueStatus` の数値は Line Control の案内ダイジェスト（`tolo.line.v1.QueueDigest`）と同形で、観測が機械的に写して push する（Line Control）
- 表示名の付与とゲスト向け文言化は本サービスの責務
  表示名の所有はグラフ編集（グラフ文書のノードラベル＝言語コード別／エッジの表示名）で、本サービスは GetDisplayNames で pull した複製を保持し、`gate_point_ids` から行列のラベルを導く。ポイントのラベルは言語コード別に供給され、表示言語の選択は本サービスが行う
  ルート対象（CrowdStatus の route_id）の表示は、ルート表示名が未設定なら端点ポイント名から合成する（GetDisplayNames の `route_endpoints` を使用）
  表示名の取得契機（起動時・周期・未知 ID 検知時）の詳細は実装フェーズで決定
- QR 設置箇所は GetDisplayNames の応答の `revision_id`（現在の公開版）を指定して GetQrLocations で取得する
  起動時と周期の GetDisplayNames の pull で、前回 GetQrLocations に使った版と異なる `revision_id` が返ったときに取り直す。版の取得や公開の通知のために別の RPC やイベントバスは設けない
  Graph Authoring で設置箇所の種類を変えると、次の取り直しの後から、印刷済みの QR で開くページ構成も変わる
- イベントの属するテナントは、本サービスがイベントの設定として保持する。`UpdateGuestSettings` で設定を保存するときに、`event_access` の内部 JWT の `tenant_id` クレームから取る
  QR 由来の計上を Observation へ送るとき、`event_id` と合わせて `tenant_id` に入れる（Observation）。Operation の `ListStaffMessages` と Graph Authoring の `GetQrLocations` でも同じく `tenant_id` に入れる（Operation、Graph Authoring）
  `token_use=service` の内部 JWT はテナントのクレームを持たないため、送り先がテナントを確定できるようにリクエストで渡す
  設定を保存するまでテナントは分からないため、`tenant_id` を要するこの3つの呼び出しは、そのイベントでは設定の保存後から始める。`tenant_id` を要さない `GetGuestSnapshot` と `GetDisplayNames` は設定の有無によらず呼ぶ
- 設定を保存していないイベントは既定の設定で扱う。既定のページ構成は、QR の種類によらず混雑、行列、スタッフメッセージの3つをこの順に並べたものとする
  設定を保存したイベントでも、設置箇所の種類のページ構成がなければ既定のページ構成を返す
  本サービスが知らない設置箇所（QR 設置箇所をまだ取得していないイベントの設置箇所や、取得した一覧にない `qr_location_id`）への OpenGuestPage も既定のページ構成を返す。計上には設置箇所とテナントが分かっている必要があるため、この場合は計上しない
  ゲスト向けの RPC は、設定の有無によらず読み取りモデルから応答する
- 天候取得に使うイベント所在地は本サービスの設定として保持する（Tenant Management の Event には持たせない）
  所在地は天候のコンポーネント API とあわせてイベント設定に足す
- QR はトークンなしの発行 URL で確定（出し分けの軸は設置箇所×種類）
  偽造対策（署名・動的化）は将来導入。導入時も設置箇所単位の位置づけは変えない
- ゲスト用ページは組み換え可能なコンポーネント群で構成する
  各コンポーネントは本サービスのコンポーネント API から個別に取得し、API はいずれも読み取りモデルのみで応答する（「アクセス時は自 DB のみ」の不変条件は API 単位で維持）
  人数計上はページ表示時の1回（設置箇所単位）で、コンポーネント呼び出しでは計上しない
  コンポーネント API（混雑・行列・メッセージ）とページ構成の入出力は `GuestPageService` の proto に定める。天候は後回しにし、入出力はまだ定義しない
- サイネージのリアルタイム性・WebRTC 配信は将来判断（読み取りモデルは共用できる想定）
