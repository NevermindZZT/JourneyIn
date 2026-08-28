# JourneyIn 验收说明

## 1. 本地启动

~~~powershell
$env:JOURNEYIN_DATA_DIR = "D:/data/journeyin/journeyin.db"
$env:JOURNEYIN_LISTEN = "127.0.0.1:8080"
$env:CGO_ENABLED = "0"
go run ./cmd/journeyin
~~~

打开 http://127.0.0.1:8080。

验收时不要在每次构建前删除 data 目录。建议将 JOURNEYIN_DATA_DIR 设置为绝对路径；如果不设置，程序默认使用用户配置目录中的 JourneyIn/journeyin.db，并会兼容读取当前工作目录或可执行文件旁已有的 data/journeyin.db。

## 2. 本地开发和构建验证（优先）

本地开发不需要 Docker，推荐前后端分两个进程运行。

终端 A：

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:GOTELEMETRY = 'off'
$env:CGO_ENABLED = '0'
go run ./cmd/journeyin -listen 127.0.0.1:8080 -data D:/data/journeyin/journeyin.db
~~~

终端 B：

~~~powershell
cd web
pnpm install
pnpm run dev
~~~

打开 http://127.0.0.1:5173。Vite 会把 /api 和 /mcp 代理到 127.0.0.1:8080。

如果当前开发环境阻止 esbuild 安装脚本，可先执行以下命令完成依赖安装和类型验证；该方式不会生成 production bundle：

~~~powershell
pnpm install --ignore-scripts
& '.\node_modules\.bin\vue-tsc.cmd' --noEmit
~~~

本地 production 构建顺序：

~~~powershell
cd ..
pnpm --dir web install --frozen-lockfile
pnpm --dir web typecheck
pnpm --dir web build
$env:CGO_ENABLED = '0'
go test ./...
go vet ./...
go build -trimpath -o dist/journeyin.exe ./cmd/journeyin
~~~

构建后直接运行 dist/journeyin.exe，再打开 http://127.0.0.1:8080。若只验证当前嵌入 fallback，也可以不执行 Web build，因为 web/dist/index.html 已包含可用的基础页面。

自动检查运行：

~~~powershell
pwsh -File scripts/verify-local.ps1 -ServerUrl http://127.0.0.1:8080 -WebUrl http://127.0.0.1:8080
~~~

本地人工检查清单：

- [ ] 页面不再显示“Web 资源构建完成后，此页面将由 Vue/Ionic 应用替换”。
- [ ] 点击右上角 + 能打开新建旅行规划窗口，提交后生成 draft 并出现在列表。
- [ ] 点击设置按钮能打开设置页，显示百度/高德 Key 状态、申请链接和环境变量配置方法。
- [ ] 默认主题跟随系统；切换浅色/深色后立即生效，刷新页面仍保持选择。
- [ ] 开启 API Token 的服务端返回 401 时自动显示登录窗口；令牌只保存在本机浏览器。
- [ ] 页面显示全屏地图；行程、搜索和详情显示为可隐藏的浮窗卡片。
- [ ] 点击右上角面板按钮可以隐藏/恢复行程浮窗。
- [ ] 点击右上角 + 可以创建 draft Trip。
- [ ] 创建 Trip 后点击“添加地点”，搜索结果显示候选地点、地址、BD-09LL 坐标。
- [ ] 选择候选地点后，规划点列表立即增加，地图显示已保存 Marker。
- [ ] 添加两个规划点后点击“生成路线”，路线按相邻点生成并保存到 Trip。
- [ ] 刷新或重启服务后，规划点和路线快照仍存在，不重新调用搜索接口。
- [ ] 导入一个 Trip JSON 后，列表显示天数和规划点数量。
- [ ] 点击规划点能打开详情、日期、Markdown、天气、天气更新时间和参考链接。
- [ ] 详情中可以搜索并添加多个子规划点；子规划点显示自己的日期、坐标和天气入口。
- [ ] 没有浏览器端 Key 时显示明确降级，不绘制假路线。
- [ ] 配置百度浏览器端 Key 后加载真实地图和 BD-09LL Marker。
- [ ] 地图 HUD 的“卫星图”按钮切换到卫星图，再切回“标准图”；切换不产生 POI、路线或天气请求。
- [ ] 地图默认显示规划点名称和日期 Label；点击“隐藏标签”后 Label 消失，点击“显示标签”后恢复。
- [ ] 路线存在距离/耗时信息时，标签开启后显示每段路线的长度和耗时。
- [ ] 行程列表与选中行程详情分层显示；点击“‹ 行程列表”可以返回列表。
- [ ] 行程总体说明有“编辑行程说明”入口，可以保存二次修改。
- [ ] 规划点说明有“编辑地点说明”入口，并可点击“全屏编辑”进入全屏输入状态。
- [ ] 窄屏详情卡片滚动时，滚动条保持在圆角卡片内部，不突破卡片边界。
- [ ] 导出 JSON 能重新导入并保持 revision/内容。
- [ ] 创建分享后只读页面可访问，share token 不出现在服务端日志。

