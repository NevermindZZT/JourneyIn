<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { MonitorFormData } from './MonitorEditModal.vue'

export interface EvaluationItem {
  id: string
  monitor_id: string
  window_start: string
  window_end: string
  score: number
  level: 'strongly_recommended' | 'acceptable' | 'waiting'
  details: {
    summary: string
    moon_phase_name: string
    moon_illumination: number
    galactic_hours: number
    holiday_name?: string
    leave_days_required: number
    weather_condition?: string
    cloud_cover_avg: number
    rain_prob_avg: number
    scores: {
      astro_score: number
      weather_score: number
      calendar_score: number
      budget_score: number
    }
  }
}

export interface MonitorItem {
  id: string
  title: string
  place_name: string
  lat: number
  lng: number
  crs: string
  criteria: {
    months?: number[]
    stargazing_mode?: boolean
    max_leave_days?: number
    max_cloud_cover?: number
    max_rain_prob?: number
    min_duration_days?: number
    max_duration_days?: number
  }
  origin_city?: string
  max_budget?: number
  enabled: boolean
  latest_evaluation?: EvaluationItem
}

const props = defineProps<{
  apiFetch: (path: string, options?: RequestInit) => Promise<Response>
}>()

const emit = defineEmits<{
  (e: 'newMonitor'): void
  (e: 'editMonitor', item: MonitorItem): void
  (e: 'openTrip', tripID: string): void
}>()

const monitors = ref<MonitorItem[]>([])
const loading = ref(false)
const filterTab = ref<'all' | 'recommended' | 'watching'>('all')
const expandedMonitorID = ref<string | null>(null)
const monitorTimeline = ref<Record<string, EvaluationItem[]>>({})
const convertingID = ref<string | null>(null)
const evaluatingID = ref<string | null>(null)
const toastMsg = ref('')

onMounted(() => {
  void loadMonitors()
})

async function loadMonitors() {
  loading.value = true
  try {
    const res = await props.apiFetch('/api/v1/monitors')
    if (res.ok) {
      monitors.value = (await res.json()) as MonitorItem[]
    }
  } catch (err) {
    console.error('load monitors error:', err)
  } finally {
    loading.value = false
  }
}

const filteredMonitors = computed(() => {
  if (filterTab.value === 'recommended') {
    return monitors.value.filter(m => m.latest_evaluation?.level === 'strongly_recommended')
  }
  if (filterTab.value === 'watching') {
    return monitors.value.filter(m => m.latest_evaluation?.level !== 'strongly_recommended')
  }
  return monitors.value
})

const recommendedCount = computed(() => {
  return monitors.value.filter(m => m.latest_evaluation?.level === 'strongly_recommended').length
})

async function toggleTimeline(m: MonitorItem) {
  if (expandedMonitorID.value === m.id) {
    expandedMonitorID.value = null
    return
  }
  expandedMonitorID.value = m.id
  if (!monitorTimeline.value[m.id]) {
    try {
      const res = await props.apiFetch(`/api/v1/monitors/${m.id}/evaluations`)
      if (res.ok) {
        monitorTimeline.value[m.id] = (await res.json()) as EvaluationItem[]
      }
    } catch (err) {
      console.error('load evaluations error:', err)
    }
  }
}

async function triggerEvaluate(m: MonitorItem) {
  evaluatingID.value = m.id
  try {
    const res = await props.apiFetch(`/api/v1/monitors/${m.id}/evaluate`, { method: 'POST' })
    if (res.ok) {
      const data = (await res.json()) as { evaluations?: EvaluationItem[] }
      if (data.evaluations?.length) {
        monitorTimeline.value[m.id] = data.evaluations
      }
      await loadMonitors()
      showToast('时机评估已更新！')
    }
  } catch (err) {
    console.error('evaluate error:', err)
  } finally {
    evaluatingID.value = null
  }
}

async function deleteMonitor(m: MonitorItem) {
  if (!window.confirm(`确认删除对【${m.place_name}】的监控规则吗？`)) return
  try {
    const res = await props.apiFetch(`/api/v1/monitors/${m.id}`, { method: 'DELETE' })
    if (res.ok) {
      await loadMonitors()
      showToast('监控已移除')
    }
  } catch (err) {
    console.error('delete monitor error:', err)
  }
}

