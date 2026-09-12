<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { domToPng } from 'modern-screenshot'
import QrcodeVue from 'qrcode.vue'
import RouteTopologyMap from './RouteTopologyMap.vue'

type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord & { crs?: string }> }
type LinkData = { id?: string; title: string; url: string }
type Stop = { id: string; sequence: number; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown> }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; mode?: string; snapshots?: Array<{ geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number }> }
type Day = { id: string; date: string; title?: string; notes_markdown?: string; stops: Stop[]; legs?: Leg[] }
type TripDoc = { title: string; date_range?: { start: string; end: string }; timezone: string; description_markdown?: string; days: Day[] }

const props = withDefaults(defineProps<{
  isOpen: boolean
  trip: TripDoc | null
  shareUrl?: string
  currentTheme?: 'light' | 'dark'
}>(), {
  shareUrl: '',
  currentTheme: 'light',
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'createShare'): void
}>()

const posterCardRef = ref<HTMLElement | null>(null)
const posterTheme = ref<'light' | 'dark'>('light')
const posterLayout = ref<'long' | 'day'>('long')
type DescLinesMode = '2' | '4' | 'all'
const descLinesMode = ref<DescLinesMode>('4')
const activeDayIndex = ref<number>(0)
const isGenerating = ref(false)
const generatedImage = ref<string>('')
const toastMessage = ref('')

watch(() => props.currentTheme, (val) => {
  if (val) posterTheme.value = val
}, { immediate: true })

watch(() => props.isOpen, (open) => {
  if (open) {
    generatedImage.value = ''
    toastMessage.value = ''
    activeDayIndex.value = 0
  }
})

const fallbackShareUrl = computed(() => {
  return props.shareUrl || window.location.href
})

const allDays = computed(() => props.trip?.days || [])

const allStops = computed(() => {
  const result: Stop[] = []
  allDays.value.forEach(d => {
    if (d.stops) result.push(...d.stops)
  })
  return result
})

const allLegs = computed(() => {
  const result: Leg[] = []
  allDays.value.forEach(d => {
    if (d.legs) result.push(...d.legs)
  })
  return result
})

const currentDisplayDays = computed(() => {
  if (!props.trip) return []
  if (posterLayout.value === 'long') {
    return allDays.value
  }
  return allDays.value.slice(activeDayIndex.value, activeDayIndex.value + 1)
})

const currentDisplayStops = computed(() => {
  if (posterLayout.value === 'long') {
    return allStops.value
  }
  return currentDisplayDays.value[0]?.stops || []
})

const currentDisplayLegs = computed(() => {
  if (posterLayout.value === 'long') {
    return allLegs.value
  }
  return currentDisplayDays.value[0]?.legs || []
})

const totalDistanceKm = computed(() => {
  let meters = 0
  allLegs.value.forEach(leg => {
    const d = leg.snapshots?.[0]?.distance_m
    if (typeof d === 'number') meters += d
  })
  return meters > 0 ? Math.round(meters / 1000) : 0
})

function formatDisplayDate(dateStr?: string) {
  if (!dateStr) return ''
  return dateStr.replaceAll('-', '/')
}

function formatDuration(seconds?: number) {
  if (!seconds) return ''
  const mins = Math.round(seconds / 60)
  if (mins < 60) return `${mins}分钟`
  const hours = Math.floor(mins / 60)
  const remMins = mins % 60
  return remMins > 0 ? `${hours}小时${remMins}分` : `${hours}小时`
}

function formatDistance(meters?: number) {
  if (!meters) return ''
  if (meters < 1000) return `${meters}米`
  return `${(meters / 1000).toFixed(1)}公里`
}

function modeLabel(mode?: string) {
  if (!mode) return '出行'
  const map: Record<string, string> = {
    driving: '驾车',
    walking: '步行',
    cycling: '骑行',
    transit: '公共交通',
  }
  return map[mode] || mode
}

