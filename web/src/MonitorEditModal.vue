<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import UiSelect from './UiSelect.vue'

export interface MonitorCriteriaData {
  months?: number[]
  stargazing_mode?: boolean
  max_leave_days?: number
  max_cloud_cover?: number
  max_rain_prob?: number
  min_duration_days?: number
  max_duration_days?: number
}

export interface MonitorFormData {
  id?: string
  title: string
  place_name: string
  lat: number
  lng: number
  crs: string
  criteria: MonitorCriteriaData
  origin_city?: string
  max_budget?: number
}

const props = defineProps<{
  isOpen: boolean
  initialData?: MonitorFormData | null
  apiFetch?: (path: string, options?: RequestInit) => Promise<Response>
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', data: MonitorFormData): void
}>()

const title = ref('')
const placeName = ref('')
const lat = ref<number | ''>('')
const lng = ref<number | ''>('')
const crs = ref('gcj02')
const selectedMonths = ref<number[]>([6, 7, 8, 9])
const stargazingMode = ref(false)
const maxLeaveDays = ref(1)
const maxCloudCover = ref(30)
const maxRainProb = ref(20)
const minDuration = ref(2)
const maxDuration = ref(4)
const originCity = ref('')
const maxBudget = ref<number | ''>('')
const templateMode = ref<'custom' | 'stargazing' | 'weekend' | 'holiday'>('custom')

// 地点搜索相关状态
const searchQuery = ref('')
const searchResults = ref<any[]>([])
const searchLoading = ref(false)
const showManualCoords = ref(false)

async function doSearchPlaces() {
  const q = searchQuery.value.trim()
  if (!q) return
  searchLoading.value = true
  searchResults.value = []
  try {
    const fetchFn = props.apiFetch || window.fetch
    const res = await fetchFn('/api/v1/maps/pois/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        query: q,
        page: 1,
        page_size: 8,
      }),
    })
    if (res.ok) {
      const data = (await res.json()) as { items?: any[] }
      searchResults.value = data.items || []
    }
  } catch (err) {
    console.error('search POIs error:', err)
  } finally {
    searchLoading.value = false
  }
}

function selectCandidate(candidate: any) {
  placeName.value = candidate.name || ''
  if (!title.value || title.value.endsWith('最佳游玩时机监控')) {
    title.value = (candidate.name || '') + ' · 最佳游玩时机监控'
  }
  if (candidate.location) {
    lat.value = candidate.location.lat
    lng.value = candidate.location.lng
    crs.value = candidate.location.crs || 'gcj02'
  }
  searchResults.value = []
  showManualCoords.value = false
}

function clearSelectedPlace() {
  placeName.value = ''
  lat.value = ''
  lng.value = ''
  searchQuery.value = ''
  searchResults.value = []
}

const monthsList = [
  { val: 1, label: '1月' }, { val: 2, label: '2月' }, { val: 3, label: '3月' }, { val: 4, label: '4月' },
  { val: 5, label: '5月' }, { val: 6, label: '6月' }, { val: 7, label: '7月' }, { val: 8, label: '8月' },
  { val: 9, label: '9月' }, { val: 10, label: '10月' }, { val: 11, label: '11月' }, { val: 12, label: '12月' },
]

const crsOptions = [
  { value: 'gcj02', label: 'GCJ-02 (高德/腾讯)' },
  { value: 'bd09ll', label: 'BD-09LL (百度)' },
  { value: 'wgs84', label: 'WGS-84 (GPS标准)' },
]

const maxLeaveDaysOptions = [
  { value: 0, label: '0 天 (仅限周末/法定假无需请假)' },
  { value: 1, label: '最多请假 1 天 (如请周五休3天)' },
  { value: 2, label: '最多请假 2 天 (拼假连休4-5天)' },
  { value: 3, label: '最多请假 3 天 (长假探险)' },
]