## 3. 地图规划验收

推荐验证顺序：

1. 新建一个 1 天 draft Trip。
2. 在行程浮窗点击“添加地点”。
3. 输入“西湖”，区域输入“杭州市”，点击“搜索地点”。
4. 从候选中选一个明确地点，点击“添加”；不要默认接受模糊同名地点。
5. 再搜索并添加第二个地点。
6. 选择步行/驾车/骑行/公交，点击“生成路线”。
7. 在主规划点详情中添加至少两个子规划点；关闭详情时子点不显示在地图，重新打开详情时子点 Marker 显示。
8. 点击“获取天气”，确认天气条件/温度和 fetched_at 显示；6 小时内再次刷新不应产生新的上游天气请求。
9. 导出 JSON，检查每个 Stop 的 location 中有 preferred、coordinates、crs、source/provider_refs；检查 children 和 Day legs 中有 route snapshot geometry。
10. 重启同一个二进制并使用同一个绝对数据路径，确认行程点、子点、天气和路线仍在。

查询次数原则：

- 搜索仅在点击提交时执行，不监听输入框逐字请求。
- 选择候选后直接保存候选返回的地址、坐标和 UID，不再次地理编码。
- 只有显式点击“生成路线”才算路；重复相同请求优先命中 SQLite cache。
- `JOURNEYIN_MAP_MAX_CONCURRENCY` 默认 2；设置 `JOURNEYIN_MAP_DAILY_LIMIT` 可在本地增加每日网络调用上限。

## 4. 地图配置

百度 WebAPI 与 BMapGL 使用不同的 Key 配置：

~~~powershell
$env:JOURNEYIN_BAIDU_SERVER_AK = "<server-ak>"
$env:JOURNEYIN_BAIDU_BROWSER_AK = "<browser-ak>"
~~~

浏览器端 Key 必须在百度控制台配置正确的域名白名单；百度和高德服务端 Key 可以通过环境变量或设置页写入 SQLite。建议配置 `JOURNEYIN_AMAP_SERVER_KEY` 后，在设置的“地点检索”中选择高德优先；Provider 不可用时会自动回退另一家。没有可用服务端 Key 时页面会显示降级模式，不会生成假路线。

官方百度 JSAPI 4.0 / legacy BMapGL 最小 smoke test：

~~~text
http://127.0.0.1:8080/bmap-smoke.html
~~~

页面会输出 sdk、Map、Point、WebGL 和 tiles 状态。若 sdk 为 undefined 或 Map 不可用，应优先检查浏览器端 AK、当前 host 白名单（localhost 与 127.0.0.1 需分别配置）、JSAPI WebGL 服务和浏览器控制台错误。

高德导航无需配置路线服务 Key 即可验证 HTTPS URI/native scheme；高德路线、POI 和天气 Provider 后续按官方账号能力补充。

设置页 Key 持久化验收：

1. 打开设置，填写浏览器端 Key 和服务端 Key 测试值。
2. 点击“保存地图 Key 到数据库”。
3. 确认浏览器端 Key 立即生效，服务端 Key 只显示“已配置”。
4. 重启 JourneyIn，重新打开设置，确认状态仍然存在。
5. 确认服务端 Key 不出现在设置读取响应、Trip JSON、分享 JSON 和日志中。