async function convertToTrip(m: MonitorItem, winStart: string, winEnd: string) {
  convertingID.value = m.id
  try {
    const res = await props.apiFetch(`/api/v1/monitors/${m.id}/convert-trip`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ window_start: winStart, window_end: winEnd }),
    })
    if (!res.ok) {
      throw new Error('转换行程失败')
    }
    const data = (await res.json()) as { trip_id?: string }
    if (data.trip_id) {
      showToast('已为你自动生成行程草稿！正在直达行程规划…')
      emit('openTrip', data.trip_id)
    }
  } catch (err) {
    alert(err instanceof Error ? err.message : '创建行程失败')
  } finally {
    convertingID.value = null
  }
}

function showToast(text: string) {
  toastMsg.value = text
  setTimeout(() => {
    if (toastMsg.value === text) toastMsg.value = ''
  }, 3500)
}

function formatMonths(months?: number[]): string {
  if (!months?.length || months.length === 12) return '全年适游'
  return months.map(m => m + '月').join('、')
}

defineExpose({
  loadMonitors,
})
</script>

<template>
  <div class="travel-radar-view">
    <!-- 头部栏 -->
    <header class="radar-header">
      <div class="header-main">
        <span class="eyebrow">TRAVEL OPPORTUNITY RADAR</span>
        <h2>旅行时机雷达</h2>
        <p>多维监测目标景点的适游月份、天文月相、节假日与气象少云窗口，捕捉最佳游玩良机</p>
      </div>
      <button type="button" class="btn-create-monitor" @click="emit('newMonitor')">
        <span class="icon">+</span>
        <span>添加心愿景点监控</span>
      </button>
    </header>

    <!-- 顶部状态横幅 (如果有推荐) -->
    <div v-if="recommendedCount > 0" class="radar-highlight-banner">
      <div class="banner-badge">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 23c-4.97 0-9-4.03-9-9 0-3.64 2.2-6.77 5.38-8.11C8.75 8.13 9 10.45 9 11.5c0 1.25.54 2.39 1.41 3.17.65.59 1.48.97 2.39 1.07.72.08 1.45-.11 2.05-.51.87-.58 1.4-1.57 1.4-2.64 0-1.22-.39-2.35-1.07-3.29C17.2 11.08 19 13.56 19 16.5c0 3.59-2.91 6.5-6.5 6.5h-.5z"/></svg>
        <span>强烈推荐</span>
      </div>
      <div class="banner-text">
        当前雷达巡检已锁定 <strong>{{ recommendedCount }} 个绝佳出行窗口</strong>，包含新月纯黑夜、假期免请假与晴空少云时机，可直接一键生成行程！
      </div>
    </div>

    <!-- 筛选工具条 -->
    <div class="radar-filter-bar">
      <div class="filter-pills">
        <button
          type="button"
          class="filter-btn"
          :class="{ active: filterTab === 'all' }"
          @click="filterTab = 'all'"
        >
          全部监控 ({{ monitors.length }})
        </button>
        <button
          type="button"
          class="filter-btn"
          :class="{ active: filterTab === 'recommended' }"
          @click="filterTab = 'recommended'"
        >
          <span class="btn-icon-label">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 23c-4.97 0-9-4.03-9-9 0-3.64 2.2-6.77 5.38-8.11C8.75 8.13 9 10.45 9 11.5c0 1.25.54 2.39 1.41 3.17.65.59 1.48.97 2.39 1.07.72.08 1.45-.11 2.05-.51.87-.58 1.4-1.57 1.4-2.64 0-1.22-.39-2.35-1.07-3.29C17.2 11.08 19 13.56 19 16.5c0 3.59-2.91 6.5-6.5 6.5h-.5z"/></svg>
            <span>强烈推荐 ({{ recommendedCount }})</span>
          </span>
        </button>
        <button
          type="button"
          class="filter-btn"
          :class="{ active: filterTab === 'watching' }"
          @click="filterTab = 'watching'"
        >
          <span class="btn-icon-label">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
            <span>监控等待中</span>
          </span>
        </button>
      </div>
    </div>

    <!-- 监控卡片列表 -->
    <div v-if="loading" class="radar-empty">
      <span>正在扫描雷达时机…</span>
    </div>

    <div v-else-if="filteredMonitors.length === 0" class="radar-empty">
      <div class="empty-icon">
        <svg width="54" height="54" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2a10 10 0 1 0 10 10" />
          <path d="M12 6a6 6 0 1 0 6 6" />
          <circle cx="12" cy="12" r="2" />
          <line x1="12" y1="12" x2="19" y2="5" />
        </svg>
      </div>
      <h3>暂无符合条件的景点监控</h3>
      <p>你可以点击上方按钮添加你向往的旅行目的地，系统将为你每日自动巡检最佳游玩时间窗口。</p>
      <button type="button" class="btn-create-monitor empty-create-btn" @click="emit('newMonitor')">
        立即添加第一个心愿景点
      </button>
    </div>

    <div v-else class="monitor-cards-grid">
      <article
        v-for="m in filteredMonitors"
        :key="m.id"
        class="monitor-card"
        :class="{ 'is-recommended': m.latest_evaluation?.level === 'strongly_recommended' }"
      >
        <div class="card-header">
          <div class="card-title-group">
            <h3>{{ m.place_name }}</h3>
            <span v-if="m.title && m.title !== m.place_name" class="card-subtitle">{{ m.title }}</span>
          </div>
          <div class="card-status-badge" :class="[m.latest_evaluation?.level || 'waiting']">
            <span v-if="m.latest_evaluation?.level === 'strongly_recommended'" class="status-content">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 23c-4.97 0-9-4.03-9-9 0-3.64 2.2-6.77 5.38-8.11C8.75 8.13 9 10.45 9 11.5c0 1.25.54 2.39 1.41 3.17.65.59 1.48.97 2.39 1.07.72.08 1.45-.11 2.05-.51.87-.58 1.4-1.57 1.4-2.64 0-1.22-.39-2.35-1.07-3.29C17.2 11.08 19 13.56 19 16.5c0 3.59-2.91 6.5-6.5 6.5h-.5z"/></svg>
              <span>强烈推荐出行</span>
            </span>
            <span v-else-if="m.latest_evaluation?.level === 'acceptable'" class="status-content">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
              <span>备选时机</span>
            </span>
            <span v-else class="status-content">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              <span>监控等待中</span>
            </span>
          </div>
        </div>

        <!-- 规则标签 -->
        <div class="criteria-tags">
          <span class="criteria-tag">{{ formatMonths(m.criteria?.months) }}</span>
          <span v-if="m.criteria?.stargazing_mode" class="criteria-tag tag-stargazing">
            <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l1.9 5.8a2 2 0 0 0 1.3 1.3L21 12l-5.8 1.9a2 2 0 0 0-1.3 1.3L12 21l-1.9-5.8a2 2 0 0 0-1.3-1.3L3 12l5.8-1.9a2 2 0 0 0 1.3-1.3L12 3z"/></svg>
            <span>银河星空</span>
          </span>
          <span class="criteria-tag">{{ m.criteria?.max_leave_days === 0 ? '免请假' : `最多请${m.criteria?.max_leave_days}天` }}</span>
          <span v-if="m.origin_city" class="criteria-tag">{{ m.origin_city }}出发</span>
          <span v-if="m.max_budget" class="criteria-tag">预算≤{{ m.max_budget }}元</span>
        </div>

        <!-- 最佳窗口预测展示区 -->
        <div v-if="m.latest_evaluation" class="opportunity-window-box">
          <div class="window-top">
            <div class="window-date">
              <strong>{{ m.latest_evaluation.window_start }} ~ {{ m.latest_evaluation.window_end }}</strong>
              <small>{{ m.latest_evaluation.details?.summary }}</small>
            </div>
            <div class="window-score">
              <span class="score-val">{{ m.latest_evaluation.score }}</span>
              <span class="score-lbl">分</span>
            </div>
          </div>

          <!-- 要素卡片细分 -->
          <div class="factors-grid">
            <div class="factor-item">
              <span class="factor-lbl">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
                <span>月相条件</span>
              </span>
              <strong>{{ m.latest_evaluation.details?.moon_phase_name || '正常' }}</strong>
              <small>月面照亮 {{ Math.round((m.latest_evaluation.details?.moon_illumination || 0) * 100) }}%</small>
            </div>
            <div class="factor-item">
              <span class="factor-lbl">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/></svg>
                <span>银河核心</span>
              </span>
              <strong>{{ m.latest_evaluation.details?.galactic_hours > 0 ? `${m.latest_evaluation.details.galactic_hours}小时` : '不可见' }}</strong>
              <small>纯黑夜有效观测时段</small>
            </div>
            <div class="factor-item">
              <span class="factor-lbl">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <span>气象少云</span>
              </span>
              <strong>{{ m.latest_evaluation.details?.weather_condition || '少云' }}</strong>
              <small>云量 {{ m.latest_evaluation.details?.cloud_cover_avg || 20 }}%</small>
            </div>
            <div class="factor-item">
              <span class="factor-lbl">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>
                <span>假期匹配</span>
              </span>
              <strong>{{ m.latest_evaluation.details?.holiday_name || '周末' }}</strong>
              <small>{{ m.latest_evaluation.details?.leave_days_required === 0 ? '无需请假' : `需请假 ${m.latest_evaluation.details?.leave_days_required} 天` }}</small>
            </div>
          </div>
        </div>

        <div v-else class="no-eval-box">
          <span>暂无匹配的近期窗口，正在等待时机开启…</span>
        </div>

        <!-- 操作按钮栏 -->
        <div class="card-actions">
          <button
            v-if="m.latest_evaluation"
            type="button"
            class="btn-convert"
            :disabled="convertingID === m.id"
            @click="convertToTrip(m, m.latest_evaluation.window_start, m.latest_evaluation.window_end)"
          >
            <span v-if="convertingID === m.id">正在生成行程…</span>
            <span v-else class="btn-icon-label">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polygon points="3 11 22 2 13 21 11 13 3 11"/></svg>
              <span>一键生成此窗口行程</span>
            </span>
          </button>
          <button
            type="button"
            class="btn-sec"
            @click="toggleTimeline(m)"
          >
            {{ expandedMonitorID === m.id ? '收起时间轴' : '查看未来45天窗口' }}
          </button>
          <button
            type="button"
            class="btn-sec"
            :disabled="evaluatingID === m.id"
            title="重新执行天气与月相评估"
            @click="triggerEvaluate(m)"
          >
            <span v-if="evaluatingID === m.id">…</span>
            <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </button>
          <button
            type="button"
            class="btn-sec"
            title="编辑监控"
            @click="emit('editMonitor', m)"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>
          </button>
          <button
            type="button"
            class="btn-sec btn-del"
            title="删除监控"
            @click="deleteMonitor(m)"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>

        <!-- 展开的未来 45 天打分时间轴 -->
        <div v-if="expandedMonitorID === m.id" class="timeline-expand-box">
          <label class="timeline-title">未来 45 天旅行窗口机会时间轴</label>
          <div v-if="!monitorTimeline[m.id]?.length" class="timeline-empty">
            <span>正在分析后续候选窗口…</span>
          </div>
          <div v-else class="timeline-list">
            <div
              v-for="ev in monitorTimeline[m.id]"
              :key="ev.id"
              class="timeline-row"
              :class="[ev.level]"
            >
              <div class="timeline-dates">
                <strong>{{ ev.window_start }} ~ {{ ev.window_end }}</strong>
                <small>{{ ev.details?.summary }}</small>
              </div>
              <div class="timeline-score-badge">
                {{ ev.score }} 分
              </div>
              <button
                type="button"
                class="timeline-convert-btn"
                @click="convertToTrip(m, ev.window_start, ev.window_end)"
              >
                选此窗口成行
              </button>
            </div>
          </div>
        </div>
      </article>
    </div>

    <!-- 底部反馈 -->
    <div v-if="toastMsg" class="radar-toast">{{ toastMsg }}</div>
  </div>
