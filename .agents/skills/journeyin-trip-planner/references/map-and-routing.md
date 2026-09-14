# 地图能力、地理编码与路线规划全规范 (map-and-routing.md)

本规范定义 `journeyin-trip-planner` 在行程规划中的**地图提供方优先级（高德优先、百度备用）、真实可执行地理编码工具、坐标系管理、双平台导航链接生成、真实路线快照与天气获取**标准。

---

## 1. 地图提供方核心策略（高德优先，双轨共存）

JourneyIn 整体架构严格贯彻**高德地图优先（AMap First）**原则：

1. **首选提供方**：**高德地图 (AMap)** 为系统第一默认地图、地理编码与算路提供方；
2. **备选 / 补充提供方**：**百度地图 (Baidu Map)** 保持完整兼容与支持，作为第二提供方并列共存；
3. **双平台友好**：最终生成的规划点 Markdown 中，同时提供高德地图与百度地图的快捷打开入口（高德在前、百度在后），满足用户在不同手机与环境下的导航需求；
4. **行程地图配置**：生成的 Canonical Trip JSON 中，默认指定：

```json
   "map": {
     "preferred_provider": "amap",
     "enabled_providers": ["amap", "baidu"],
     "default_mode": "walking"
   }
```

---

## 2. 真实地理编码与地点解析执行方式 (Geocoding & POI)

对筛选出的每一个规划点，必须通过**真实可执行的工具或接口**获取权威地理信息，严禁捏造。

### 2.1 高德地图真实执行方式（首选，三种途径任选其一）

根据环境准备情况，按以下真实途径执行高德地点解析：

#### 途径 A：通过官方 MCP 工具（推荐）

若环境中已挂载官方 MCP 服务 `@amap/amap-maps-mcp-server`：
- **关键字搜索 POI**：调用 `maps_text_search(keywords: "鼓浪屿日光岩", city: "厦门")`
  - 读取返回结果：`id`（保存为 `amap_poiid`）、`name`、`location`（如 `"118.067341,24.445218"`）、`address`、`adcode`。
- **结构化地址编码**：调用 `maps_geo(address: "思明区晃岩路62号", city: "厦门")`
  - 读取返回结果：`location`、`adcode`、`formatted_address`。

#### 途径 B：通过高德官方 Web 服务 REST API（直连）

若具备网络调用或脚本执行能力且持有高德 Web 服务 Key：
- **POI 搜索**：`GET https://restapi.amap.com/v3/place/text?key={KEY}&keywords={点名}&city={城市}&offset=5&page=1`
- **地理编码**：`GET https://restapi.amap.com/v3/geocode/geo?key={KEY}&address={地址}&city={城市}`
- **解析数据**：将返回的 `location` 字符串（按逗号拆分为 `lng` 和 `lat`，均为浮点数）。

#### 途径 C：通过 JourneyIn 本地服务内置代理（免 Key）

若本地 JourneyIn 服务已启动且设置中已配置高德 Key：
- **POI 搜索**：`POST http://127.0.0.1:8080/api/v1/maps/poi`，Body: `{"provider": "amap", "query": "日光岩", "region": "厦门"}`
- **地理编码**：`POST http://127.0.0.1:8080/api/v1/maps/geocode`，Body: `{"provider": "amap", "address": "日光岩", "city": "厦门"}`

### 2.2 百度地图真实执行方式（备选）

当高德不可用或需要百度双平台补全时：
- **百度 Agent Plan**：使用 `bmap-cli` 或加载 `baidu-ai-map` Skill，调用语义化检索工具；
- **百度官方 MCP**：调用 `@baidumap/mcp-server-baidu-map` 工具；
- **百度 WebAPI**：调用 `GET https://api.map.baidu.com/place/v2/search` 或 `geocoding/v3`；
- 读取结果：提取 `location.lat`、`location.lng`（BD-09LL 坐标）和 `uid`（保存为 `baidu_uid`）。

### 2.3 消歧与多候选处理原则（严禁盲选）

- 当搜索返回多个同名地点、多个景区出入口（如“南门”、“东门游客中心”）或不同分店时：
  - **绝对禁止静默使用第 1 条结果**；
  - 必须在规划草案或审阅阶段向用户展示候选名称、完整地址及坐标，由用户明确选择。

### 2.4 坐标缺失的处理红线

- 若某个规划点因地名模糊、偏僻无法获取可靠坐标：
  - **严禁凭空捏造经纬度或使用 (0, 0)**；
  - 必须主动告知用户该地点无法定位，并询问：
    1. 提供更准确的别名或详细地址重新检索；
    2. 或用户明确许可“**本次以无坐标 Draft 形式先保存该点**”。
  - 未获用户许可，不得将无坐标点直接混入正常规划点落库。

---

## 3. 坐标系 (CRS) 规范与数据结构

JourneyIn 严格区分坐标系，禁止通过数字外观猜测 CRS：

- **基准推荐坐标系**：`gcj02`（高德、腾讯及主流国内地图的通用国标加密坐标系）；
- **百度坐标系**：`bd09ll`；
- **通用国际坐标系**：`wgs84`。

### 规范 Location 数据结构示例

符合 `schemas/trip.v1.json` 的规范节点：

```json
{
  "preferred": "gcj02",
  "coordinates": {
    "gcj02": {
      "lat": 24.445218,
      "lng": 118.067341,
      "crs": "gcj02"
    },
    "bd09ll": {
      "lat": 24.451025,
      "lng": 118.073892,
      "crs": "bd09ll"
    }
  },
  "source": "amap",
  "provider_refs": {
    "amap_poiid": "B000A85678",
    "baidu_uid": "7c86a12345678"
  },
  "geocoded_at": "2026-10-01T08:30:00Z",
  "precision": "exact",
  "confidence": 0.95
}
```