function stripMarkdown(text?: string) {
  if (!text) return ''
  return text
    .replace(/#+\s/g, '')
    .replace(/\*\*(.*?)\*\*/g, '$1')
    .replace(/\*(.*?)\*/g, '$1')
    .replace(/\[(.*?)\]\(.*?\)/g, '$1')
    .trim()
}

async function exportImage() {
  if (!posterCardRef.value) return
  isGenerating.value = true
  toastMessage.value = '正在渲染高清海报…'

  try {
    await nextTick()
    // 等待微任务完成以确保 DOM 状态稳定
    await new Promise(r => setTimeout(r, 150))

    const dataUrl = await domToPng(posterCardRef.value, {
      scale: 2, // 2x Retina 高清导出
      backgroundColor: posterTheme.value === 'dark' ? '#0f172a' : '#ffffff',
    })

    generatedImage.value = dataUrl
    toastMessage.value = '海报已生成！已为你触发下载，手机端也可长按图片保存。'

    // 尝试直接下载
    const a = document.createElement('a')
    const sanitizedTitle = (props.trip?.title || 'journeyin-trip').replace(/[/\\?%*:|"<>]/g, '-')
    const suffix = posterLayout.value === 'day' ? `-Day${activeDayIndex.value + 1}` : '-All'
    a.download = `${sanitizedTitle}${suffix}.png`
    a.href = dataUrl
    a.click()
  } catch (err) {
    console.error('Export error:', err)
    toastMessage.value = '生成图片失败，请重试或检查浏览器设置。'
  } finally {
    isGenerating.value = false
  }
}

async function shareNative() {
  if (!generatedImage.value) {
    await exportImage()
  }
  if (!generatedImage.value) return

  // 尝试使用 Web Share API 分享文件
  try {
    if (navigator.share && navigator.canShare) {
      const res = await fetch(generatedImage.value)
      const blob = await res.blob()
      const file = new File([blob], 'trip-share.png', { type: 'image/png' })
      if (navigator.canShare({ files: [file] })) {
        await navigator.share({
          title: props.trip?.title || '旅行行程分享',
          text: `查看我的旅行行程【${props.trip?.title || 'JourneyIn'}】：`,
          files: [file],
        })
        return
      }
    }
    toastMessage.value = '当前设备不支持直接调起分享面板，已生成下载图片，请长按下方预览图保存。'
  } catch (e) {
    // 用户取消分享不视作错误
    if ((e as Error).name !== 'AbortError') {
      toastMessage.value = '调起分享失败，请直接保存图片。'
    }
  }
}
</script>

<template>
  <div v-if="isOpen" class="modal-backdrop poster-modal-backdrop" @click.self="emit('close')">
    <section class="modal-panel poster-modal-panel" role="dialog" aria-modal="true" aria-labelledby="poster-modal-title">
      <!-- 弹窗顶栏 -->
      <header class="poster-modal-header">
        <div>
          <p class="eyebrow">SHARE AS IMAGE</p>
          <h2 id="poster-modal-title">分享行程海报</h2>
        </div>
        <button class="modal-close" type="button" aria-label="关闭海报弹窗" @click="emit('close')">×</button>
      </header>

      <!-- 样式与版式控制工具条 -->
      <div class="poster-controls-bar">
        <div class="control-group">
          <span class="control-label">版式</span>
          <div class="segmented-pill">
            <button
              type="button"
              :class="{ active: posterLayout === 'long' }"
              @click="posterLayout = 'long'"
            >
              全景长图
            </button>
            <button
              type="button"
              :class="{ active: posterLayout === 'day' }"
              @click="posterLayout = 'day'"
            >
              单日卡片
            </button>
          </div>
        </div>

        <div v-if="posterLayout === 'day' && allDays.length > 1" class="control-group">
          <span class="control-label">天数</span>
          <div class="day-picker-pills">
            <button
              v-for="(d, idx) in allDays"
              :key="d.id"
              type="button"
              class="day-pill-btn"
              :class="{ active: activeDayIndex === idx }"
              @click="activeDayIndex = idx"
            >
              Day {{ idx + 1 }}
            </button>
          </div>
        </div>

        <div class="control-group">
          <span class="control-label">色彩</span>
          <div class="segmented-pill">
            <button
              type="button"
              :class="{ active: posterTheme === 'light' }"
              @click="posterTheme = 'light'"
            >
              清新明亮
            </button>
            <button
              type="button"
              :class="{ active: posterTheme === 'dark' }"
              @click="posterTheme = 'dark'"
            >
              深色质感
            </button>
          </div>
        </div>

        <div class="control-group">
          <span class="control-label">地点说明</span>
          <div class="segmented-pill">
            <button
              type="button"
              :class="{ active: descLinesMode === '2' }"
              @click="descLinesMode = '2'"
            >
              2行
            </button>
            <button
              type="button"
              :class="{ active: descLinesMode === '4' }"
              @click="descLinesMode = '4'"
            >
              4行
            </button>
            <button
              type="button"
              :class="{ active: descLinesMode === 'all' }"
              @click="descLinesMode = 'all'"
            >
              全部
            </button>
          </div>
        </div>
      </div>

      <!-- 海报渲染与预览区 (外层可滚动) -->
      <div class="poster-scroll-wrapper">
        <!-- 真正被截取生成图片的核心 DOM 容器 -->
        <div
          ref="posterCardRef"
          class="poster-card"
          :class="[posterTheme]"
        >
          <!-- 海报头部 -->
          <div class="poster-hero">
            <div class="poster-brand">
              <span class="brand-sparkle">✦</span>
              <span class="brand-title">JOURNEYIN ITINERARY</span>
            </div>
            <h1 class="poster-trip-title">{{ trip?.title || '旅行规划' }}</h1>
            <div class="poster-meta-row">
              <span class="meta-date">
                {{ formatDisplayDate(trip?.date_range?.start) }} - {{ formatDisplayDate(trip?.date_range?.end) }}
              </span>
              <span class="meta-badge">{{ allDays.length }} 天行程</span>
              <span class="meta-badge">{{ currentDisplayStops.length }} 个规划点</span>
              <span v-if="totalDistanceKm > 0" class="meta-badge">总里程 ~{{ totalDistanceKm }}km</span>
            </div>
            <p v-if="trip?.description_markdown && posterLayout === 'long'" class="poster-summary-quote">
              {{ stripMarkdown(trip.description_markdown).slice(0, 120) }}{{ (trip.description_markdown.length > 120) ? '…' : '' }}
            </p>
          </div>

          <!-- 路线拓扑图板块 -->
          <div class="poster-map-section">
            <div class="section-title-tag">
              <span>{{ posterLayout === 'day' ? `DAY ${activeDayIndex + 1} 路线示意` : '全线空间拓扑' }}</span>
            </div>
            <RouteTopologyMap
              :stops="currentDisplayStops"
              :legs="currentDisplayLegs"
              :theme="posterTheme"
              :width="512"
              :height="260"
            />
          </div>

          <!-- 每日详细时间轴 -->
          <div class="poster-timeline-section">
            <div v-for="(day, dIdx) in currentDisplayDays" :key="day.id" class="day-section-block">
              <div class="day-heading">
                <div class="day-badge-wrap">
                  <span class="day-num-tag">Day {{ posterLayout === 'day' ? activeDayIndex + 1 : dIdx + 1 }}</span>
                  <span class="day-date-str">{{ formatDisplayDate(day.date) }}</span>
                </div>
                <div v-if="day.title" class="day-sub-title">{{ day.title }}</div>
              </div>

              <p v-if="day.notes_markdown" class="day-notes-box" :class="'clamp-' + descLinesMode">
                {{ stripMarkdown(day.notes_markdown) }}
              </p>

              <!-- 当天的地点列表 -->
              <div class="stops-timeline">
                <div v-for="(stop, sIdx) in day.stops" :key="stop.id" class="timeline-step">
                  <div class="step-axis">
                    <span class="step-circle">{{ sIdx + 1 }}</span>
                    <span v-if="sIdx < day.stops.length - 1" class="step-line"></span>
                  </div>
                  <div class="step-content">
                    <div class="step-header">
                      <strong class="stop-name">{{ stop.title }}</strong>
                      <span v-if="stop.time_window?.arrival" class="stop-time">
                        {{ stop.time_window.arrival }} 到达
                      </span>
                    </div>
                    <p v-if="stop.address" class="stop-address">{{ stop.address }}</p>
                    <p v-if="stop.description_markdown" class="stop-desc" :class="'clamp-' + descLinesMode">
                      {{ stripMarkdown(stop.description_markdown) }}
                    </p>

                    <!-- 下一段交通 -->
                    <div v-if="day.legs?.[sIdx]" class="step-leg-info">
                      <span class="leg-icon">↳</span>
                      <span class="leg-mode">{{ modeLabel(day.legs[sIdx].mode) }}</span>
                      <span v-if="day.legs[sIdx].snapshots?.[0]?.distance_m" class="leg-detail">
                        {{ formatDistance(day.legs[sIdx].snapshots?.[0]?.distance_m) }}
                      </span>
                      <span v-if="day.legs[sIdx].snapshots?.[0]?.duration_s" class="leg-detail">
                        约 {{ formatDuration(day.legs[sIdx].snapshots?.[0]?.duration_s) }}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 海报底部：二维码与水印 -->
          <footer class="poster-footer-card">
            <div class="footer-left">
              <div class="footer-brand-title">JourneyIn 线路规划</div>
              <p class="footer-slogan">在地图上规划每一段旅程 · 扫码查看实时交互地图与导航</p>
              <span class="footer-copy">Made with JourneyIn · 只读快照分享</span>
            </div>
            <div class="footer-qr">
              <QrcodeVue
                :value="fallbackShareUrl"
                :size="72"
                level="M"
                render-as="svg"
                class="qr-svg-wrapper"
              />
            </div>
          </footer>
        </div>
      </div>

      <!-- 提示反馈信息 -->
      <div v-if="toastMessage" class="poster-toast-bar">
        <span>{{ toastMessage }}</span>
      </div>

      <!-- 弹窗底部操作按钮 -->
      <footer class="poster-modal-actions">
        <button type="button" class="secondary-action" @click="emit('close')">关闭</button>
        <button
          type="button"
          class="secondary-action"
          :disabled="isGenerating"
          @click="shareNative"
        >
          系统分享
        </button>
        <button
          type="button"
          class="primary-action"
          :disabled="isGenerating"
          @click="exportImage"
        >
          <span v-if="isGenerating" class="spin-dot"></span>
          {{ isGenerating ? '正在渲染图片…' : '保存高清海报' }}
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.poster-modal-backdrop {
  position: fixed;
  inset: 0;
  background-color: color-mix(in srgb, #000 68%, transparent);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  padding: 16px;
}

.poster-modal-panel {
  position: relative;
  background: var(--surface);
  color: var(--ink);
  border: 1px solid var(--line);
  border-radius: 24px;
  width: 100%;
  max-width: 660px;
  height: min(92vh, 880px);
  display: flex;
  flex-direction: column;
  box-shadow: 0 24px 80px color-mix(in srgb, #000 45%, transparent);
  overflow: hidden;
}

.poster-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 24px;
  background: var(--surface);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.poster-modal-header h2 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--ink);
}

.poster-modal-header .eyebrow {
  margin: 0 0 2px;
  font-size: 0.6875rem;
  letter-spacing: 0.08em;
  color: var(--p);
  font-weight: 800;
}

.poster-controls-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  padding: 12px 24px;
  background: var(--md-surface-container-low);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.control-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.control-label {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--muted);
}

.segmented-pill {
  display: inline-flex;
  background: var(--md-surface-container-high);
  border: 1px solid var(--line);
  border-radius: 20px;
  padding: 2px;
}

.segmented-pill button {
  border: none;
  background: transparent;
  padding: 4px 12px;
  border-radius: 16px;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--muted);
  cursor: pointer;
  transition: all 0.2s ease;
}

.segmented-pill button.active {
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 1px 4px color-mix(in srgb, #000 15%, transparent);
}

.day-picker-pills {
  display: inline-flex;
  gap: 4px;
  flex-wrap: wrap;
}

.day-pill-btn {
  border: 1px solid var(--line);
  background: var(--surface);
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.75rem;
  color: var(--muted);
  cursor: pointer;
  transition: all 0.2s ease;
}

.day-pill-btn.active {
  background: var(--p);
  color: var(--md-on-primary);
  border-color: var(--p);
  font-weight: 700;
}

/* 核心滚动容器：支持横向和纵向流动，窄屏下可横向滑动浏览 */
.poster-scroll-wrapper {
  flex: 1;
  min-height: 0;
  overflow-x: auto;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  padding: 24px 16px;
  background: var(--md-surface-container);
  display: block;
}

/* 核心海报容器：固定保持 560px 规整宽度，绝不因窄屏被压缩变形 */
.poster-card {
  width: 560px;
  min-width: 560px;
  margin: 0 auto;
  box-sizing: border-box;
  border-radius: 24px;
  padding: 28px 24px;
  box-shadow: 0 12px 40px color-mix(in srgb, #000 30%, transparent);
  transition: background 0.25s ease;
  overflow: visible;
}

.poster-card.light {
  background: #ffffff;
  color: #1e293b;
}

.poster-card.dark {
  background: #0f172a;
  color: #f8fafc;
}

.poster-hero {
  margin-bottom: 20px;
}

.poster-brand {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}

.brand-sparkle {
  color: #0284c7;
  font-size: 1rem;
}

.poster-card.dark .brand-sparkle {
  color: #38bdf8;
}

.brand-title {
  font-size: 0.6875rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  color: #0284c7;
}

.poster-card.dark .brand-title {
  color: #38bdf8;
}

.poster-trip-title {
  font-size: 1.5rem;
  font-weight: 800;
  margin: 0 0 10px;
  line-height: 1.3;
}

.poster-meta-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 0.8125rem;
}

.meta-date {
  font-weight: 600;
  opacity: 0.85;
}

.meta-badge {
  background: rgba(2, 132, 199, 0.12);
  color: #0284c7;
  padding: 2px 8px;
  border-radius: 12px;
  font-weight: 600;
  font-size: 0.75rem;
}

.poster-card.dark .meta-badge {
  background: rgba(56, 189, 248, 0.15);
  color: #38bdf8;
}

.poster-summary-quote {
  margin: 12px 0 0;
  padding: 8px 12px;
  font-size: 0.8125rem;
  line-height: 1.5;
  border-left: 3px solid #0284c7;
  background: rgba(2, 132, 199, 0.05);
  border-radius: 0 8px 8px 0;
  opacity: 0.9;
}

.poster-card.dark .poster-summary-quote {
  border-left-color: #38bdf8;
  background: rgba(56, 189, 248, 0.08);
}

.poster-map-section {
  margin-bottom: 24px;
}

.section-title-tag {
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  color: #64748b;
  margin-bottom: 8px;
}

.poster-card.dark .section-title-tag {
  color: #94a3b8;
}

.poster-timeline-section {
  display: flex;
  flex-direction: column;
  gap: 20px;
  margin-bottom: 28px;
}

.day-section-block {
  border-radius: 16px;
  padding: 16px;
  background: rgba(148, 163, 184, 0.08);
  border: 1px solid rgba(148, 163, 184, 0.15);
}

.poster-card.dark .day-section-block {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 255, 255, 0.08);
}

.day-heading {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px 16px;
  margin-bottom: 12px;
}

.day-badge-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  white-space: nowrap;
}