</template>

<style scoped>
.travel-radar-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  max-width: 1140px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}

.radar-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.header-main .eyebrow {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  color: var(--p);
}

.header-main h2 {
  margin: 4px 0 6px;
  font-size: 26px;
  font-weight: 800;
  color: var(--ink);
}

.header-main p {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
}

.btn-create-monitor {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 0 18px;
  border-radius: 12px;
  border: 0;
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.2s ease;
  white-space: nowrap;
}

.btn-create-monitor:hover {
  background: color-mix(in srgb, var(--p) 86%, #000);
}

.radar-highlight-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 18px;
  background: color-mix(in srgb, var(--p) 12%, var(--surface));
  border: 1px solid var(--p);
  border-radius: 14px;
}

.banner-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  font-size: 11px;
  font-weight: 800;
  white-space: nowrap;
  line-height: 1;
}

.banner-badge svg {
  flex-shrink: 0;
  display: block;
}

.btn-icon-label {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  line-height: 1;
}

.btn-icon-label svg {
  flex-shrink: 0;
  display: block;
}

.banner-text {
  font-size: 13px;
  color: var(--ink);
  line-height: 1.45;
}

.radar-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--line);
  padding-bottom: 12px;
}

.filter-pills {
  display: flex;
  gap: 6px;
}

