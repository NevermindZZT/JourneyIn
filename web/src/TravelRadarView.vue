<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

export interface EvaluationItem {
  id: string; monitor_id: string; window_start: string; window_end: string; score: number
  level: 'strongly_recommended' | 'acceptable' | 'waiting'
  details: { summary?: string; moon_phase_name?: string; moon_illumination?: number; galactic_hours?: number; holiday_name?: string; leave_days_required?: number; weather_condition?: string; cloud_cover_avg?: number; rain_prob_avg?: number; scores?: { astro_score?: number; weather_score?: number; calendar_score?: number; budget_score?: number } }
}
export interface MonitorItem {
  id: string; title: string; place_name: string; lat: number; lng: number; crs: string
  criteria: { months?: number[]; stargazing_mode?: boolean; max_leave_days?: number; max_cloud_cover?: number; max_rain_prob?: number; min_duration_days?: number; max_duration_days?: number }
  origin_city?: string; max_budget?: number; enabled: boolean; latest_evaluation?: EvaluationItem
}
const props = defineProps<{ apiFetch: (path: string, options?: RequestInit) => Promise<Response> }>()
const emit = defineEmits<{
  (e: 'newMonitor'): void
  (e: 'editMonitor', item: MonitorItem): void
  (e: 'openTrip', tripID: string): void
  (e: 'monitorsUpdated', items: MonitorItem[]): void
  (e: 'visibleMonitorsChanged', monitorIDs: string[]): void
  (e: 'monitorSelected', monitorID: string | null): void
  (e: 'windowSelected', value: { monitorID: string; evaluation: EvaluationItem | null }): void
  (e: 'sheetDragStart', event: PointerEvent): void
  (e: 'sheetHeaderClick', event: MouseEvent): void
}>()

const monitors = ref<MonitorItem[]>([])
const loading = ref(false)
const loadError = ref('')
const filterTab = ref<'all' | 'recommended' | 'watching'>('all')
const selectedMonitorID = ref<string | null>(null)
const evaluationsByMonitor = ref<Record<string, EvaluationItem[]>>({})
const selectedEvaluationID = ref<string | null>(null)
const convertingID = ref<string | null>(null)
const evaluatingID = ref<string | null>(null)
const deletingID = ref<string | null>(null)
const toastMsg = ref('')

const selectedMonitor = computed(() => monitors.value.find(item => item.id === selectedMonitorID.value) || null)
const selectedEvaluations = computed(() => [...(evaluationsByMonitor.value[selectedMonitorID.value || ''] || [])].sort((a, b) => a.window_start.localeCompare(b.window_start)))
const selectedEvaluation = computed(() => {
  const list = evaluationsByMonitor.value[selectedMonitorID.value || ''] || []
  return list.find(item => item.id === selectedEvaluationID.value) || selectedMonitor.value?.latest_evaluation || list.slice().sort((a, b) => b.score - a.score)[0] || null
})
const recommendedCount = computed(() => monitors.value.filter(item => item.latest_evaluation?.level === 'strongly_recommended').length)
const watchingCount = computed(() => monitors.value.length - recommendedCount.value)
const filteredMonitors = computed(() => {
  let items = monitors.value.slice()
  if (filterTab.value === 'recommended') items = items.filter(item => item.latest_evaluation?.level === 'strongly_recommended')
  if (filterTab.value === 'watching') items = items.filter(item => item.latest_evaluation?.level !== 'strongly_recommended')
  return items.slice().sort((a, b) => {
    const scoreA = a.latest_evaluation?.score ?? -1
    const scoreB = b.latest_evaluation?.score ?? -1
    if (scoreA !== scoreB) return scoreB - scoreA
    return (a.latest_evaluation?.window_start || '9999').localeCompare(b.latest_evaluation?.window_start || '9999')
  })
})