> **坐标字段拆分说明**：从高德接口返回的 `location: "118.067341,24.445218"`，逗号前是经度 (`lng = 118.067341`)，逗号后是纬度 (`lat = 24.445218`)，存入 JSON 时务必分别写入 `lng` 和 `lat`，不得反转。

---

## 4. 双地图导航与定位链接生成（高德优先）

每个规划点的 Markdown 说明中，必须生成安全、有效的地图定位入口，**高德在前，百度在后**。

### 4.1 高德地图真实链接（首选导航入口）

高德地图 URI 规范中，**经纬度顺序为：经度在前，纬度在后 (`position=lng,lat`)**。

- **移动端与 App 唤起（推荐主入口）**：

```text
  https://uri.amap.com/marker?position={lng},{lat}&name={URL编码名称}&src=journeyin-trip-planner&coordinate=gaode&callnative=1
```

  *说明*：在手机浏览器中打开可自动尝试唤起高德地图 App 并定位至该点；在 PC 浏览器中自动展示高德网页版。
- **纯网页查看回退**：

```text
  https://uri.amap.com/marker?position={lng},{lat}&name={URL编码名称}&src=journeyin-trip-planner&coordinate=gaode&callnative=0
```

### 4.2 百度地图真实链接（备用导航入口）

百度地图 URI 规范中，**经纬度顺序为：纬度在前，经度在后 (`location=lat,lng`)**。

- **百度地图 HTTPS 网页回退（推荐）**：

```text
  https://map.baidu.com/marker?location={lat},{lng}&title={URL编码名称}&content={URL编码地址}&output=html&src=journeyin-trip-planner
```

- **百度地图 App Scheme（仅在 Markdown 中以代码文本提供）**：

```text
  baidumap://map/marker?location={lat},{lng}&title={URL编码名称}&content={URL编码地址}&coord_type=bd09ll&src=journeyin-trip-planner
```

### 4.3 链接排版格式示范

在规划点 Stop 的 Markdown 正文【地图与导航】小节中，标准排版如下：

```markdown
### 地图与导航
- 📍 **高德地图定位**：[在高德地图中打开](https://uri.amap.com/marker?position=118.067341,24.445218&name=%E6%97%A5%E5%85%89%E5%B2%A9&src=journeyin-trip-planner&coordinate=gaode&callnative=1)（支持 App 唤起）
- 📍 **百度地图定位**：[在百度地图中打开](https://map.baidu.com/marker?location=24.451025,118.073892&title=%E6%97%A5%E5%85%89%E5%B2%A9&content=%E8%8B%97%E6%B5%AA%E5%B1%BF%E6%99%AF%E5%8C%BA&output=html&src=journeyin-trip-planner)
```

---

## 5. 路线设计与真实路线快照生成 (Routing)

路线规划负责连接同一天内相邻的规划点。

### 5.1 顺序原则与智能优化建议

- 默认保留用户认可或按游览时序编排的 Stop 顺序；
- 如果 Agent 发现既有顺序存在绕路或走回头路：
  - 提出“优化调整建议”，列出原动线与建议动线的时间和距离对比；
  - 必须由用户确认后方可调整 Stop sequence，禁止未经同意自行乱序。

### 5.2 调用 JourneyIn MCP 路线规划 (`plan_trip` / `refresh_routes`)

行程在 JourneyIn 中 commit 成功后，若用户请求生成路线，调用 MCP 工具生成真实路线快照：

```json
{
  "trip_id": "commit返回的trip_id",
  "expected_revision": 1,
  "provider": "amap",
  "mode": "walking"
}
```

- **提供方参数**：**默认传 `provider: "amap"`**；若用户明确要求百度，传 `"baidu"`；
- **出行模式**：支持 `"walking"`（步行）、`"driving"`（驾车）、`"transit"`（公交）；
- **版本控制**：每次调用必须传入最新的 `expected_revision`；
- **真实 Geometry 原则**：
  - 真实道路几何由服务端提供方返回并持久化在 `Day.legs[].snapshots` 中；
  - 某段道路无法算路时，如实保持未生成状态，**严禁使用起终点直线冒充真实路线**。

---

## 6. 天气快照获取与展示规范 (Weather)

天气信息具有时效性，必须标注来源与获取时间：

1. **真实高德天气获取途径**：
   - 官方 MCP 工具：调用 `maps_weather(city: "adcode或城市名")`；
   - 官方 Web API：`GET https://restapi.amap.com/v3/weather/weatherInfo?city={adcode}&extensions=all|base&key={KEY}`；
   - JourneyIn 本地 API：`POST /api/v1/maps/weather` 带 `adcode`；
   - 真实指标覆盖：实时天气观测（当前气温、实时天况、湿度、风向风力）与多日预报（最低温 ~ 最高温区间）。
2. **数据结构**：
   在 Stop 或 Day 中记录：

```json
   {
     "source": "amap",
     "forecast_date": "2026-10-01",
     "condition": "多云转晴",
     "temp_min_c": 19,
     "temp_max_c": 27,
     "current_temp_c": 23,
     "current_condition": "多云",
     "humidity_percent": 65,
     "wind_direction": "东南风",
     "wind_power": "3级",
     "fetched_at": "2026-10-01T08:00:00Z"
   }
```

3. **远期出行诚实处理**：
   若行程日期超出地图 API 的有效预报窗口（通常为 4~7 天），在文档中注明“暂无实时天气预报，出行前 3 天建议再次刷新”，**禁止编造虚假气温**。