.filter-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 32px;
  padding: 6px 14px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface);
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  line-height: 1;
}

.filter-btn:hover {
  background: var(--panel);
}

.filter-btn.active {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  border-color: var(--p);
}

.radar-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  color: var(--muted);
  gap: 12px;
}

.empty-icon {
  font-size: 48px;
}

.empty-create-btn {
  margin-top: 12px;
}

.monitor-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(480px, 1fr));
  gap: 20px;
}

.monitor-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
  border-radius: 18px;
  background: var(--surface);
  border: 1px solid var(--line);
  box-shadow: 0 4px 18px rgba(0, 0, 0, 0.05);
  transition: all 0.2s ease;
}

.monitor-card.is-recommended {
  border-color: color-mix(in srgb, #ff6b35 70%, var(--line));
  box-shadow: 0 6px 24px rgba(255, 107, 53, 0.12);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-title-group h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 800;
  color: var(--ink);
}

.card-subtitle {
  font-size: 12px;
  color: var(--muted);
}

.card-status-badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 8px;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
  line-height: 1;
}

.status-content {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  line-height: 1;
}

.status-content svg {
  flex-shrink: 0;
  display: block;
}

.card-status-badge.strongly_recommended {
  background: color-mix(in srgb, #ff6b35 18%, var(--surface));
  color: #ff6b35;
  border: 1px solid #ff6b35;
}

.card-status-badge.acceptable {
  background: color-mix(in srgb, #0284c7 15%, var(--surface));
  color: #0284c7;
  border: 1px solid #0284c7;
}

.card-status-badge.waiting {
  background: var(--panel);
  color: var(--muted);
  border: 1px solid var(--line);
}

.criteria-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.criteria-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--panel);
  color: var(--muted);
  font-size: 11px;
  font-weight: 600;
  line-height: 1.2;
}