watch(filteredMonitors, items => emit('visibleMonitorsChanged', items.map(item => item.id)), { immediate: true })
onMounted(() => { void loadMonitors() })
async function loadMonitors() {
  loading.value = true; loadError.value = ''
  try {
    const res = await props.apiFetch('/api/v1/monitors')
    if (!res.ok) throw new Error('无法读取景点监控列表')
    const payload = await res.json() as MonitorItem[]
    monitors.value = Array.isArray(payload) ? payload : []
    emit('monitorsUpdated', monitors.value)
    if (selectedMonitorID.value && !monitors.value.some(item => item.id === selectedMonitorID.value)) await returnToList()
  } catch (err) { loadError.value = err instanceof Error ? err.message : '读取监控列表失败' }
  finally { loading.value = false }
}
async function loadEvaluations(monitorID: string, force = false) {
  if (!force && evaluationsByMonitor.value[monitorID]) return evaluationsByMonitor.value[monitorID]
  try {
    const res = await props.apiFetch('/api/v1/monitors/' + encodeURIComponent(monitorID) + '/evaluations?limit=100')
    if (!res.ok) throw new Error('无法读取未来机会窗口')
    const payload = await res.json() as EvaluationItem[]
    const items = Array.isArray(payload) ? payload : []
    evaluationsByMonitor.value = { ...evaluationsByMonitor.value, [monitorID]: items }
    return items
  } catch (err) { loadError.value = err instanceof Error ? err.message : '读取机会窗口失败'; return [] }
}
async function refreshMonitor(targetID?: string) {
  const idToRefresh = targetID || selectedMonitorID.value
  if (idToRefresh) {
    const next = { ...evaluationsByMonitor.value }
    delete next[idToRefresh]
    evaluationsByMonitor.value = next
  }
  await loadMonitors()
  if (idToRefresh && selectedMonitorID.value === idToRefresh) {
    const freshEvals = await loadEvaluations(idToRefresh, true)
    const currentMon = monitors.value.find(item => item.id === idToRefresh)
    selectedEvaluationID.value = currentMon?.latest_evaluation?.id || (freshEvals[0]?.id ?? null)
    emit('windowSelected', { monitorID: idToRefresh, evaluation: selectedEvaluation.value })
  }
}
async function openMonitorDetails(monitorID: string) {
  const item = monitors.value.find(candidate => candidate.id === monitorID)
  if (!item) return
  selectedMonitorID.value = monitorID
  selectedEvaluationID.value = item.latest_evaluation?.id || null
  loadError.value = ''
  emit('monitorSelected', monitorID)
  const items = await loadEvaluations(monitorID)
  if (selectedMonitorID.value !== monitorID) return
  if (!selectedEvaluationID.value && items.length) selectedEvaluationID.value = items.slice().sort((a, b) => b.score - a.score)[0]?.id || null
  emit('windowSelected', { monitorID, evaluation: selectedEvaluation.value })
}
function selectMonitorFromMap(monitorID: string) { void openMonitorDetails(monitorID) }
async function returnToList() {
  selectedMonitorID.value = null; selectedEvaluationID.value = null; loadError.value = ''
  emit('monitorSelected', null); emit('windowSelected', { monitorID: '', evaluation: null })
}
function selectEvaluation(evaluation: EvaluationItem) {
  selectedEvaluationID.value = evaluation.id
  if (selectedMonitorID.value) emit('windowSelected', { monitorID: selectedMonitorID.value, evaluation })
}
function formatMonths(months?: number[]) {
  if (!months?.length || months.length === 12) return '全年适游'
  return months.slice().sort((a,b)=>a-b).map(month => month + '月').join('、')
}
function formatLeaveDays(days?: number) { return days == null || days <= 0 ? '免请假' : '最多请' + days + '天' }
function levelLabel(level?: EvaluationItem['level']) {
  if (level === 'strongly_recommended') return '强烈推荐'
  if (level === 'acceptable') return '备选时机'
  if (level === 'waiting') return '待观察'
  return '等待评估'
}
function levelClass(level?: EvaluationItem['level']) { return level === 'strongly_recommended' ? 'strongly_recommended' : level === 'acceptable' ? 'acceptable' : 'waiting' }
function showToast(message: string) { toastMsg.value = message; window.setTimeout(() => { if (toastMsg.value === message) toastMsg.value = '' }, 3200) }
async function triggerEvaluate(item: MonitorItem) {
  evaluatingID.value = item.id; loadError.value = ''
  try {
    const res = await props.apiFetch('/api/v1/monitors/' + encodeURIComponent(item.id) + '/evaluate', { method: 'POST' })
    if (!res.ok) throw new Error('重新评估失败')
    const data = await res.json() as { evaluations?: EvaluationItem[] }
    if (data.evaluations) evaluationsByMonitor.value = { ...evaluationsByMonitor.value, [item.id]: data.evaluations }
    await refreshMonitor(item.id); showToast('时机评估已更新')
  } catch (err) { loadError.value = err instanceof Error ? err.message : '重新评估失败' }
  finally { evaluatingID.value = null }
}
async function deleteMonitor(item: MonitorItem) {
  if (!window.confirm('确认删除对「' + item.place_name + '」的景点监控吗？')) return
  deletingID.value = item.id
  try {
    const res = await props.apiFetch('/api/v1/monitors/' + encodeURIComponent(item.id), { method: 'DELETE' })
    if (!res.ok) throw new Error('删除监控失败')
    if (selectedMonitorID.value === item.id) await returnToList()
    const next = { ...evaluationsByMonitor.value }; delete next[item.id]; evaluationsByMonitor.value = next
    await loadMonitors(); showToast('监控已删除')
  } catch (err) { loadError.value = err instanceof Error ? err.message : '删除监控失败' }
  finally { deletingID.value = null }
}
async function convertToTrip(item: MonitorItem, evaluation: EvaluationItem | null) {
  if (!evaluation) return
  convertingID.value = item.id; loadError.value = ''
  try {
    const res = await props.apiFetch('/api/v1/monitors/' + encodeURIComponent(item.id) + '/convert-trip', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ window_start: evaluation.window_start, window_end: evaluation.window_end }),
    })
    if (!res.ok) { const payload = await res.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '转换行程失败') }
    const data = await res.json() as { trip_id?: string }
    if (data.trip_id) { showToast('已生成行程草稿，正在打开…'); emit('openTrip', data.trip_id) }
  } catch (err) { loadError.value = err instanceof Error ? err.message : '创建行程草稿失败' }
  finally { convertingID.value = null }
}
function onHeaderPointerDown(event: PointerEvent) {
  if ((event.target as HTMLElement | null)?.closest('button, a, input, select, textarea, [role="button"]')) return
  emit('sheetDragStart', event)
}

