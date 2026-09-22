---
name: journeyin-trip-planner
description: 完成从需求确认、环境预检、真实资料检索、高德优先地点解析到 JourneyIn 安全保存的全流程旅行规划。
---

# JourneyIn 全流程旅行规划 Skill

## 适用场景

当用户希望从需求确认开始，完成资料检索、旅行方案设计、地点定位、路线生成，并把完整规划写入 JourneyIn 时使用本 Skill。

本 Skill 是 JourneyIn 官方配套的全流程旅行规划总控，**无需依赖任何第三方外部旅行规划 Skill**，自身具备完整的资料检索规范、地理编码、高德优先双地图能力、Markdown 质量检查与两阶段安全保存机制。

---

## 核心原则（不可逾越的红线）

1. **先搜索，后规划**：任何景点、美食、拍照点、住宿区域、交通建议、营业时间和门票信息，必须先有真实互联网来源证据；绝对禁止凭大模型记忆直接捏造事实。
2. **小红书实际深度检索是硬门槛**：资料整理前必须实际调用 `agent-reach` 路由的小红书后端，执行 5+2 核心维度检索。禁止只说“建议去小红书查看”，禁止用普通网页搜索冒充小红书。
3. **高德地图优先，高德/百度双提供方**：
   - 地理编码、POI 检索、坐标系（GCJ-02）、路线规划与天气预报**默认优先使用高德地图能力**；
   - 完整支持**百度地图**作为备选提供方；
   - 规划点详情中同时生成**高德地图**与**百度地图**的双平台打开入口，**高德在前、百度在后**；
   - Trip JSON 中默认指定 `preferred_provider: "amap"`。
4. **先获取可靠坐标，再生成导航链接与路线**：
   - 地址、POI 名称、坐标、坐标系（CRS）、高德 POI ID、百度 UID 必须分离清晰；
   - 严禁猜坐标或伪造经纬度。若遇无法解析的点，必须向用户说明并征得“本次以无坐标 Draft 保存”的明确许可，否则不得擅自写入。
5. **规划点优先且信息完整**：每一个推荐的景点、餐厅、酒店、车站必须成为独立的 Stop，并在 Stop Markdown 中写满推荐理由、游览时间、实用信息、配图、导航入口和来源。
6. **信息全量落库，不丢在聊天中**：住宿方案、预算、交通策略、时间动线、天气、备选方案和检索记录，必须结构化写入 Trip 总体、当天或规划点 Markdown 说明中。
7. **双重确认门槛（Gate A 与 Gate B）**：
   - **Gate A（内容审阅门槛）**：在调用任何 JourneyIn 写入或保存工具前，必须向用户完整展示查询资料和拟写入的所有 Markdown 原文，获得用户明确确认；
   - **Gate B（MCP 预览门槛）**：调用 `validate_trip` 与 `preview_save_trip` 后，再次展示预览摘要、Diff 与警告，经用户最终确认后方可调用 `commit_save_trip`。
8. **严禁粗略纯文本落库**：必须通过严谨的 Markdown 内容检查器，杜绝空洞摘要、敷衍占位符或扁平化纯文本。
9. **真实路线与天气不可伪造**：未生成道路路线快照时如实显示未生成，禁止用起终点直线冒充真实路线；超出预报窗口的天气如实注明“暂无实时预报”，禁止凭空推断温度。
10. **不自动分享**：保存成功后仅落入用户私有数据，不自动创建公开分享链接。

---

## 严格执行状态机

全流程必须按以下状态顺序严格流转，上一阶段未完成前严禁跳入下一阶段：

```text
1. 需求确认 (Requirement Gathering)
   ↓
2. 环境与依赖预检 (Environment Preflight)
   │  ├─ 检查高德地图能力 (首选) 与百度能力 (备选)
   │  ├─ 检查 agent-reach 及小红书后端
   │  └─ 检查 JourneyIn MCP 核心服务
   ↓
3. 真实资料深度检索 (Research & Verification)
   │  ├─ 小红书 5+2 核心维度检索
   │  ├─ 精选笔记精读 (分离来源事实与推断)
   │  └─ 真实图片搜集 (必须稳定直链与来源)
   ↓
4. 地理信息解析与地图链接 (Geocoding & Links)
   │  ├─ 高德优先获取 GCJ-02 坐标与高德 POI ID
   │  ├─ 备选百度获取 BD-09LL 坐标与百度 UID
   │  └─ 生成双平台 App/Web 定位入口 (高德在前)
   ↓
5. 组织完整规划 Markdown 与内容检查 (Markdown Builder & Lint)
   │  ├─ 构造总体、每日与全部规划点完整 Markdown
   │  └─ 执行 Markdown 字符数与结构化检查器
   ↓
6. 【Gate A】规划内容全量展示与用户审阅确认 (Content Review Gate)
   │  └─ 必须获得用户明确答复：“确认内容，可以生成保存预览”
   ↓
7. JourneyIn 安全保存与路线落库 (MCP Save & Plan)
   │  ├─ validate_trip (格式与 Schema 校验)
   │  ├─ preview_save_trip (生成预览快照)
   │  ├─ 【Gate B】向用户汇报预览 Diff，获得最终提交确认
   │  ├─ commit_save_trip (带幂等键提交)
   │  ├─ plan_trip (高德优先生成真实路线快照)
   │  └─ 输出最终成功报告
```