watch(() => props.isOpen, (open) => {
  if (open) {
    if (props.initialData) {
      title.value = props.initialData.title || props.initialData.place_name || ''
      placeName.value = props.initialData.place_name || ''
      lat.value = props.initialData.lat ?? ''
      lng.value = props.initialData.lng ?? ''
      crs.value = props.initialData.crs || 'gcj02'
      selectedMonths.value = props.initialData.criteria?.months?.length ? [...props.initialData.criteria.months] : [5, 6, 7, 8, 9, 10]
      stargazingMode.value = props.initialData.criteria?.stargazing_mode ?? false
      maxLeaveDays.value = props.initialData.criteria?.max_leave_days ?? 1
      maxCloudCover.value = props.initialData.criteria?.max_cloud_cover ?? 30
      maxRainProb.value = props.initialData.criteria?.max_rain_prob ?? 20
      minDuration.value = props.initialData.criteria?.min_duration_days ?? 2
      maxDuration.value = props.initialData.criteria?.max_duration_days ?? 4
      originCity.value = props.initialData.origin_city || ''
      maxBudget.value = props.initialData.max_budget || ''
      templateMode.value = stargazingMode.value ? 'stargazing' : 'custom'
    } else {
      resetForm()
    }
  }
})

function resetForm() {
  searchQuery.value = ''
  searchResults.value = []
  showManualCoords.value = false
  title.value = ''
  placeName.value = ''
  lat.value = ''
  lng.value = ''
  crs.value = 'gcj02'
  selectedMonths.value = [5, 6, 7, 8, 9, 10]
  stargazingMode.value = false
  maxLeaveDays.value = 1
  maxCloudCover.value = 30
  maxRainProb.value = 20
  minDuration.value = 2
  maxDuration.value = 4
  originCity.value = ''
  maxBudget.value = ''
  templateMode.value = 'custom'
}

function applyTemplate(tpl: 'stargazing' | 'weekend' | 'holiday') {
  templateMode.value = tpl
  if (tpl === 'stargazing') {
    stargazingMode.value = true
    selectedMonths.value = [5, 6, 7, 8, 9] // 夏季至初秋银河季
    maxCloudCover.value = 25
    maxRainProb.value = 15
    maxLeaveDays.value = 1
    minDuration.value = 2
    maxDuration.value = 3
  } else if (tpl === 'weekend') {
    stargazingMode.value = false
    selectedMonths.value = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
    maxLeaveDays.value = 0 // 纯周末
    maxCloudCover.value = 45
    maxRainProb.value = 25
    minDuration.value = 2
    maxDuration.value = 2
  } else if (tpl === 'holiday') {
    stargazingMode.value = false
    selectedMonths.value = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
    maxLeaveDays.value = 2 // 拼假
    maxCloudCover.value = 40
    maxRainProb.value = 30
    minDuration.value = 3
    maxDuration.value = 5
  }
}

function toggleMonth(m: number) {
  const idx = selectedMonths.value.indexOf(m)
  if (idx >= 0) {
    if (selectedMonths.value.length > 1) {
      selectedMonths.value.splice(idx, 1)
    }
  } else {
    selectedMonths.value.push(m)
    selectedMonths.value.sort((a, b) => a - b)
  }
}

function selectAllMonths() {
  selectedMonths.value = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
}

const canSubmit = computed(() => {
  return placeName.value.trim() !== '' && typeof lat.value === 'number' && typeof lng.value === 'number'
})

function submit() {
  if (!canSubmit.value) return
  emit('save', {
    id: props.initialData?.id,
    title: title.value.trim() || placeName.value.trim(),
    place_name: placeName.value.trim(),
    lat: Number(lat.value),
    lng: Number(lng.value),
    crs: crs.value,
    criteria: {
      months: selectedMonths.value,
      stargazing_mode: stargazingMode.value,
      max_leave_days: maxLeaveDays.value,
      max_cloud_cover: maxCloudCover.value,
      max_rain_prob: maxRainProb.value,
      min_duration_days: minDuration.value,
      max_duration_days: maxDuration.value,
    },
    origin_city: originCity.value.trim(),
    max_budget: typeof maxBudget.value === 'number' ? maxBudget.value : 0,
  })
}
</script>