## 5. REST 验收

创建 Trip：

~~~powershell
$payload = @{
  schema_version = 1
  title = '验收行程'
  status = 'draft'
  timezone = 'Asia/Shanghai'
  date_range = @{ start = '2026-04-18'; end = '2026-04-18' }
  days = @(@{ id = 'day-1'; date = '2026-04-18'; stops = @(@{ id = 'stop-1'; sequence = 1; title = '西湖' }) })
} | ConvertTo-Json -Depth 10
$trip = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/import -ContentType application/json -Body $payload
$trip
~~~

验证列表、详情、导出：

~~~powershell
Invoke-RestMethod http://127.0.0.1:8080/api/v1/trips
Invoke-RestMethod http://127.0.0.1:8080/api/v1/trips/$($trip.id)
Invoke-WebRequest http://127.0.0.1:8080/api/v1/trips/$($trip.id)/export.json -OutFile trip-export.json
~~~

更新时使用：

~~~text
If-Match: revision-1
~~~

使用旧 revision 更新应返回 409。

## 6. MCP 验收

远程 MCP 地址：

~~~text
http://127.0.0.1:8080/mcp
~~~

远程监听时先设置：

~~~powershell
$env:JOURNEYIN_MCP_TOKEN = '<strong-random-token>'
$env:JOURNEYIN_LISTEN = '0.0.0.0:8080'
go run ./cmd/journeyin
~~~

Agent 应按以下顺序操作：

~~~text
journeyin.get_capabilities
读取 journeyin://schema/trip/v1
journeyin.validate_trip
journeyin.preview_save_trip
展示摘要、diff、warning 并请求用户确认
journeyin.commit_save_trip
~~~

本地 stdio：

~~~text
journeyin mcp stdio
~~~

完整规则见 .agents/skills/journeyin-save-trip/SKILL.md。

## 7. 分享验收

~~~powershell
$share = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/shares -ContentType application/json -Body (@{ trip_id = $trip.id; ttl_seconds = 3600 } | ConvertTo-Json)
Invoke-WebRequest $share.url
Invoke-WebRequest ($share.url + '.json')
~~~

分享页必须只读，不能使用 share URL 调用 Trip 写 API；日志中只应出现 /s/<redacted>。

## 8. 同步验收

~~~powershell
$change = @{
  changes = @(@{
    change_id = 'change-acceptance-1'
    aggregate_id = 'trip-sync-acceptance'
    device_id = 'device-a'
    operation = 'upsert'
    base_revision = 0
    new_revision = 1
    hash = 'payload-hash-1'
    payload = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($payload))
  })
} | ConvertTo-Json -Depth 10
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/sync/push -ContentType application/json -Body $change
Invoke-RestMethod http://127.0.0.1:8080/api/v1/sync/pull?cursor=0
~~~

相同 change_id 重试必须幂等；错误 base_revision 应返回 revision_conflict。

## 9. 自动化检查

~~~powershell
$env:GOMODCACHE = Join-Path $env:TEMP 'journeyin-gomod'
$env:GOCACHE = Join-Path $env:TEMP 'journeyin-gocache'
$env:CGO_ENABLED = '0'
go test ./...
go vet ./...
cd web
pnpm install --ignore-scripts
& '.\\node_modules\\.bin\\vue-tsc.cmd' --noEmit
~~~

生产构建还需要在能够运行 esbuild 子进程的普通开发机/CI 执行：

~~~text
pnpm --dir web build
~~~

## 10. 已知边界

- 百度真实路线/天气调用需要有效的服务端 AK；BMapGL 需要有效浏览器端 AK 和域名白名单。
- 当前高德 Provider 提供导航 URL 适配和明确的未配置降级，路线/POI/天气聚合需单独接入高德账号能力。
- Docker 构建依赖本机 Docker daemon 和网络；当前受限环境可能无法访问 Docker Desktop。
- 移动端已提供 Capacitor 配置和包装说明，但 Android/iOS 原生工程签名、商店配置和真机验证需要在对应平台完成。
