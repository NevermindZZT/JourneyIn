# JourneyIn

[![CI](https://github.com/NevermindZZT/JourneyIn/actions/workflows/ci.yml/badge.svg)](https://github.com/NevermindZZT/JourneyIn/actions/workflows/ci.yml)
[![Docker Image Version](https://img.shields.io/docker/v/nevermindzzt/journeyin?sort=semver)](https://hub.docker.com/r/nevermindzzt/journeyin)
[![Docker Pulls](https://img.shields.io/docker/pulls/nevermindzzt/journeyin)](https://hub.docker.com/r/nevermindzzt/journeyin)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> **在地图上规划每一段旅程**
>
> JourneyIn 是地图优先的旅行规划工具：把地点、顺序、路线、天气和备注放在同一张可保存的行程地图上。

![snapshot](./docs/snapshot.jpg)

项目主页：[github.com/NevermindZZT/JourneyIn](https://github.com/NevermindZZT/JourneyIn)。

JourneyIn 当前提供 Go + SQLite 服务端、Vue/Ionic Web 客户端、Trip JSON、地图 Provider、只读分享、同步接口、MCP 和 Docker 配置。项目采用 [MIT License](LICENSE)，允许在保留版权和许可声明的前提下使用、修改和分发。

## 本地运行

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:CGO_ENABLED = '0'
go run ./cmd/journeyin -listen 127.0.0.1:8080 -data D:/data/journeyin/journeyin.db
~~~

打开 http://127.0.0.1:8080 。

### Docker 镜像

发布镜像位于 Docker Hub：

~~~text
nevermindzzt/journeyin:latest
nevermindzzt/journeyin:0.2.3
~~~

使用持久化卷启动：

~~~powershell
docker run --detach --name journeyin --publish 8080:8080 --volume journeyin-data:/data --env JOURNEYIN_AUTH_USERNAME=admin --env JOURNEYIN_AUTH_PASSWORD=<strong-login-password> --env JOURNEYIN_MCP_TOKEN=<strong-random-token> nevermindzzt/journeyin:latest
~~~

镜像由 GitHub Actions 在推送 `v*.*.*` tag 时构建并发布，同时生成 `linux/amd64` 和 `linux/arm64` 多架构镜像。发布需要仓库 Secret `DOCKERHUB_TOKEN`。

从 Docker Hub 拉取镜像部署可直接使用 `docker-compose.hub.yml`；该文件不执行本地构建，并且每次启动都会检查远端镜像：

~~~powershell
$env:JOURNEYIN_AUTH_USERNAME = 'admin'
$env:JOURNEYIN_AUTH_PASSWORD = '<strong-login-password>'
$env:JOURNEYIN_MCP_TOKEN = '<strong-random-token>'
docker compose -f docker-compose.hub.yml pull
docker compose -f docker-compose.hub.yml up --detach
~~~

如需固定版本，可设置 `$env:JOURNEYIN_IMAGE = 'nevermindzzt/journeyin:0.2.3'` 后再执行上述命令。

默认使用 Docker named volume `journeyin-data`，由容器以非 root 用户写入，不需要创建或修改宿主机目录权限。若希望把数据直接放在宿主机目录，可设置 `JOURNEYIN_DATA_PATH` 切换为 bind mount；Compose 会自动创建目录，镜像启动时会检查固定的 `/data`，按需修复数据库目录及 SQLite sidecar 文件的所有者，然后同一个 Go 进程在打开 SQLite 前永久降权为 UID/GID 65532，不需要 `chmod 777` 或手动 `chown`：

~~~powershell
$env:JOURNEYIN_DATA_PATH = './data'
docker compose -f docker-compose.hub.yml up --detach
~~~

Linux rootful Docker 和常见 Docker Desktop 环境可直接使用。rootless Docker、NFS/受限文件系统或只读挂载可能禁止容器内 `chown`；这类环境请继续使用 named volume，或提前按运行用户配置宿主机权限。`JOURNEYIN_DATA_PATH` 应映射到宿主机目录或 named volume，应用数据库固定为 `/data/journeyin.db`。

构建和重新运行不会删除数据库。若省略 `-data` 与 `JOURNEYIN_DATA_DIR`，默认使用用户配置目录中的 `JourneyIn/journeyin.db`；程序也会兼容读取当前目录、可执行文件目录或其上级目录下已有的 `data/journeyin.db`。验收时建议始终使用固定绝对路径。

如果省略 `-data` 和 `JOURNEYIN_DATA_DIR`，Windows 默认保存到 `%AppData%/JourneyIn/journeyin.db`；Linux/macOS 使用用户配置目录。构建 Go 二进制不会清理数据库。建议验收时固定使用绝对数据路径。

Docker 部署必须设置 `JOURNEYIN_AUTH_USERNAME` 和 `JOURNEYIN_AUTH_PASSWORD`；浏览器访问 REST API 时会显示账号密码登录面板，登录成功后使用 24 小时 HttpOnly 会话 Cookie。监听非 localhost 地址时还必须设置 `JOURNEYIN_MCP_TOKEN` 保护 `/mcp`。`JOURNEYIN_API_TOKEN` 仍作为兼容旧客户端的可选 Bearer Token。

## 本地开发：前后端分离

推荐开发时不先启动 Docker，而是分别运行 Go 服务端和 Vite Web 服务。

终端 A：启动 Go API，端口 8080。

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:GOTELEMETRY = 'off'
$env:CGO_ENABLED = '0'
go run ./cmd/journeyin -listen 127.0.0.1:8080 -data D:/data/journeyin/journeyin.db
~~~

终端 B：启动 Vue/Ionic 开发服务，端口 5173。

~~~powershell
cd web
pnpm install
pnpm run dev
~~~

打开 http://127.0.0.1:5173。web/vite.config.ts 已配置 /api、/mcp 和 /_AMapService 到 8080 的代理。

## 本地构建与验证（当前优先）

执行完整本地构建：

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:GOTELEMETRY = 'off'
$env:CGO_ENABLED = '0'
pnpm --dir web install --frozen-lockfile
pnpm --dir web typecheck
pnpm --dir web build
go test ./...
go vet ./...
go build -trimpath -o dist/journeyin.exe ./cmd/journeyin
~~~

如果当前受限环境阻止 esbuild 子进程，可先只安装依赖并执行直接类型检查；这不会生成 production bundle：

~~~powershell
pnpm --dir web install --ignore-scripts
cd web
& '.\node_modules\.bin\vue-tsc.cmd' --noEmit
~~~

启动服务端后，可运行基础验证脚本：

~~~powershell
pwsh -File scripts/verify-local.ps1 -ServerUrl http://127.0.0.1:8080 -WebUrl http://127.0.0.1:8080
~~~

该脚本检查 health、capabilities、Trip Schema 和嵌入式 Web 根页面。手工验收步骤见 ACCEPTANCE.md。

## 地图规划流程

1. 点击右上角 `+` 新建一个 draft Trip。
2. 打开行程面板的“添加地点”。
3. 输入地点关键词、城市和搜索类型，点击“搜索地点”；系统先查本地 7 天地点目录，再按“地点检索”设置选择高德或百度。高德景点搜索使用 POI 类型码；Provider 不可用时自动尝试另一家。无 POI 结果时会降级到已缓存的地理编码结果。对于甘加、白石崖等景区名称，可以选择“景点”类型。
4. 从候选结果中明确选择一个地点并点击“添加”；Trip 会保存名称、地址、原始 Provider 坐标、CRS、来源、Provider UID，以及高德返回的 citycode/adcode（如有）。
5. 打开规划点详情，可以继续关联多个子规划点；只有进入该主规划点详情时，子点 Marker 才会显示在地图上。
6. 点击规划点区域右上方的“调整顺序”进入排序模式；拖动每行左侧的 ⋮⋮ 手柄到目标位置，松开后立即保存。可在当前 Day 内调整主规划点顺序；主点顺序变化会清除该日及下一日受影响的旧路线。子规划点在主点详情中同样支持拖动排序，不改变主点路线。调整完成后选择路线 Provider、路线方式并点击“生成路线”重新按相邻点分段请求；跨天行程会自动把前一天最后一个规划点接到后一天第一个规划点。规划点区域根据当前地图 Provider 显示对应路线总距离、预计总时长和路线段数，并将 geometry、距离、耗时、Provider、mode 和 CRS 保存到 Trip 的 `Day.legs[].snapshots[]`。同一逻辑路线段可以并存百度和高德快照；切换 Provider 不会复用另一家的 geometry。服务端会按 Provider 串行化上游地图请求，高德/百度临时错误和限流状态会在单次请求内有限重试。
7. 在规划点详情点击“获取天气/刷新天气”；天气快照写入该点，显示查询时间，6 小时内重复刷新优先使用 SQLite cache。
8. 后续地图重绘直接使用已保存的规划点、子规划点和当前 Provider 路线快照，不重复搜索或地理编码。地图 HUD 可切换百度地图/高德地图；“卫星图/标准图”只切换当前 Provider 图层，不触发 POI/路线请求。
9. 地图默认显示地点名和行程日期 Label；HUD 可切换“显示标签/隐藏标签”，设置保存在浏览器 localStorage。
10. 行程面板先展示行程列表；点击某条行程后进入独立的选中行程详情，使用“‹ 行程列表”返回，不会把其他行程卡片插入当前行程信息中。行程总体说明和规划点说明都可以二次编辑；规划点说明编辑时可进入全屏输入模式。面板提供“导入 Trip”“导出 JSON”和“在线分享”入口；导入会先调用只读校验接口，在线分享生成可撤销的只读链接，访问时读取该行程最新数据，不需要为每次更新生成新链接。

规划 API：

~~~text
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/validate
POST /api/v1/import
GET  /api/v1/trips/{trip_id}/export.json
POST /api/v1/shares
POST /api/v1/shares/{share_id}/revoke
POST /api/v1/maps/pois/search
POST /api/v1/trips/{trip_id}/days/{day_id}/stops
POST /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}/move  # body: {"direction":"up"} 或 {"target_sequence":2}
DELETE /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}
POST /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}/children
POST /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}/weather
POST /api/v1/trips/{trip_id}/plan
POST /api/v1/trips/{trip_id}/routes/refresh
PUT  /api/v1/settings/poi
DELETE /api/v1/settings/place-directory
~~~

地图选点：在地图 HUD 点击“地图选点”，再点击地图位置；填写名称和日期后保存。百度地图写入 BD-09LL，高德地图写入 GCJ-02；坐标会保留 Provider 和 CRS，不调用 POI 检索。规划点顺序通过 move 接口调整，主点移动后必须重新生成受影响 Provider 的路线；移动和删除都需要 `If-Match: revision-<n>`，删除前端会先二次确认。

默认只在显式规划操作时调用地图服务；POI/geocode/route/weather 由 SQLite cache、同请求 singleflight、全局并发信号量、每个 Provider 的独立串行闸门和可选每日上限控制。百度/高德临时 DNS、内部错误和限流状态会在单次请求内有限重试；多天路线会在后一天保存从前一天最后一个规划点到当天第一个规划点的边界路线。配置：

~~~powershell
$env:JOURNEYIN_MAP_PROVIDER = 'baidu' # 或 amap，作为新 Trip 和无偏好页面的默认 Provider
$env:JOURNEYIN_MAP_MAX_CONCURRENCY = '2'
$env:JOURNEYIN_MAP_DAILY_LIMIT = '0' # 0 表示不在 JourneyIn 内设置上限
$env:JOURNEYIN_AMAP_SERVER_KEY = '<amap-webservice-key>'
$env:JOURNEYIN_AMAP_JS_KEY = '<amap-web-js-key>'
$env:JOURNEYIN_AMAP_SECURITY_JS_CODE = '<amap-js-security-code>'
~~~

高德服务端 Key 优先从设置页保存的 SQLite app_settings 读取；只有设置项为空时才使用环境变量 fallback。高德 POI 结果使用 GCJ-02，保存地点时会同时保留原始 CRS 和用于百度 BMap 显示的 BD-09LL 转换坐标。地点搜索结果进入本地目录后保留 7 天，设置页可以手动清除。

## MCP

远程 MCP endpoint：

~~~text
http://127.0.0.1:8080/mcp
~~~

本地 stdio：

~~~text
journeyin mcp stdio
~~~

当前工具：

~~~text
journeyin.get_capabilities
journeyin.validate_trip
journeyin.preview_save_trip
journeyin.commit_save_trip
journeyin.plan_trip
journeyin.refresh_routes
journeyin.get_trip
journeyin.list_trips
~~~

保存必须遵循 validate -> preview -> 用户确认 -> commit。详细 Agent 行为见 .agents/skills/journeyin-save-trip/SKILL.md。

## REST 示例

~~~powershell
$payload = @{ 
  schema_version = 1
  title = '杭州周末行'
  status = 'draft'
  timezone = 'Asia/Shanghai'
  date_range = @{ start = '2026-04-18'; end = '2026-04-18' }
  days = @(@{ id = 'day-1'; date = '2026-04-18'; stops = @(@{ id = 'stop-1'; sequence = 1; title = '西湖' }) })
} | ConvertTo-Json -Depth 10
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/trips -ContentType application/json -Body $payload
~~~

## 地图与分享 API

~~~text
POST /api/v1/maps/geocode
POST /api/v1/maps/reverse-geocode
POST /api/v1/maps/route
POST /api/v1/maps/weather
POST /api/v1/maps/navigation
POST /api/v1/shares
GET  /s/<token>
GET  /s/<token>.json
GET  /api/v1/sync/pull
POST /api/v1/sync/push
GET  /api/v1/settings
PUT  /api/v1/settings/map-keys
~~~

百度服务端 Key 使用 JOURNEYIN_BAIDU_SERVER_AK；浏览器端 JSAPI 4.0/BMap（兼容 legacy BMapGL）Key 使用 JOURNEYIN_BAIDU_BROWSER_AK。高德 Web Service Key 使用 JOURNEYIN_AMAP_SERVER_KEY，JS API 2.0 浏览器 Key 使用 JOURNEYIN_AMAP_JS_KEY，JS 安全密钥使用 JOURNEYIN_AMAP_SECURITY_JS_CODE；高德安全密钥只在服务端代理中使用。百度和高德 Key 都可以在设置页填写并保存到 SQLite 的 app_settings 表；已保存的设置优先于环境变量，服务端 Key 和 JS 安全密钥只返回已配置状态，不回显原文。没有 Key 时服务返回明确的 provider_unavailable，不伪造路线。设置接口为 GET /api/v1/settings 和 PUT /api/v1/settings/map-keys。

使用官方最小 JSAPI 初始化验证：构建 Web 资源和 Go 二进制后打开 /bmap-smoke.html 或 /amap-smoke.html；主应用通过官方 @amap/amap-jsapi-loader 加载高德 JS API 2.0。高德路线由服务端 Web API 路径规划 2.0 生成，地图 Provider 切换不会复用另一家的路线 geometry；当前显示 Provider 没有对应路线时必须显式生成。

## Docker

~~~powershell
$env:JOURNEYIN_AUTH_USERNAME = 'admin'
$env:JOURNEYIN_AUTH_PASSWORD = '<strong-login-password>'
$env:JOURNEYIN_MCP_TOKEN = '<strong-random-token>'
$env:JOURNEYIN_AMAP_JS_KEY = '<amap-web-js-key>'
$env:JOURNEYIN_AMAP_SERVER_KEY = '<amap-webservice-key>'
$env:JOURNEYIN_AMAP_SECURITY_JS_CODE = '<amap-js-security-code>'
docker compose up --build -d
~~~

数据默认保存于 Docker named volume `journeyin-data`；如果设置 `JOURNEYIN_DATA_PATH`，则保存于指定的宿主机目录或 named volume。生产环境请将服务放在 HTTPS 反向代理后，并注入地图服务商的 Key；使用高德 JS API 2.0 时还需配置 JOURNEYIN_AMAP_SECURITY_JS_CODE，服务端会通过 /_AMapService/ 仅代理受限的高德 Web API 路径，不代理瓦片或任意主机。

## 开源协议

JourneyIn 采用 [MIT License](LICENSE) 开源协议。完整许可文本见仓库根目录的 [`LICENSE`](LICENSE) 文件。

## 当前阶段

已完成 Go 单二进制基础、嵌入式资源、Trip 校验、SQLite revision、百度与高德 POI/地理编码/路线/天气 Provider、百度/高德导航 URI、百度 BMap 与高德 JS API 2.0 Web 地图、Provider 切换、跨 CRS 路线显示、全屏地图浮窗、子规划点、天气快照、持久化只读分享、Trip 同步 push/pull、MCP resources 和 Docker 配置。路线按相邻规划点分段；同一逻辑路线段可保留多个 Provider/mode 快照，切换 Provider 不复用另一家 geometry；地图 API 访问受 SQLite cache、Provider 串行并发控制和每日上限保护。

方案与实现约束：

- plan.md
- AGENTS.md