<template>
  <div v-if="isOpen" class="modal-backdrop monitor-modal-backdrop" @click.self="emit('close')">
    <div class="modal-panel monitor-modal" role="dialog" aria-modal="true" aria-labelledby="monitor-modal-title">
      <header class="monitor-modal-header">
        <div>
          <span class="eyebrow">OPPORTUNITY MONITOR</span>
          <h2 id="monitor-modal-title">{{ initialData?.id ? '编辑景点监控规则' : '新建景点时机监控' }}</h2>
          <p>设置天气、月相、节假日等偏好，系统将在最佳出行窗口触发智能推荐</p>
        </div>
        <button class="modal-close" type="button" aria-label="关闭弹窗" @click="emit('close')">×</button>
      </header>

      <div class="monitor-modal-body">
        <!-- 1. 预设快速模板 -->
        <section class="form-section">
          <label class="section-title">快速配置模板</label>
          <div class="template-pills">
            <button
              type="button"
              class="tpl-btn"
              :class="{ active: templateMode === 'stargazing' }"
              @click="applyTemplate('stargazing')"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l1.9 5.8a2 2 0 0 0 1.3 1.3L21 12l-5.8 1.9a2 2 0 0 0-1.3 1.3L12 21l-1.9-5.8a2 2 0 0 0-1.3-1.3L3 12l5.8-1.9a2 2 0 0 0 1.3-1.3L12 3z"/></svg>
              <span>银河与星空摄影</span>
            </button>
            <button
              type="button"
              class="tpl-btn"
              :class="{ active: templateMode === 'weekend' }"
              @click="applyTemplate('weekend')"
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="8" rx="2"/><path d="M5 11l2-5h10l2 5"/><circle cx="7" cy="19" r="2"/><circle cx="17" cy="19" r="2"/></svg>
              <span>周末短途自驾</span>
            </button>
            <button
              type="button"
              class="tpl-btn"
              :class="{ active: templateMode === 'holiday' }"
              @click="applyTemplate('holiday')"
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>
              <span>节假日拼假游</span>
            </button>
          </div>
        </section>

        <!-- 2. 目标景点与智能搜索 -->
        <section class="form-section">
          <div class="section-title-row">
            <label class="section-title">目标景点与坐标</label>
            <button
              type="button"
              class="text-btn toggle-manual-btn"
              @click="showManualCoords = !showManualCoords"
            >
              {{ showManualCoords ? '收起微调坐标' : '手动微调坐标' }}
            </button>
          </div>

          <!-- 地点快速搜索输入条 -->
          <div class="poi-search-bar">
            <div class="search-input-box">
              <input
                v-model="searchQuery"
                type="text"
                placeholder="输入景点名称搜索 (如：冷嘎措、西湖、莫尔格勒河)"
                @keydown.enter.prevent="doSearchPlaces"
              />
              <button
                v-if="searchQuery"
                type="button"
                class="search-clear-btn"
                aria-label="清空搜索"
                @click="searchQuery = ''; searchResults = []"
              >
                ×
              </button>
            </div>
            <button
              type="button"
              class="btn-do-search"
              :disabled="searchLoading || !searchQuery.trim()"
              @click="doSearchPlaces"
            >
              {{ searchLoading ? '搜索中…' : '搜索地点' }}
            </button>
          </div>

          <!-- 搜索候选结果列表 (下拉展开) -->
          <div v-if="searchResults.length > 0" class="poi-candidate-dropdown">
            <div
              v-for="(candidate, idx) in searchResults"
              :key="candidate.id || candidate.name + idx"
              class="poi-candidate-item"
              @click="selectCandidate(candidate)"
            >
              <div class="cand-details">
                <strong class="cand-title">{{ candidate.name }}</strong>
                <span class="cand-address">{{ candidate.address || '暂无详细地址' }}</span>
                <small class="cand-meta">
                  {{ candidate.location?.crs || 'gcj02' }} · {{ candidate.location?.lat.toFixed(5) }}, {{ candidate.location?.lng.toFixed(5) }} · {{ candidate.provider === 'amap' ? '高德' : '百度' }}
                </small>
              </div>
              <button type="button" class="cand-select-btn">选择此地</button>
            </div>
          </div>

          <!-- 已选定地点高亮卡片 -->
          <div v-if="placeName && typeof lat === 'number' && typeof lng === 'number'" class="selected-place-banner">
            <div class="selected-place-info">
              <span class="lock-tag">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                <span>已锁定景点</span>
              </span>
              <strong class="place-name">{{ placeName }}</strong>
              <small class="coords-info">{{ crs.toUpperCase() }} · 纬度 {{ lat.toFixed(5) }}, 经度 {{ lng.toFixed(5) }}</small>
            </div>
            <button type="button" class="btn-clear-place" title="清除已选" @click="clearSelectedPlace">重选</button>
          </div>

          <!-- 规则名称与备注 -->
          <div class="form-row">
            <label class="field-label">
              <span>景点名称 *</span>
              <input v-model="placeName" type="text" placeholder="例如：冷嘎措" required />
            </label>
            <label class="field-label">
              <span>监控规则备注</span>
              <input v-model="title" type="text" placeholder="如：冷嘎措拍贡嘎雪山与倒影" />
            </label>
          </div>

          <!-- 手动微调坐标 (支持折叠) -->
          <div v-if="showManualCoords" class="form-row coords-row">
            <label class="field-label">
              <span>纬度 (Lat) *</span>
              <input v-model.number="lat" type="number" step="any" placeholder="29.58" required />
            </label>
            <label class="field-label">
              <span>经度 (Lng) *</span>
              <input v-model.number="lng" type="number" step="any" placeholder="101.78" required />
            </label>
            <label class="field-label crs-field">
              <span>坐标系</span>
              <UiSelect v-model="crs" aria-label="坐标系" :options="crsOptions" />
            </label>
          </div>
        </section>

        <!-- 3. 适游月份 -->
        <section class="form-section">
          <div class="section-title-row">
            <label class="section-title">适游月份范围</label>
            <button type="button" class="text-btn" @click="selectAllMonths">全选月份</button>
          </div>
          <div class="months-grid">
            <button
              v-for="m in monthsList"
              :key="m.val"
              type="button"
              class="month-btn"
              :class="{ selected: selectedMonths.includes(m.val) }"
              @click="toggleMonth(m.val)"
            >
              {{ m.label }}
            </button>
          </div>
        </section>

        <!-- 4. 天文与气象要求 -->
        <section class="form-section">
          <label class="section-title">天文与气象条件</label>
          <label class="toggle-card" :class="{ active: stargazingMode }">
            <input v-model="stargazingMode" type="checkbox" />
            <div class="toggle-info">
              <strong>开启星空/银河摄影模式</strong>
              <small>优先推荐新月无月夜 (月照率 &lt; 15%)，避开满月强光害；自动计算夜间银河核心可见时长</small>
            </div>
          </label>
          <div class="form-row sliders-row">
            <label class="field-label">
              <span>最高允许云量: <strong>{{ maxCloudCover }}%</strong></span>
              <input v-model.number="maxCloudCover" type="range" min="10" max="80" step="5" />
            </label>
            <label class="field-label">
              <span>最高允许降水概率: <strong>{{ maxRainProb }}%</strong></span>
              <input v-model.number="maxRainProb" type="range" min="5" max="60" step="5" />
            </label>
          </div>
        </section>

        <!-- 5. 假期与拼假 -->
        <section class="form-section">
          <label class="section-title">工作与假期约束</label>
          <div class="form-row">
            <label class="field-label">
              <span>可接受请假天数</span>
              <UiSelect v-model="maxLeaveDays" aria-label="可接受请假天数" :options="maxLeaveDaysOptions" />
            </label>
            <label class="field-label">
              <span>期望游玩天数</span>
              <div class="inline-duration">
                <input v-model.number="minDuration" type="number" min="1" max="15" placeholder="最短" />
                <span>至</span>
                <input v-model.number="maxDuration" type="number" min="1" max="30" placeholder="最长" />
                <span>天</span>
              </div>
            </label>
          </div>
        </section>

        <!-- 6. 交通与预算 (可选) -->
        <section class="form-section">
          <label class="section-title">交通与往返预算 (可选)</label>
          <div class="form-row">
            <label class="field-label">
              <span>常住/出发城市</span>
              <input v-model="originCity" type="text" placeholder="如：成都、北京、上海" />
            </label>
            <label class="field-label">
              <span>交通预算上限 (元)</span>
              <input v-model.number="maxBudget" type="number" placeholder="留空不限制，如 1200" />
            </label>
          </div>
        </section>
      </div>

      <footer class="monitor-modal-footer">
        <button type="button" class="btn-cancel" @click="emit('close')">取消</button>
        <button type="button" class="btn-submit" :disabled="!canSubmit" @click="submit">
          {{ initialData?.id ? '保存规则' : '创建监控规则' }}
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.monitor-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--md-scrim);
  backdrop-filter: blur(8px);
  padding: 16px;
  box-sizing: border-box;
}

