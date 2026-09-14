# Trip JSON 规范、Markdown 检查器与 MCP 保存流程 (trip-template-and-validation.md)

本规范定义 `journeyin-trip-planner` 在行程完成规划后的**Canonical Trip JSON 构建标准、Markdown 质量检查器、Gate A 内容审阅、MCP 校验与保存 (Gate B) 及真实路线落库**全流程。

---

## 1. Canonical Trip JSON 标准模板

生成符合 `schemas/trip.v1.json` 的完整文档。遵循**高德地图优先**原则：

```json
{
  "$schema": "https://journeyin.local/schema/trip/v1.json",
  "schema_version": 1,
  "title": "目的地 N 天深度旅行规划",
  "status": "draft",
  "locale": "zh-CN",
  "timezone": "Asia/Shanghai",
  "date_range": {
    "start": "2026-10-01",
    "end": "2026-10-03"
  },
  "description_markdown": "包含目的地概览、行程亮点、住宿安排、预算交通、真实封面图与检索记录的完整富文本 Markdown。",
  "links": [
    {
      "id": "link_guide_01",
      "title": "小红书精选攻略来源",
      "url": "https://www.xiaohongshu.com/explore/...",
      "kind": "source"
    }
  ],
  "map": {
    "preferred_provider": "amap",
    "enabled_providers": ["amap", "baidu"],
    "default_mode": "walking"
  },
  "days": [
    {
      "id": "day_01",
      "date": "2026-10-01",
      "title": "第 1 天：主题与概览",
      "notes_markdown": "当天早中晚行程动线、餐饮、住宿、天气、备选方案与配图的完整 Markdown 说明。",
      "stops": [
        {
          "id": "stop_01_01",
          "sequence": 1,
          "kind": "poi",
          "title": "规划点名称",
          "address": "省市区详细地址",
          "location": {
            "preferred": "gcj02",
            "coordinates": {
              "gcj02": {
                "lat": 24.445218,
                "lng": 118.067341,
                "crs": "gcj02"
              }
            },
            "source": "amap",
            "provider_refs": {
              "amap_poiid": "B000A85678"
            },
            "geocoded_at": "2026-10-01T08:30:00Z"
          },
          "description_markdown": "包含推荐理由、事实推断、时间安排、门票营业信息、配图、高德/百度定位入口与来源出处的完整 Markdown。",
          "links": [
            {
              "id": "link_stop_01",
              "title": "官方页面或来源出处",
              "url": "https://...",
              "kind": "reference"
            }
          ]
        }
      ],
      "legs": []
    }
  ],
  "metadata": {
    "source": "journeyin-trip-planner",
    "planner_version": "0.4.3"
  }
}
```

---

## 2. Markdown 完整度硬规则与检查器

JourneyIn 严禁将规划点写成“一句话纯文本”或把关键信息留在聊天中。在生成 JSON 前，必须通过以下**内容检查器**：

### 2.1 各字段内容指标要求

| 字段 | 结构要求 | 篇幅下限 | 必须包含的核心小节 |
|---|---|---|---|
| **`Trip.description_markdown`** | 至少 6 个二级标题 | 建议 800+ 字符 | 行程亮点与概览、住宿区域与酒店方案、预算估算、总体交通策略、封面配图与来源、小红书检索记录、安全与风险提示 |
| **`Day.notes_markdown`** | 至少 4 个结构化小节 | 建议 300+ 字符 | 当日时序动线（上午/下午/晚上）、特色餐饮安排、当天住宿、交通衔接方式、备选应急方案、当天主题配图 |
| **`Stop.description_markdown`** | 至少 5 个结构化小节 | 建议 400+ 字符 | 推荐理由（区分事实与推断）、游览时间与停留建议、实用信息（门票/预约/开放时间/闭馆日）、配图与来源、**地图与导航（高德优先+百度备选）**、参考来源 |

### 2.2 违规内容拦截项

出现以下情况必须打回重新编写，不得进入保存流程：
- 出现“待补充”、“详见上文”、“暂无”、“略”等敷衍占位符；
- 将规划点仅写成一句话纯文本；
- 只有景点名而无具体门票、开放时间或预约提醒；
- 规划点缺少公开来源链接；
- 包含未经验证的假链接、本地文件路径或私密令牌。