.criteria-tag svg {
  flex-shrink: 0;
  display: block;
}

.tag-stargazing {
  background: color-mix(in srgb, #6366f1 15%, var(--surface));
  color: #818cf8;
}

.opportunity-window-box {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  background: var(--panel);
  border-radius: 14px;
  border: 1px solid var(--line);
}

.window-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.window-date strong {
  font-size: 15px;
  color: var(--ink);
}

.window-date small {
  display: block;
  color: var(--p);
  font-size: 12px;
  font-weight: 600;
  margin-top: 2px;
}

.window-score {
  display: flex;
  align-items: baseline;
  color: #ff6b35;
}

.score-val {
  font-size: 26px;
  font-weight: 900;
  font-variant-numeric: tabular-nums;
}

.score-lbl {
  font-size: 12px;
  font-weight: 700;
  margin-left: 2px;
}

.factors-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}

.factor-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  background: var(--surface);
  border-radius: 10px;
  border: 1px solid var(--line);
}

.factor-lbl {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 10px;
  color: var(--muted);
  font-weight: 700;
  line-height: 1;
}

.factor-lbl svg {
  flex-shrink: 0;
  display: block;
}

.factor-item strong {
  font-size: 12px;
  color: var(--ink);
}

.factor-item small {
  font-size: 10px;
  color: var(--muted);
}

.no-eval-box {
  padding: 24px;
  text-align: center;
  background: var(--panel);
  border-radius: 14px;
  color: var(--muted);
  font-size: 12px;
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: auto;
}

.btn-convert {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  flex: 1;
  min-height: 38px;
  padding: 0 16px;
  border-radius: 10px;
  border: 0;
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.btn-convert svg {
  flex-shrink: 0;
  display: block;
}

.btn-convert:hover:not(:disabled) {
  background: color-mix(in srgb, var(--p) 86%, #000);
}

.btn-sec {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  min-height: 38px;
  padding: 0 12px;
  border-radius: 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}

.btn-sec svg {
  flex-shrink: 0;
  display: block;
}

.btn-sec:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--panel));
}

.btn-del {
  color: #ef4444;
  font-size: 15px;
  padding: 0 10px;
}

.timeline-expand-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--line);
}

.timeline-title {
  font-size: 11px;
  font-weight: 700;
  color: var(--muted);
}

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 240px;
  overflow-y: auto;
}

.timeline-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--panel);
  font-size: 12px;
}

.timeline-dates {
  display: flex;
  flex-direction: column;
  flex: 1;
}

.timeline-dates strong {
  color: var(--ink);
  font-size: 12px;
}

.timeline-dates small {
  color: var(--muted);
  font-size: 10px;
}

.timeline-score-badge {
  font-size: 12px;
  font-weight: 800;
  color: var(--p);
}

.timeline-convert-btn {
  padding: 4px 8px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface);
  color: var(--p);
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.radar-toast {
  position: fixed;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  padding: 10px 20px;
  background: var(--ink);
  color: var(--surface);
  border-radius: 999px;
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 8px 24px rgba(0,0,0,0.3);
  z-index: 9999;
}

@media (max-width: 600px) {
  .monitor-cards-grid {
    grid-template-columns: 1fr;
  }
  .radar-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
