---
name: journeyin-attraction-monitor
description: 使用 JourneyIn 旅行时机雷达与景点监控 MCP 工具，为心愿景点设定监控条件、检索最佳出行窗口（月相/银河/节假日/气象少云），并将良机一键转化为行程草案。
---

# JourneyIn 景点监控与旅行时机雷达 Skill

## 适用场景

当用户提出以下需求时使用本 Skill：
- 想去某个向往的目的地，但不确定什么时间去最合适（“什么时候去冷嘎措最好？”、“推荐最近适合去拍星空的周末”）；
- 希望对特定景点开启长期时机监控（“帮我监控一下莫尔格勒河，有合适周末或假期提醒我”）；
- 查看当前旅行雷达中所有受监控景点的最佳游玩良机（“看看最近雷达有什么推荐的出行窗口”）；
- 将雷达推荐的黄金窗口一键转化为 JourneyIn 的真实行程草案并继续深化规划。

---

## 核心 MCP 工具能力

| MCP 工具名 | 参数 | 职责与说明 |
|---|---|---|
| `journeyin.create_attraction_monitor` | `place_name` (必填), `lat`, `lng`, `crs`, `months`, `stargazing_mode`, `max_leave_days`, `max_cloud_cover`, `max_rain_prob`, `origin_city`, `max_budget` | 创建新的心愿景点监控规则。系统自动每日后台巡检未来 45 天日历、天文月相与天气。 |
| `journeyin.list_attraction_monitors` | `enabled_only` (可选，默认 false) | 列出所有受监控的景点及其最新评定等级 (🔥强烈推荐 / ⭐备选 / ⏳等待)。 |
| `journeyin.get_attraction_monitor` | `monitor_id` (必填) | 获取指定监控项详情以及未来 45 天全部候选窗口时间轴 (含月照率、银河时长、假期与综合得分)。 |
| `journeyin.get_radar_opportunities` | (无) | 获取雷达大盘汇总，筛选出当前所有处于“🔥强烈推荐”的绝佳出行窗口。 |
| `journeyin.convert_monitor_to_trip` | `monitor_id`, `window_start`, `window_end` | 将命中的机会窗口一键转化为 JourneyIn 规范化 Trip 草稿，景点自动置于 D1。 |

---

## 标准工作流

### 流程一：新建景点监控规则
1. **确认目标地点与可靠坐标**：
   - 提取用户期望的景点名称（如“冷嘎措”）；
   - 通过高德或百度地图工具解析真实经纬度与坐标系（如 `lat: 29.58, lng: 101.78, crs: "gcj02"`），严禁凭空编造经纬度。
2. **确认用户的出行约束**：
   - **适游月份**（如 `[6, 7, 8, 9, 10]`）；
   - **是否观星/拍摄银河**（如为摄影设置 `stargazing_mode: true`，系统将自动优先计算新月纯黑夜与夜间银心黑夜时长）；
   - **上班族请假天数上限**（`max_leave_days: 0` 仅限周末/法定假无需请假，`1` 允许请 1 天休 3 天等）；
   - **气象少云容忍度**（如 `max_cloud_cover: 25`）；
   - **出发地与往返预算**（可选）。
3. **调用 `journeyin.create_attraction_monitor`** 创建监控，系统将在后台即时执行未来 45 天的综合评分分析。
4. **向用户反馈监控建立成功**，并告知当前预测的最佳时机。

### 流程二：查询旅行雷达推荐良机
1. 调用 `journeyin.get_radar_opportunities` 获取当前处于推荐状态的目的地；
2. 如果用户指定了某景点，调用 `journeyin.get_attraction_monitor` 查看未来 45 天的打分时间轴；
3. 以结构化表格向用户汇报：
   - **推荐窗口**（如 `2026-06-19 ~ 2026-06-21 端午连休 3 天 · 免请假`）；
   - **月相条件**（如 `新月纯黑夜 · 月照率 4%`）；
   - **银河核心**（如 `夜间黄金观测时段 5.5 小时`）；
   - **天气预报**（如 `晴空少云 · 云量 12%`）；
   - **综合评分**（如 `88.5 分 · 强烈推荐`）。

### 流程三：将推荐良机转化为行程（成行闭环）
1. 当用户认可某推荐窗口时，询问是否生成行程草稿；
2. 获得确认后调用 `journeyin.convert_monitor_to_trip`，传入 `monitor_id`、`window_start` 和 `window_end`；
3. 工具返回新创建的 `trip_id`；
4. 衔接进入 `journeyin-trip-planner`，基于该 Trip 草稿继续深度补全周边的交通动线、次级规划点、酒店餐饮与路网规划。
