# 环境探测与依赖配置指南 (env-setup.md)

本指南为 `journeyin-trip-planner` 在开始旅行规划前执行的**环境探测、依赖检查、配置引导与工具安装**标准规范。
所有工具和接口均为**官方真实发布且可直接执行的工具与接口**。

---

## 1. 核心依赖能力矩阵

| 能力维度 | 真实工具 / Skill / 接口 | 运行方式 | 核心用途 |
|---|---|---|---|
| **JourneyIn 核心 MCP** | `journeyin` MCP Server | stdio 或 HTTP (`/mcp`) | 数据校验 (`validate_trip`)、保存预览 (`preview_save_trip`)、提交行程 (`commit_save_trip`)、路线规划 (`plan_trip`) |
| **旅行保存原子规范** | `journeyin-save-trip` Skill | Agent Skill | 规范 Canonical Trip JSON 结构、Gate 确认与安全约束 |
| **互联网与小红书检索** | `agent-reach` CLI / MCP | `agent-reach doctor` / `opencli` / `mcporter` | 小红书 5+2 核心维度真实检索、笔记正文提取、全网多源补充 |
| **高德地图（首选地图能力）** | **官方 MCP Server**：`@amap/amap-maps-mcp-server` | `npx -y @amap/amap-maps-mcp-server` (需 `AMAP_MAPS_API_KEY`) | 提供真实可调用的 `maps_geo`、`maps_text_search`、`maps_weather` 等标准 MCP 工具 |
| **高德地图（备用接口 1）** | **高德官方 Web 服务 REST API** | HTTP GET (`https://restapi.amap.com/v3/...`) | 直接调用地理编码 (`/v3/geocode/geo`)、POI 搜索 (`/v3/place/text`)、天气 (`/v3/weather/weatherInfo`) |
| **高德地图（备用接口 2）** | **JourneyIn 本地地图代理 API** | HTTP POST (`http://127.0.0.1:8080/api/v1/maps/...`) | 通过 JourneyIn 服务端已保存的高德 Key 代理请求地理编码与 POI |
| **百度地图（备选地图能力）** | `bmap-cli` + `baidu-ai-map` / `@baidumap/mcp-server-baidu-map` | CLI `bmap-cli-windows-amd64.exe` 或百度 MCP | 百度 Agent Plan 语义化地点、路线、天气，BD-09LL 坐标与百度 UID |

---

## 2. 高德地图真实环境探测与配置（首选）

高德地图开放平台官方发布了标准的官方 MCP Server，同时也提供了稳定的 Web 服务 REST API。

### 2.1 高德官方 MCP Server 配置 (`@amap/amap-maps-mcp-server`)

高德开放平台官方发布的 npm 包为 `@amap/amap-maps-mcp-server`（bin 命令：`mcp-amap`）。

#### 配置方式（在 MCP 客户端配置中添加）

```json
{
  "mcpServers": {
    "amap-maps": {
      "command": "npx",
      "args": ["-y", "@amap/amap-maps-mcp-server"],
      "env": {
        "AMAP_MAPS_API_KEY": "<高德Web服务API_KEY>"
      }
    }
  }
}
```

#### 提供的真实 MCP 工具清单及入参

- `maps_geo`：地址转经纬度（支持名胜景区、建筑物）
  - 入参：`{ "address": "详细地址或地标名", "city": "可选指定城市" }`
  - 返回：格式化地址、经纬度（经度在前，纬度在后 `lng,lat`）、行政区划代码 (adcode)
- `maps_text_search`：关键字搜索 POI
  - 入参：`{ "keywords": "景点或餐厅名", "city": "城市名", "types": "可选POI类型" }`
  - 返回：POI 列表（包含 `id` 即 `amap_poiid`、`name`、`location`、`address`、`adcode`）
- `maps_regeocode`：经纬度转地址与行政区划
  - 入参：`{ "location": "经度,纬度" }`
- `maps_weather`：城市天气查询
  - 入参：`{ "city": "城市名称或标准adcode" }`