.monitor-modal {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(640px, 100%);
  max-height: min(92vh, 860px);
  background: var(--surface);
  color: var(--ink);
  border-radius: 20px;
  box-shadow: 0 20px 60px -10px rgba(0, 0, 0, 0.55);
  border: 1px solid var(--line);
  overflow: hidden;
  box-sizing: border-box;
  padding: 0;
}

.monitor-modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 18px 24px 14px;
  background: var(--surface);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.monitor-modal-header .eyebrow {
  margin: 0 0 2px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: var(--p);
}

.monitor-modal-header h2 {
  margin: 0;
  font-size: 19px;
  font-weight: 800;
  color: var(--ink) !important;
}

.monitor-modal-header p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 12px;
}

.monitor-modal-body {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 18px 24px;
  overflow-y: auto;
  flex: 1;
  box-sizing: border-box;
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.section-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.text-btn {
  background: transparent;
  border: 0;
  color: var(--p);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  padding: 0;
}

.template-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tpl-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  line-height: 1;
  padding: 8px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  color: var(--ink);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tpl-btn svg {
  flex-shrink: 0;
  display: block;
}

.tpl-btn:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--panel));
}

.tpl-btn.active {
  background: color-mix(in srgb, var(--p) 20%, var(--surface));
  border-color: var(--p);
  color: var(--p);
  box-shadow: 0 0 0 1px var(--p);
}