---

## 模块化指南索引

本 Skill 采用模块化架构，各阶段的详细操作指令、代码模板与规范参见子文档：

- ⚙️ **环境与依赖配置**：请参考 [`references/env-setup.md`](./references/env-setup.md)
  - 核心能力矩阵、高德地图 Key 配置与探测、百度 CLI/Agent Plan 探测、agent-reach 体检、缺失 Skill 搜索与安装审批流程。
- 🔍 **资料检索与来源验收**：请参考 [`references/research-workflow.md`](./references/research-workflow.md)
  - 需求确认清单、小红书 5+2 检索命令、精选笔记提炼（事实 vs 推断）、图片直链嵌入与 Gate 0 资料验收门槛。
- 🗺️ **地图能力与路线规范**：请参考 [`references/map-and-routing.md`](./references/map-and-routing.md)
  - 高德优先双轨架构、地理编码规范、坐标系与 Location 结构体、双平台 URI 生成（经纬度顺序校验）、真实路线与天气快照。
- 💾 **Trip JSON 与 MCP 保存**：请参考 [`references/trip-template-and-validation.md`](./references/trip-template-and-validation.md)
  - Canonical Trip JSON 模板、Markdown 完整度硬规则与检查器、Gate A 展示模板、MCP validate/preview/commit 流程与落库验证。

---

## 各阶段执行核心规范

### 阶段 1：需求确认与目标定义

- 必填项目：**目的地**、**旅行天数**、**明确的日期范围 (YYYY-MM-DD)**；
- 扩展记录：出发地、同行人约束、偏好、预算等级、节奏强度、交通方式、时区（默认 `Asia/Shanghai`）。
- 缺漏核心信息时必须主动询问，禁止脑补日期或行程跨度。

### 阶段 2：环境与依赖预检

- 详细指引参见 [`references/env-setup.md`](./references/env-setup.md)；
- 检查 JourneyIn MCP 服务是否连通；
- 运行 `agent-reach doctor --json` 锁定小红书 active backend；
- **地图环境探测**：
  - 优先检测高德地图能力（服务端配置或环境变量）；
  - 若无高德能力，提供高德 Web 服务 Key 申请与配置指引；或询问是否使用百度地图/以无坐标草案进行规划。
- 缺失必需 Skill 时，必须搜索官方来源并征得用户明确同意后方可安装，绝不静默下载。

### 阶段 3：真实资料深度检索

- 详细指引参见 [`references/research-workflow.md`](./references/research-workflow.md)；
- 严格执行小红书 5+2 维度搜索（攻略、景点、美食、拍照、行程 + 住宿区域、推荐酒店）；
- 正文精读，严格区分**来源事实**与**规划推断**；
- 获取真实、公开的图片直链并附带来源；无直链时如实提供参考页面链接，严禁伪造图片 URL；
- 完成 Gate 0 资料验收。

### 阶段 4：高德优先的地点解析与地图链接

- 详细指引参见 [`references/map-and-routing.md`](./references/map-and-routing.md)；
- **首选高德解析（真实可执行工具）**：通过官方 MCP `@amap/amap-maps-mcp-server`（提供 `maps_text_search`、`maps_geo`、`maps_weather`）、高德官方 WebAPI，或 JourneyIn 本地代理接口获取 GCJ-02 坐标、高德 POI ID (`amap_poiid`) 与规范地址；
- **备选百度解析**：通过 `baidu-ai-map`、百度官方 MCP 或百度 WebAPI 获取 BD-09LL 坐标、百度 UID (`baidu_uid`)；
- 同名地点或多入口必须展示候选由用户消歧；
- 生成双平台导航入口：**高德地图在前（经度在前纬度在后，支持唤起 App），百度地图在后（纬度在前经度在后）**。

### 阶段 5：编写完整 Markdown 与质量检查

- 详细指引参见 [`references/trip-template-and-validation.md`](./references/trip-template-and-validation.md)；
- 构造 `Trip.description_markdown`（800+ 字符，6 个二级标题）；
- 构造每日 `Day.notes_markdown`（300+ 字符，早中晚安排、餐饮住宿、天气备选）；
- 构造各点 `Stop.description_markdown`（400+ 字符，理由、时间、门票预约、配图、高德/百度导航、来源）；
- 执行 Markdown 内容检查器，杜绝空洞纯文本。

### 阶段 6：【Gate A】规划内容用户审阅与确认

- **调用任何 JourneyIn 写入工具前，必须执行此门槛**；
- 全量展示检索记录、提取证据、地点坐标结果，以及将要写入的所有 Markdown 原文；
- 向用户提问：“请确认以上查询资料与完整规划内容是否符合预期，是否同意生成 JourneyIn 保存预览？”；
- **未获得用户明确肯定答复前，严禁调用 validate/preview/commit！**