function onHeaderClick(event: MouseEvent) {
  if ((event.target as HTMLElement | null)?.closest('button, a, input, select, textarea, [role="button"]')) return
  emit('sheetHeaderClick', event)
}

defineExpose({ loadMonitors, refreshMonitor, selectMonitorFromMap })
</script>

<template>
  <div class="travel-radar-view is-panel">
    <header class="radar-panel-header" @pointerdown="onHeaderPointerDown" @click="onHeaderClick">
      <div class="radar-panel-heading">
        <button v-if="selectedMonitor" type="button" class="radar-detail-back" aria-label="返回监控列表" @click="returnToList">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/><path d="M9 12h12"/></svg>
        </button>
        <div class="radar-panel-heading-copy">
          <span class="eyebrow">TRAVEL OPPORTUNITY RADAR</span>
          <h2>{{ selectedMonitor?.place_name || '旅行时机雷达' }}</h2>
          <p v-if="selectedMonitor">{{ selectedMonitor.title }}</p>
          <p v-else>监控 {{ monitors.length }} 个心愿景点 · {{ recommendedCount }} 个强烈推荐</p>
        </div>
      </div>
      <button v-if="!selectedMonitor" type="button" class="radar-add-button" aria-label="添加心愿景点监控" @click="emit('newMonitor')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg><span>添加</span>
      </button>
    </header>

    <template v-if="!selectedMonitor">
      <div class="radar-list-summary"><span class="radar-list-count">{{ filteredMonitors.length }} 个景点</span><span class="radar-list-hint">按推荐度与窗口日期排序</span></div>
      <div class="radar-filter-pills" role="group" aria-label="监控点筛选">
        <button type="button" :class="{ active: filterTab === 'all' }" @click="filterTab = 'all'">全部 {{ monitors.length }}</button>
        <button type="button" :class="{ active: filterTab === 'recommended' }" @click="filterTab = 'recommended'">强烈推荐 {{ recommendedCount }}</button>
        <button type="button" :class="{ active: filterTab === 'watching' }" @click="filterTab = 'watching'">待观察 {{ watchingCount }}</button>
      </div>
      <div class="radar-list-scroll">
        <div v-if="loading" class="radar-state-message" role="status">正在读取景点监控…</div>
        <div v-else-if="loadError" class="radar-state-message radar-state-error" role="alert"><span>{{ loadError }}</span><button type="button" @click="loadMonitors">重试</button></div>
        <div v-else-if="filteredMonitors.length === 0" class="radar-state-message radar-empty-panel">
          <svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3"/><path d="M12 3v2M21 12h-2M12 21v-2M3 12h2"/></svg>
          <strong>{{ monitors.length ? '当前筛选下没有监控点' : '还没有心愿景点' }}</strong>
          <span>{{ monitors.length ? '切换筛选条件查看其他景点。' : '添加景点，雷达会持续寻找合适的出行窗口。' }}</span>
          <button v-if="!monitors.length" type="button" class="radar-add-button" @click="emit('newMonitor')">添加景点</button>
        </div>
        <div v-else class="radar-monitor-list">
          <button v-for="item in filteredMonitors" :key="item.id" type="button" class="radar-monitor-row" :class="levelClass(item.latest_evaluation?.level)" @click="openMonitorDetails(item.id)">
            <span class="radar-row-indicator" :class="levelClass(item.latest_evaluation?.level)" aria-hidden="true"></span>
            <span class="radar-row-main">
              <span class="radar-row-title"><strong>{{ item.place_name }}</strong><span class="radar-row-level" :class="levelClass(item.latest_evaluation?.level)">{{ levelLabel(item.latest_evaluation?.level) }}</span></span>
              <span class="radar-row-window">{{ item.latest_evaluation ? item.latest_evaluation.window_start + ' ~ ' + item.latest_evaluation.window_end : '暂无匹配的近期窗口' }}</span>
              <span class="radar-row-tags"><span>{{ formatMonths(item.criteria?.months) }}</span><span v-if="item.criteria?.stargazing_mode">星空监控</span><span>{{ formatLeaveDays(item.criteria?.max_leave_days) }}</span></span>
            </span>
            <span v-if="item.latest_evaluation" class="radar-row-score" :class="levelClass(item.latest_evaluation.level)">{{ Math.round(item.latest_evaluation.score) }}<small>分</small></span>
            <span v-else class="radar-row-score radar-row-score-empty" aria-label="尚无评分">—</span>
            <svg class="radar-row-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg>
          </button>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="radar-detail-scroll">
        <div class="radar-detail-score-card" :class="levelClass(selectedEvaluation?.level)">
          <div class="radar-detail-score-copy"><span class="radar-detail-level">{{ levelLabel(selectedEvaluation?.level) }}</span><strong>{{ selectedEvaluation ? selectedEvaluation.window_start + ' ~ ' + selectedEvaluation.window_end : '暂无可用窗口' }}</strong><small>{{ selectedEvaluation?.details?.summary || '雷达尚未计算出可用的近期出行窗口。' }}</small></div>
          <div v-if="selectedEvaluation" class="radar-detail-score-value"><strong>{{ Math.round(selectedEvaluation.score) }}</strong><small>分</small></div>
        </div>
        <div class="radar-detail-tags"><span>{{ formatMonths(selectedMonitor.criteria?.months) }}</span><span v-if="selectedMonitor.criteria?.stargazing_mode">星空监控</span><span>{{ formatLeaveDays(selectedMonitor.criteria?.max_leave_days) }}</span><span v-if="selectedMonitor.origin_city">{{ selectedMonitor.origin_city }}出发</span><span v-if="selectedMonitor.max_budget">预算 ≤ {{ selectedMonitor.max_budget }} 元</span></div>
        <section v-if="selectedEvaluation" class="radar-detail-section" aria-label="推荐因素">
          <h3>推荐因素</h3>
          <div class="radar-factor-list">
            <div class="radar-factor-row"><span>月相</span><strong>{{ selectedEvaluation.details.moon_phase_name || '未启用' }}</strong><small>{{ selectedEvaluation.details.moon_illumination != null ? '月面照亮 ' + Math.round(selectedEvaluation.details.moon_illumination * 100) + '%' : '暂无月相数据' }} · 天文 {{ selectedEvaluation.details.scores?.astro_score ?? '—' }}分</small></div>
            <div class="radar-factor-row"><span>银河核心</span><strong>{{ selectedEvaluation.details.galactic_hours ? selectedEvaluation.details.galactic_hours + ' 小时' : '不可见' }}</strong><small>纯黑夜有效时段 · 天文 {{ selectedEvaluation.details.scores?.astro_score ?? '—' }}分</small></div>
            <div class="radar-factor-row"><span>天气</span><strong>{{ selectedEvaluation.details.weather_condition || '暂无预报' }}</strong><small>{{ selectedEvaluation.details.cloud_cover_avg != null ? '云量 ' + selectedEvaluation.details.cloud_cover_avg + '%' : '云量暂无' }} · 气象 {{ selectedEvaluation.details.scores?.weather_score ?? '—' }}分</small></div>
            <div class="radar-factor-row"><span>假期</span><strong>{{ selectedEvaluation.details.holiday_name || '周末/工作日窗口' }}</strong><small>{{ selectedEvaluation.details.leave_days_required === 0 ? '无需请假' : '需请假 ' + (selectedEvaluation.details.leave_days_required ?? 0) + ' 天' }} · 日历 {{ selectedEvaluation.details.scores?.calendar_score ?? '—' }}分</small></div>
          </div>
        </section>
        <section class="radar-detail-section" aria-label="未来出行窗口">
          <div class="radar-detail-section-heading"><h3>未来出行窗口</h3><span>45 天内</span></div>
          <div v-if="!selectedEvaluations.length" class="radar-state-message">{{ loadError || '暂无候选窗口；可以重新评估或继续等待条件变化。' }}</div>
          <div v-else class="radar-window-list">
            <div v-for="evaluation in selectedEvaluations" :key="evaluation.id" class="radar-window-row" :class="[levelClass(evaluation.level), { selected: selectedEvaluation?.id === evaluation.id }]">
              <button type="button" class="radar-window-select" @click="selectEvaluation(evaluation)"><strong>{{ evaluation.window_start }} ~ {{ evaluation.window_end }}</strong><small>{{ evaluation.details.summary || levelLabel(evaluation.level) }}</small></button>
              <span class="radar-window-score">{{ Math.round(evaluation.score) }} 分</span>
              <button type="button" class="radar-window-convert" :disabled="convertingID === selectedMonitor.id" @click="convertToTrip(selectedMonitor, evaluation)">生成行程</button>
            </div>
          </div>
        </section>
        <p v-if="loadError" class="radar-inline-error" role="alert">{{ loadError }}</p>
      </div>
      <footer class="radar-detail-actions">
        <button type="button" class="radar-create-trip" :disabled="!selectedEvaluation || convertingID === selectedMonitor.id" @click="convertToTrip(selectedMonitor, selectedEvaluation)">{{ convertingID === selectedMonitor.id ? '正在生成…' : '生成所选窗口行程草稿' }}</button>
        <button type="button" class="radar-icon-action" :disabled="evaluatingID === selectedMonitor.id" title="重新评估" aria-label="重新评估" @click="triggerEvaluate(selectedMonitor)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.5 9a9 9 0 0 1 14.9-3.4L23 10M1 14l4.6 4.4A9 9 0 0 0 20.5 15"/></svg></button>
        <button type="button" class="radar-icon-action" title="编辑监控规则" aria-label="编辑监控规则" @click="emit('editMonitor', selectedMonitor)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/></svg></button>
        <button type="button" class="radar-icon-action danger" :disabled="deletingID === selectedMonitor.id" title="删除监控" aria-label="删除监控" @click="deleteMonitor(selectedMonitor)"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4h8v2m3 0-1 14H6L5 6m4 4v6m6-6v6"/></svg></button>
      </footer>
    </template>
    <div v-if="toastMsg" class="radar-toast">{{ toastMsg }}</div>
  </div>