---

## 3. 写入前规划内容审阅门槛 (Gate A)

在调用任何 JourneyIn 写入或保存工具之前，**必须先向用户展示完整的查询资料和拟写入的所有 Markdown 原文，获得明确同意**。

### 3.1 展示内容清单

1. **实际检索证据**：执行的检索关键词、平台、有效笔记、提取的事实与推断；
2. **地点解析结果**：选取的规范名称、地址、坐标、CRS、高德 POI ID；
3. **`Trip.description_markdown` 全文**；
4. **每个 `Day.notes_markdown` 全文**；
5. **每个 `Stop.description_markdown` 全文**；
6. **未解析/待确认项清爽列出**（若有）。

### 3.2 Gate A 确认提问模板

展示完成后，向用户提出明确确认：

> “以上为本次行程规划的全部查询资料、路线逻辑以及拟写入 JourneyIn 的完整总体说明、每日说明和每个规划点的详细内容。请审阅确认：  
> 1. 行程安排、选点与路线动线是否符合您的预期？  
> 2. 是否同意按以上内容生成 JourneyIn 保存预览？”

**通过标准**：必须得到用户明确的首肯（如“确认无误，可以保存”、“好的，请生成预览”）。含糊或未确认时，不得调用保存工具。

---

## 4. MCP 校验与保存两阶段提交 (Gate B)

Gate A 通过后，启动 JourneyIn MCP 标准保存流程。

### 4.1 阶段一：格式校验 (`validate_trip`)

调用 MCP 工具 `validate_trip`：

```json
{
  "trip_json": "完整 Trip JSON 字符串"
}
```

- 检查是否存在 Schema 语法错误、必填字段缺失或非法引用；
- 若返回 errors，在内存中修复后重新调用校验，直至完全通过。

### 4.2 阶段二：创建预览 (`preview_save_trip`)

调用 MCP 工具 `preview_save_trip`：

```json
{
  "trip_json": "已通过校验的 JSON 字符串",
  "operation": "create"
}
```

*(更新既有行程时传 operation: "replace" 或受限 "merge"，并带上 expected_revision)*。

MCP 工具将返回：
- `preview_id`：短期有效的预览 ID；
- `confirmation_token`：短期确认令牌；
- `summary` 与 `diff`：变更统计；
- `warnings`：潜在风险提示。

### 4.3 Gate B 确认与最终 Commit (`commit_save_trip`)

向用户简要汇报预览结果（包含天数、点数、路线段数与有效期限），征得用户对保存执行的最终首肯后调用：

```json
{
  "preview_id": "从 preview 返回的 ID",
  "confirmation_token": "从 preview 返回的 token",
  "idempotency_key": "为本次操作新生成的唯一 UUID 字符串",
  "expected_revision": 0
}
```

- **幂等性保障**：网络超时重试时，必须复用相同的 `idempotency_key`；参数发生变化时必须重新生成预览和新的幂等键。

---

## 5. 真实路线规划落库 (`plan_trip`)

提交成功后，若用户要求路线或规划方案中包含交通动线，调用 MCP 路线规划工具生成服务端路线快照：

```json
{
  "trip_id": "commit 返回的 trip_id",
  "expected_revision": 1,
  "provider": "amap",
  "mode": "walking"
}
```

- **高德优先**：**默认指定 `provider: "amap"`**；
- 若用户指定百度，传 `provider: "baidu"`；
- 可根据每段行程特性分别指定 `mode: "walking"`、`"driving"` 或 `"transit"`；
- 完成后再次调用 `get_trip` 验证路线快照已成功持久化落库。

---

## 6. 最终完成汇报

向用户输出清晰的完工报告：
1. **行程状态**：行程已成功创建/更新并持久化至 JourneyIn；
2. **基本指标**：行程 ID、版本号 (Revision)、天数、规划点总数、真实路线段数；
3. **查看入口**：若 MCP 提供了 view_url，附带展示前端查看链接；
4. **安全提示**：明确说明行程保存在私有数据库中，未自动生成公开分享链接（如需分享可使用“生成只读分享”功能）。