### 阶段 7：JourneyIn 安全校验、预览 (Gate B)、提交与路线落库

- 详细指引参见 [`references/trip-template-and-validation.md`](./references/trip-template-and-validation.md)；
- 构造符合 `schemas/trip.v1.json` 的 Trip JSON（默认 `preferred_provider: "amap"`）；
- 调用 `journeyin.validate_trip` 验证 Schema；
- 调用 `journeyin.preview_save_trip` 生成预览；
- **Gate B 确认**：展示预览 ID、变更摘要与警告，获得最终确认；
- 调用 `journeyin.commit_save_trip`（传入预览 ID、确认 Token 与新生成的 UUID 幂等键）；
- 提交成功后调用 `journeyin.plan_trip`（默认 `provider: "amap"`）生成真实路线快照；
- 验证数据已持久化并向用户汇报。

### 已有行程的局部增补模式

当用户要求为已有行程或既有主/子规划点补充详细介绍、来源、名称、地址、类别或时间窗口时，不要默认重建整趟行程：

1. 先调用 `journeyin.get_trip`，取得 target_trip_id、当前 revision，以及稳定 day_id、stop_id；子规划点还必须确认 parent_stop_id。不得按名称、数组下标或日期猜测目标。
2. 对开放时间、门票、预约、地址、推荐理由等外部事实先完成真实来源核验；把详细说明和来源完整展示给用户，获得 Gate A 确认。
3. 若 capabilities 中 `preview_merge=true` 且 `merge_patch_version >= 2`，使用 `preview_save_trip(operation=merge)` 只提交允许字段：title、address、kind、time_window.arrival/departure（HH:MM）、description_markdown、links；子规划点带 parent_stop_id。
4. 读取 preview 的 diff、warnings 和 preserved；明确告诉用户位置、天气、路线、顺序和日期未被改动。获得 Gate B 确认后才 commit。
5. location、provider_refs、天气、exclude_from_route、日期、顺序、路线和新增/删除规划点不能放进此 merge。需要这些改动时，先说明影响并走完整 replace 预览；绝不猜测坐标或伪造天气/路线。
6. revision 冲突、预览过期或用户改变内容后，重新读取、重新生成 patch 和预览；不得复用旧确认令牌。

---

## 异常处理与恢复速查

| 异常情况 | 恢复策略 |
|---|---|
| **缺少必需 Skill** | 搜索官方来源，展示安装计划与权限范围，征得用户同意后安装；若用户拒绝则说明受阻原因。 |
| **高德 Key 未配置** | 引导用户在高德控制台申请“Web 服务”Key 并在 JourneyIn 设置中保存；或询问是否降级使用百度地图。 |
| **小红书未登录/频控** | 引导用户执行 `opencli xiaohongshu login` 扫码，或暂停并发调用；降级使用官方文旅/马蜂窝等权威渠道并诚实记录。 |
| **地点同名/存在歧义** | 列出候选地点名称、详细地址和坐标，暂停并请用户选择，严禁默认选第 1 条。 |
| **地点无法定位** | 提示用户提供更精准名称；若确实无坐标，必须获得用户明确许可后方可作为无坐标 Draft 保存。 |
| **图片无稳定直链** | 保留官方/百科参考页面链接，在正文中注明“未获取稳定图片直链”，严禁捏造虚假直链。 |
| **Gate A 审阅未通过** | 根据用户意见修改对应 Markdown 或行程安排，重新执行内容检查器并重新请求 Gate A 确认。 |
| **MCP Revision 冲突 (409)** | 重新读取目标行程最新 Revision，对比差异后重新生成预览提交，严禁静默覆盖。 |
| **路线算路失败** | 保持规划点正常保存，如实报告未生成路线快照的航段，严禁用起终点直线冒充真实路线。 |

---

## 任务完成终检验收清单

在向用户宣布规划完成前，逐项核对：

- [ ] **环境预检**：已探测高德地图与百度能力，已运行 `agent-reach doctor`；
- [ ] **真实检索**：小红书 5+2 核心维度均已实际执行并留存证据，每个规划点均有可靠来源；
- [ ] **高德优先**：地点解析以高德为主（GCJ-02 坐标、高德 POI ID），生成的 Trip JSON 中 `preferred_provider` 为 `"amap"`；
- [ ] **双平台导航**：每个规划点 Markdown 中均包含高德地图（排在第一，经度在前纬度在后）与百度地图定位入口；
- [ ] **内容质量**：Markdown 检查器通过，正文富文本结构清晰，不存在空洞纯文本或占位词；
- [ ] **Gate A 确认**：在调用保存工具前，已向用户展示全量资料与 Markdown 并获得明确许可；
- [ ] **安全保存**：严格执行 `validate` → `preview` → **Gate B 确认** → `commit` 流程；
- [ ] **真实路线**：已调用 `plan_trip`（默认 `provider: "amap"`）落库真实路线快照；
- [ ] **私有安全**：没有自动生成公开分享链接，没有泄漏临时 Token、Cookie 或密钥。