.day-num-tag {
  background: #0284c7;
  color: #ffffff;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 700;
  white-space: nowrap;
  flex-shrink: 0;
  line-height: 1.2;
}

.poster-card.dark .day-num-tag {
  background: #0284c7;
}

.day-date-str {
  font-size: 0.875rem;
  font-weight: 600;
  white-space: nowrap;
  flex-shrink: 0;
}

.day-sub-title {
  font-size: 0.8125rem;
  opacity: 0.75;
  line-height: 1.45;
  word-break: break-word;
  text-align: right;
  flex: 1 1 200px;
}

.day-notes-box {
  margin: 0 0 12px;
  font-size: 0.8125rem;
  line-height: 1.5;
  opacity: 0.8;
  word-break: break-word;
}

.stops-timeline {
  display: flex;
  flex-direction: column;
}

.timeline-step {
  display: flex;
  gap: 12px;
}

.step-axis {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 20px;
}

.step-circle {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #0284c7;
  color: #ffffff;
  font-size: 0.6875rem;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.poster-card.dark .step-circle {
  background: #0284c7;
}

.step-line {
  width: 2px;
  flex: 1;
  background: #cbd5e1;
  margin: 4px 0;
}

.poster-card.dark .step-line {
  background: #334155;
}

.step-content {
  flex: 1;
  padding-bottom: 16px;
}

.step-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.stop-name {
  font-size: 0.9375rem;
  font-weight: 700;
}

.stop-time {
  font-size: 0.75rem;
  color: #0284c7;
  font-weight: 600;
}

.poster-card.dark .stop-time {
  color: #38bdf8;
}

.stop-address {
  margin: 2px 0 4px;
  font-size: 0.75rem;
  opacity: 0.7;
}

.stop-desc {
  margin: 4px 0 6px;
  font-size: 0.8125rem;
  line-height: 1.5;
  opacity: 0.85;
  word-break: break-word;
}

.stop-desc.clamp-2,
.day-notes-box.clamp-2 {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
  text-overflow: ellipsis;
}

.stop-desc.clamp-4,
.day-notes-box.clamp-4 {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 4;
  overflow: hidden;
  text-overflow: ellipsis;
}

.stop-desc.clamp-all,
.day-notes-box.clamp-all {
  display: block;
  overflow: visible;
}

.step-leg-info {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.6875rem;
  color: #64748b;
  background: rgba(148, 163, 184, 0.12);
  padding: 2px 8px;
  border-radius: 4px;
  margin-top: 4px;
}

.poster-card.dark .step-leg-info {
  background: rgba(255, 255, 255, 0.08);
  color: #94a3b8;
}

.poster-footer-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-top: 20px;
  border-top: 1px dashed rgba(148, 163, 184, 0.3);
}