</template>

<style scoped>
.travel-radar-view.is-panel { display:flex; min-width:0; min-height:0; width:100%; height:auto; flex:1 1 auto; flex-direction:column; gap:0; padding:0; margin:0; overflow:hidden; box-sizing:border-box; }
.radar-panel-header { display:flex; min-width:0; align-items:center; justify-content:space-between; gap:10px; padding:15px 18px 13px; border-bottom:1px solid var(--line); flex:0 0 auto; }
.radar-panel-heading,.radar-panel-heading-copy { display:flex; min-width:0; }
.radar-panel-heading { align-items:center; gap:10px; }
.radar-panel-heading-copy { flex-direction:column; gap:2px; }
.radar-panel-heading-copy .eyebrow { margin:0; color:var(--p); font-size:9px; font-weight:900; letter-spacing:.1em; }
.radar-panel-heading h2 { overflow:hidden; margin:0; color:var(--ink); font-size:18px; font-weight:850; text-overflow:ellipsis; white-space:nowrap; }
.radar-panel-heading-copy p { overflow:hidden; margin:0; color:var(--muted); font-size:10px; text-overflow:ellipsis; white-space:nowrap; }
.radar-detail-back,.radar-add-button { display:inline-flex; flex:0 0 auto; align-items:center; justify-content:center; gap:6px; min-height:38px; padding:0 11px; border:1px solid var(--line); border-radius:11px; color:var(--ink); background:var(--surface); font-size:11px; font-weight:800; cursor:pointer; }
.radar-detail-back { width:38px; padding:0; }
.radar-add-button { color:var(--md-on-primary); border-color:var(--p); background:var(--p); }
.radar-add-button svg,.radar-detail-back svg { display:block; flex-shrink:0; }
.radar-list-summary { display:flex; align-items:center; justify-content:space-between; gap:8px; padding:13px 18px 8px; color:var(--muted); font-size:10px; }
.radar-list-count { color:var(--ink); font-size:11px; font-weight:800; }
.radar-list-hint { text-align:right; }
.radar-filter-pills { display:flex; gap:6px; padding:4px 18px 12px; overflow-x:auto; scrollbar-width:none; }
.radar-filter-pills::-webkit-scrollbar { display:none; }
.radar-filter-pills button { display:inline-flex; min-height:32px; flex:0 0 auto; align-items:center; justify-content:center; padding:0 10px; border:1px solid var(--line); border-radius:999px; color:var(--muted); background:var(--surface); font-size:10px; font-weight:800; white-space:nowrap; cursor:pointer; }
.radar-filter-pills button.active { border-color:var(--p); color:var(--md-on-primary); background:var(--p); }
.radar-list-scroll,.radar-detail-scroll { min-width:0; min-height:0; flex:1 1 auto; overflow-y:auto; overscroll-behavior-y:contain; padding:0 14px 16px; scrollbar-color:var(--line) transparent; scrollbar-width:thin; }
.radar-monitor-list { display:grid; gap:7px; padding-top:2px; }
.radar-monitor-row { display:flex; min-width:0; align-items:center; gap:9px; padding:10px; border:1px solid var(--line); border-radius:13px; color:var(--ink); background:var(--surface); text-align:left; cursor:pointer; transition:border-color .15s ease,background .15s ease,transform .15s ease; }
.radar-monitor-row:hover,.radar-monitor-row:focus-visible { border-color:color-mix(in srgb,var(--p) 60%,var(--line)); background:color-mix(in srgb,var(--p) 5%,var(--surface)); }
.radar-monitor-row:active { transform:scale(.99); }
.radar-row-indicator { width:4px; align-self:stretch; flex:0 0 4px; border-radius:99px; background:#7b8790; }
.radar-row-indicator.strongly_recommended { background:#ff6b35; }
.radar-row-indicator.acceptable { background:#2980b9; }
.radar-row-main { display:grid; min-width:0; flex:1 1 auto; gap:4px; }
.radar-row-title { display:flex; min-width:0; align-items:center; justify-content:space-between; gap:6px; }
.radar-row-title strong { overflow:hidden; min-width:0; font-size:13px; font-weight:850; text-overflow:ellipsis; white-space:nowrap; }
.radar-row-level { flex:0 0 auto; color:var(--muted); font-size:9px; font-weight:800; white-space:nowrap; }
.radar-row-level.strongly_recommended { color:#ff6b35; }
.radar-row-level.acceptable { color:#2980b9; }
.radar-row-window { overflow:hidden; color:var(--p); font-size:10px; font-weight:750; text-overflow:ellipsis; white-space:nowrap; }
.radar-row-tags { display:flex; min-width:0; flex-wrap:wrap; gap:4px; }
.radar-row-tags span { padding:2px 5px; border-radius:5px; color:var(--muted); background:var(--panel); font-size:9px; font-weight:700; }
.radar-row-score { display:inline-flex; align-items:baseline; flex:0 0 auto; color:#ff6b35; font-size:19px; font-weight:900; font-variant-numeric:tabular-nums; }
.radar-row-score.acceptable { color:#2980b9; }
.radar-row-score.waiting,.radar-row-score-empty { color:var(--muted); font-size:14px; }
.radar-row-score small { margin-left:2px; font-size:9px; }
.radar-row-chevron { flex:0 0 auto; color:var(--muted); }
.radar-state-message { display:flex; min-width:0; flex-direction:column; align-items:center; gap:9px; padding:24px 14px; color:var(--muted); font-size:11px; line-height:1.45; text-align:center; }
.radar-state-error { color:#d34e3b; }
.radar-state-message button { padding:6px 12px; border:1px solid var(--line); border-radius:8px; color:var(--p); background:var(--surface); font-weight:800; cursor:pointer; }
.radar-empty-panel svg { color:var(--p); }
.radar-empty-panel strong { color:var(--ink); font-size:13px; }
.radar-detail-scroll { padding-top:13px; }
.radar-detail-score-card { display:flex; min-width:0; align-items:center; justify-content:space-between; gap:10px; padding:13px; border:1px solid var(--line); border-radius:14px; background:var(--panel); }
.radar-detail-score-card.strongly_recommended { border-color:color-mix(in srgb,#ff6b35 65%,var(--line)); background:color-mix(in srgb,#ff6b35 8%,var(--panel)); }
.radar-detail-score-card.acceptable { border-color:color-mix(in srgb,#2980b9 55%,var(--line)); background:color-mix(in srgb,#2980b9 7%,var(--panel)); }
.radar-detail-score-copy { display:grid; min-width:0; gap:4px; }
.radar-detail-level { color:var(--p); font-size:10px; font-weight:850; }
.radar-detail-score-card.strongly_recommended .radar-detail-level { color:#ff6b35; }
.radar-detail-score-copy > strong { overflow-wrap:anywhere; color:var(--ink); font-size:14px; font-weight:850; }
.radar-detail-score-copy > small { color:var(--muted); font-size:10px; line-height:1.35; }
.radar-detail-score-value { display:flex; flex:0 0 auto; align-items:baseline; color:#ff6b35; }
.radar-detail-score-value strong { font-size:26px; font-weight:950; font-variant-numeric:tabular-nums; }
.radar-detail-score-value small { font-size:10px; font-weight:800; }
.radar-detail-tags { display:flex; flex-wrap:wrap; gap:5px; margin:9px 0 14px; }
.radar-detail-tags span { padding:3px 7px; border-radius:6px; color:var(--muted); background:var(--panel); font-size:9px; font-weight:750; }
.radar-detail-section { margin-top:14px; }
.radar-detail-section h3 { margin:0; color:var(--ink); font-size:12px; font-weight:850; }
.radar-detail-section-heading { display:flex; align-items:center; justify-content:space-between; margin-bottom:8px; }
.radar-detail-section-heading span { color:var(--muted); font-size:9px; font-weight:750; }
.radar-factor-list { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:6px; margin-top:8px; }
.radar-factor-row { display:grid; min-width:0; gap:3px; padding:8px; border:1px solid var(--line); border-radius:10px; background:color-mix(in srgb,var(--surface) 78%,transparent); }
.radar-factor-row > span { color:var(--muted); font-size:9px; font-weight:800; }
.radar-factor-row > strong { overflow-wrap:anywhere; color:var(--ink); font-size:10px; font-weight:800; }
.radar-factor-row > small { overflow-wrap:anywhere; color:var(--muted); font-size:9px; }
.radar-window-list { display:grid; gap:6px; margin-top:8px; }
.radar-window-row { display:grid; min-width:0; grid-template-columns:minmax(0,1fr) auto auto; align-items:center; gap:6px; padding:8px; border:1px solid var(--line); border-radius:10px; color:var(--ink); background:var(--surface); text-align:left; }
.radar-window-row.selected { border-color:var(--p); background:color-mix(in srgb,var(--p) 7%,var(--surface)); }
.radar-window-select { display:grid; min-width:0; gap:3px; padding:0; border:0; background:transparent; text-align:left; cursor:pointer; }
.radar-window-select strong { overflow:hidden; color:var(--ink); font-size:10px; text-overflow:ellipsis; white-space:nowrap; }
.radar-window-select small { overflow:hidden; color:var(--muted); font-size:9px; text-overflow:ellipsis; white-space:nowrap; }
.radar-window-score { color:#ff6b35; font-size:10px; font-weight:900; white-space:nowrap; }
.radar-window-convert { padding:5px 6px; border:0; border-radius:7px; color:var(--p); background:var(--panel); font-size:9px; font-weight:800; white-space:nowrap; cursor:pointer; }
.radar-window-convert:disabled { opacity:.5; cursor:not-allowed; }
.radar-inline-error { color:#d34e3b; font-size:10px; }
.radar-detail-actions { display:flex; flex:0 0 auto; gap:6px; padding:10px 12px calc(10px + env(safe-area-inset-bottom)); border-top:1px solid var(--line); background:var(--surface); }
.radar-create-trip { display:flex; min-width:0; min-height:40px; flex:1 1 auto; align-items:center; justify-content:center; padding:0 9px; border:0; border-radius:10px; color:var(--md-on-primary); background:var(--p); font-size:10px; font-weight:850; cursor:pointer; }
.radar-create-trip:disabled { cursor:not-allowed; opacity:.5; }
.radar-icon-action { display:grid; width:40px; min-width:40px; height:40px; flex:0 0 40px; place-items:center; border:1px solid var(--line); border-radius:10px; color:var(--ink); background:var(--panel); cursor:pointer; }
.radar-icon-action.danger { color:#e5484d; }
.radar-icon-action:disabled { cursor:not-allowed; opacity:.5; }
.radar-toast { position:fixed; z-index:9999; right:20px; bottom:20px; padding:9px 14px; border-radius:999px; color:var(--surface); background:var(--ink); font-size:11px; font-weight:750; box-shadow:0 8px 24px rgba(0,0,0,.25); }
@media (max-width:560px) {
  .radar-panel-header { padding:12px 13px 10px; gap:6px; }
  .radar-panel-heading { gap:7px; }
  .radar-panel-heading h2 { font-size:16px; }
  .radar-add-button { min-height:36px; padding-inline:8px; }
  .radar-list-summary { padding-inline:13px; }
  .radar-filter-pills { padding-inline:13px; }
  .radar-list-scroll,.radar-detail-scroll { padding-right:10px; padding-left:10px; }
  .radar-monitor-row { gap:7px; padding:9px 8px; }
  .radar-row-title { align-items:flex-start; flex-direction:column; gap:2px; }
  .radar-row-title strong { max-width:100%; }
  .radar-row-score { font-size:17px; }
  .radar-detail-actions { gap:4px; padding-inline:8px; }
  .radar-create-trip { font-size:9px; }
  .radar-icon-action { width:36px; min-width:36px; height:36px; flex-basis:36px; }
  .radar-window-row { grid-template-columns:minmax(0,1fr) auto; }
  .radar-window-convert { grid-column:1/-1; text-align:center; }
}
</style>
