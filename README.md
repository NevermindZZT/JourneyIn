# JourneyIn

JourneyIn 是一个以地图为核心的旅行规划项目。当前版本提供 Go + SQLite 服务端、Vue/Ionic Web 客户端、Trip JSON、地图 Provider、只读分享、同步接口、MCP 和 Docker 配置。

## 本地运行

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:CGO_ENABLED = '0'
go run ./cmd/journeyin -listen 127.0.0.1:8080 -data D:/data/journeyin/journeyin.db
~~~

打开 http://127.0.0.1:8080 。

构建和重新运行不会删除数据库。若省略 `-data` 与 `JOURNEYIN_DATA_DIR`，默认使用用户配置目录中的 `JourneyIn/journeyin.db`；程序也会兼容读取当前目录、可执行文件目录或其上级目录下已有的 `data/journeyin.db`。验收时建议始终使用固定绝对路径。

如果省略 `-data` 和 `JOURNEYIN_DATA_DIR`，Windows 默认保存到 `%AppData%/JourneyIn/journeyin.db`；Linux/macOS 使用用户配置目录。构建 Go 二进制不会清理数据库。建议验收时固定使用绝对数据路径。

默认 localhost 运行时 MCP 不强制 Token；监听非 localhost 地址时必须设置 JOURNEYIN_MCP_TOKEN。配置 JOURNEYIN_API_TOKEN 后，除 health 外的 REST API 需要 Bearer Token。

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

打开 http://127.0.0.1:5173。web/vite.config.ts 已配置 /api 和 /mcp 到 8080 的代理。

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
4. 从候选结果中明确选择一个地点并点击“添加”；Trip 会保存名称、地址、BD-09LL 坐标、CRS、来源和百度 UID。
5. 打开规划点详情，可以继续关联多个子规划点；只有进入该主规划点详情时，子点 Marker 才会显示在地图上。
6. 添加至少两个规划点后选择路线方式，点击“生成路线”。路线按相邻点分段请求，并将 geometry、距离和耗时保存到 Trip 的 `Day.legs[].snapshots[]`；地图标签开启时会在每段路线中点显示例如 `1.2 km · 10 分钟`。
7. 在规划点详情点击“获取天气/刷新天气”；天气快照写入该点，显示查询时间，6 小时内重复刷新优先使用 SQLite cache。
8. 后续地图重绘直接使用已保存的规划点、子规划点和路线快照，不重复搜索或地理编码。地图 HUD 的“卫星图/标准图”按钮只切换百度 JSAPI 图层，不触发 POI/路线请求。
9. 地图默认显示地点名和行程日期 Label；HUD 可切换“显示标签/隐藏标签”，设置保存在浏览器 localStorage。
10. 行程面板先展示行程列表；点击某条行程后进入独立的选中行程详情，使用“‹ 行程列表”返回，不会把其他行程卡片插入当前行程信息中。行程总体说明和规划点说明都可以二次编辑；规划点说明编辑时可进入全屏输入模式。

规划 API：

~~~text
POST /api/v1/maps/pois/search
POST /api/v1/trips/{trip_id}/days/{day_id}/stops
POST /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}/children
POST /api/v1/trips/{trip_id}/days/{day_id}/stops/{stop_id}/weather
POST /api/v1/trips/{trip_id}/plan
PUT  /api/v1/settings/poi
DELETE /api/v1/settings/place-directory
~~~

默认只在显式规划操作时调用地图服务；POI/geocode/route/weather 由 SQLite cache、同请求 singleflight、并发信号量和可选每日上限控制。配置：

~~~powershell
$env:JOURNEYIN_MAP_MAX_CONCURRENCY = '2'
$env:JOURNEYIN_MAP_DAILY_LIMIT = '0' # 0 表示不在 JourneyIn 内设置上限
$env:JOURNEYIN_AMAP_SERVER_KEY = '<amap-webservice-key>'
~~~

高德服务端 Key 可通过环境变量或设置页保存。高德 POI 结果使用 GCJ-02，保存地点时会同时保留原始 CRS 和用于百度 BMap 显示的 BD-09LL 转换坐标。地点搜索结果进入本地目录后保留 7 天，设置页可以手动清除。

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

百度服务端 Key 使用 JOURNEYIN_BAIDU_SERVER_AK；浏览器端 JSAPI 4.0/BMap（兼容 legacy BMapGL）Key 使用 JOURNEYIN_BAIDU_BROWSER_AK。也可以在设置页填写并保存到 SQLite 的 app_settings 表，服务端 Key 只返回已配置状态，不回显原文。没有 Key 时服务返回明确的 provider_unavailable，不伪造路线。设置接口为 GET /api/v1/settings 和 PUT /api/v1/settings/map-keys。

使用官方最小 JSAPI 初始化验证：构建 Web 资源和 Go 二进制后打开 /bmap-smoke.html。该页面检查 JSAPI 4.0/BMap、legacy BMapGL、Point、WebGL 和地图初始化；如果它失败，说明问题在百度浏览器端 AK、白名单、SDK 网络或浏览器环境，不在 JourneyIn 行程渲染逻辑。

## Docker

~~~powershell
$env:JOURNEYIN_MCP_TOKEN = '<strong-random-token>'
docker compose up --build -d
~~~

数据保存于 Docker volume journeyin-data。生产环境请将服务放在 HTTPS 反向代理后，并注入地图服务商的 Key。

## 当前阶段

已完成 Go 单二进制基础、嵌入式资源、Trip 校验、SQLite revision、百度 POI/地理编码/路线/天气 Provider、百度/高德导航 URI、JSAPI 4.0/BMap Web 地图、标准图/卫星图切换、全屏地图浮窗、子规划点、天气快照、持久化只读分享、Trip 同步 push/pull、MCP resources 和 Docker 配置。路线、子规划点和天气数据都通过 Trip revision 持久化；地图 API 访问受 SQLite cache、并发控制和每日上限保护。

方案与实现约束：

- plan.md
- AGENTS.md