.poster-card.dark .poster-footer-card {
  border-top-color: rgba(255, 255, 255, 0.15);
}

.footer-left {
  flex: 1;
}

.footer-brand-title {
  font-size: 0.9375rem;
  font-weight: 800;
  color: #0284c7;
}

.poster-card.dark .footer-brand-title {
  color: #38bdf8;
}

.footer-slogan {
  margin: 2px 0 4px;
  font-size: 0.75rem;
  opacity: 0.8;
  line-height: 1.4;
}

.footer-copy {
  font-size: 0.6875rem;
  opacity: 0.5;
}

.footer-qr {
  background: #ffffff;
  padding: 6px;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
}

.poster-toast-bar {
  padding: 10px 24px;
  background: var(--md-surface-container-low);
  color: var(--p);
  font-size: 0.8125rem;
  font-weight: 600;
  text-align: center;
  border-top: 1px solid var(--line);
  flex-shrink: 0;
}

.poster-modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  background: var(--surface);
  border-top: 1px solid var(--line);
  flex-shrink: 0;
}

.spin-dot {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 2px solid #ffffff;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: 6px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 640px) {
  .poster-modal-backdrop {
    padding: 8px;
  }
  .poster-modal-panel {
    border-radius: 20px;
    height: 96vh;
    max-height: 96vh;
  }
  .poster-modal-header,
  .poster-controls-bar,
  .poster-modal-actions {
    padding-inline: 16px;
  }
  .poster-controls-bar {
    gap: 10px;
    padding-block: 10px;
  }
  .poster-scroll-wrapper {
    padding: 16px 10px;
  }
}
</style>