.form-row {
  display: flex;
  gap: 12px;
}

.coords-row .field-label {
  flex: 1;
}

.crs-field {
  flex: 1.2 !important;
}

.field-label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink);
}

.field-label input:not([type="range"]):not([type="checkbox"]) {
  width: 100%;
  min-height: 42px;
  padding: 8px 12px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--surface);
  color: var(--ink);
  font: inherit;
  box-sizing: border-box;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.field-label input:not([type="range"]):not([type="checkbox"]):focus {
  outline: none;
  border-color: var(--p);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--p) 14%, transparent);
}

.inline-duration {
  display: flex;
  align-items: center;
  gap: 6px;
}

.inline-duration input {
  width: 66px;
  text-align: center;
}

.months-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 6px;
}

.month-btn {
  padding: 6px 4px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface);
  color: var(--ink);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  text-align: center;
  transition: all 0.15s ease;
}

.month-btn:hover:not(.selected) {
  border-color: var(--p);
  background: color-mix(in srgb, var(--p) 8%, var(--surface));
}

.month-btn:focus-visible {
  outline: 2px solid var(--p);
  outline-offset: 1px;
}

.month-btn.selected {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  border-color: var(--p);
}

.toggle-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 14px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 12px;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.toggle-card:hover {
  border-color: color-mix(in srgb, var(--p) 45%, var(--line));
}

.toggle-card.active {
  border-color: var(--p);
  background: color-mix(in srgb, var(--p) 8%, var(--panel));
}

.toggle-card input {
  width: 18px;
  height: 18px;
  margin-top: 2px;
  accent-color: var(--p);
  flex-shrink: 0;
}