- `maps_around_search`：周边搜索 POI
  - 入参：`{ "location": "经度,纬度", "keywords": "关键词", "radius": 3000 }`
- `maps_direction_walking` / `maps_direction_driving`：步行/驾车路线规划
  - 入参：`{ "origin": "经度,纬度", "destination": "经度,纬度" }`

### 2.2 高德官方 Web 服务 REST API（真实直接端点）

当环境中未挂载高德 MCP，但具备网络请求能力或脚本工具时，可直接向高德开放平台官方端点发起 GET 请求：

1. **地理编码 (Geo)**：

```text
   GET https://restapi.amap.com/v3/geocode/geo?key={KEY}&address={地址/点名}&city={城市}
```

   - 真实响应：`geocodes[0].location` (例如 `"118.067341,24.445218"`)、`adcode` (如 `"350203"`)。
2. **POI 关键字搜索 (Place Text)**：

```text
   GET https://restapi.amap.com/v3/place/text?key={KEY}&keywords={点名}&city={城市}&offset=10&page=1
```

   - 真实响应：`pois[0].id` (高德 POI ID)、`pois[0].name`、`pois[0].location`、`pois[0].address`。
3. **天气查询 (Weather Info)**：

```text
   GET https://restapi.amap.com/v3/weather/weatherInfo?key={KEY}&city={adcode}&extensions=all
   GET https://restapi.amap.com/v3/weather/weatherInfo?key={KEY}&city={adcode}&extensions=base
```

   - `extensions=base`：返回 `lives[0]` 实时气象（气温 `temperature`、天气 `weather`、湿度 `humidity`、风向风力）；
   - `extensions=all`：返回 `forecasts[0].casts[]` 多日预报（白天最高温 `daytemp`、夜间最低温 `nighttemp`）。

### 2.3 JourneyIn 本地服务代理接口

若本地已启动 JourneyIn 服务（默认 `http://127.0.0.1:8080`），且设置中已保存高德 Key，可免 Key 调用内置代理接口：
- `POST /api/v1/maps/geocode`：`{ "provider": "amap", "address": "日光岩", "city": "厦门" }`
- `POST /api/v1/maps/poi`：`{ "provider": "amap", "query": "厦门大学", "region": "厦门" }`
- `POST /api/v1/maps/weather`：`{ "provider": "amap", "request": { "adcode": "350203" } }`

### 2.4 缺少高德 Key 时的获取与配置指引

1. 前往[高德开放平台控制台](https://console.amap.com/)；
2. 注册并进入“应用管理” → “我的应用” → “创建新应用”；
3. 点击“添加 Key”，**服务平台务必选择“Web 服务”**；
4. 配置到当前环境：
   - 若使用官方 MCP：将 Key 填入配置文件的 `AMAP_MAPS_API_KEY` 环境变量；
   - 若使用 JourneyIn：在 JourneyIn 网页端“设置” → “地图设置”卡片中填入并保存；或设置环境变量 `AMAP_WEB_KEY`。

---

## 3. 百度地图真实环境探测与配置（备选）

百度地图作为备选与双平台定位补充：

### 3.1 百度 CLI 与 Agent Plan

- Windows 探测路径：`C:\Users\Never\bin\bmap-cli-windows-amd64.exe`；
- 执行探测：`& "C:\Users\Never\bin\bmap-cli-windows-amd64.exe" ap list`；
- 使用 `baidu-ai-map` Skill 执行语义化搜索。

### 3.2 百度官方 MCP Server

- npm 官方包：`@baidumap/mcp-server-baidu-map`。

---

## 4. 检索工具 agent-reach 真实体检

必须先在终端执行体检指令：

```bash
agent-reach doctor --json
```

- 读取 `xiaohongshu.active_backend`（`opencli` 或 `xiaohongshu-mcp`）；
- 未登录时引导用户执行 `opencli xiaohongshu login` 完成扫码。

---

## 5. 缺失依赖 Skill 的搜索与安装审批规范

1. 优先搜索项目与官方作者仓库；
2. 安装前必须向用户展示拟安装包名、官方来源 URL、权限范围，经用户明确许可后方可执行。