.toggle-info {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.toggle-info strong {
  font-size: 13px;
  color: var(--ink);
}

.toggle-info small {
  font-size: 11px;
  color: var(--muted);
  line-height: 1.4;
}

.sliders-row {
  margin-top: 8px;
}

.sliders-row .field-label span {
  display: flex;
  justify-content: space-between;
}

.sliders-row input[type="range"] {
  width: 100%;
  height: 28px;
  min-height: 0;
  padding: 0;
  border: 0;
  background: transparent;
  accent-color: var(--p);
  cursor: pointer;
}

.monitor-modal-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 24px;
  background: var(--surface);
  border-top: 1px solid var(--line);
  flex-shrink: 0;
}

.btn-cancel, .btn-submit {
  min-height: 42px;
  padding: 0 20px;
  border-radius: 11px;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-cancel {
  background: var(--panel);
  border: 1px solid var(--line);
  color: var(--ink);
}

.btn-cancel:hover {
  border-color: var(--p);
  color: var(--p);
  background: color-mix(in srgb, var(--p) 8%, var(--surface));
}

.btn-cancel:focus-visible, .btn-submit:focus-visible {
  outline: 2px solid var(--p);
  outline-offset: 2px;
}

.btn-submit {
  background: var(--p);
  border: 0;
  color: var(--md-on-primary, #ffffff);
}

.btn-submit:hover:not(:disabled) {
  background: color-mix(in srgb, var(--p) 88%, #000);
}

.btn-submit:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

@media (max-width: 580px) {
  .form-row {
    flex-direction: column;
    gap: 8px;
  }
  .months-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* 搜索输入条与候选结果样式 */
.poi-search-bar {
  display: flex;
  gap: 8px;
}

.search-input-box {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
}

.search-input-box input {
  width: 100%;
  min-height: 42px;
  padding: 8px 36px 8px 12px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--surface);
  color: var(--ink);
  font: inherit;
  box-sizing: border-box;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.search-input-box input:focus {
  outline: none;
  border-color: var(--p);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--p) 14%, transparent);
}

.search-clear-btn {
  position: absolute;
  right: 10px;
  background: transparent;
  border: 0;
  color: var(--muted);
  font-size: 16px;
  cursor: pointer;
  padding: 0;
}

.btn-do-search {
  min-height: 42px;
  padding: 0 16px;
  border: 1px solid var(--p);
  border-radius: 11px;
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.btn-do-search:hover:not(:disabled) {
  background: color-mix(in srgb, var(--p) 86%, #000);
}

.btn-do-search:focus-visible {
  outline: 2px solid var(--p);
  outline-offset: 2px;
}

.btn-do-search:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.poi-candidate-dropdown {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 220px;
  overflow-y: auto;
  padding: 6px;
  background: var(--panel);
  border: 1px solid var(--p);
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
}

.poi-candidate-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 10px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.poi-candidate-item:hover {
  border-color: var(--p);
  background: color-mix(in srgb, var(--p) 8%, var(--surface));
}

.cand-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.cand-title {
  font-size: 13px;
  color: var(--ink);
}

.cand-address {
  font-size: 11px;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cand-meta {
  font-size: 10px;
  color: var(--p);
}

.cand-select-btn {
  padding: 4px 10px;
  border: 1px solid var(--p);
  border-radius: 6px;
  background: color-mix(in srgb, var(--p) 15%, var(--surface));
  color: var(--p);
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
}

.selected-place-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  background: color-mix(in srgb, var(--p) 12%, var(--surface));
  border: 1px solid var(--p);
  border-radius: 10px;
}

.selected-place-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.lock-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  line-height: 1;
  font-size: 10px;
  font-weight: 800;
  color: var(--p);
  letter-spacing: 0.05em;
}

.lock-tag svg {
  flex-shrink: 0;
  display: block;
}

.place-name {
  font-size: 14px;
  color: var(--ink);
}

.coords-info {
  font-size: 11px;
  color: var(--muted);
}

.btn-clear-place {
  padding: 4px 8px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--surface);
  color: var(--ink);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-clear-place:hover {
  border-color: var(--p);
  color: var(--p);
  background: color-mix(in srgb, var(--p) 8%, var(--surface));
}
</style>
