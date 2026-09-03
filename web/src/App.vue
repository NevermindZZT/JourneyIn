<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  IonApp, IonChip, IonIcon,
} from '@ionic/vue'
import BMapLoader from '@baidumap/jsapi-loader'
import AMapLoader from '@amap/amap-jsapi-loader'
import { addOutline, chevronDownOutline, chevronUpOutline, closeOutline, cloudOfflineOutline, createOutline, linkOutline, logInOutline, mapOutline, menuOutline, navigateOutline, searchOutline, settingsOutline, sunnyOutline } from 'ionicons/icons'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import PrototypePreview from './PrototypePreview.vue'
import UiSelect from './UiSelect.vue'
import MarkdownEditor from './MarkdownEditor.vue'

type Theme = 'system' | 'light' | 'dark'
type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord & { crs?: string }>; source?: string; provider_refs?: Record<string, unknown>; citycode?: string; adcode?: string; geocoded_at?: string; precision?: string; confidence?: number }
type LinkData = { id?: string; title: string; url: string; kind?: string }
type Stop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown>; children?: SubStop[] }
type SubStop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown> }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; mode?: string; snapshots?: Array<{ provider?: string; coordinate_system?: string; mode?: string; strategy?: string; source?: string; geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; fetched_at?: string }> }
type Day = { id: string; date: string; title?: string; notes_markdown?: string; stops: Stop[]; legs?: Leg[] }
type TripDocument = { title: string; date_range?: { start: string; end: string }; timezone: string; description_markdown?: string; links?: LinkData[]; map?: { preferred_provider?: 'baidu' | 'amap'; enabled_providers?: Array<'baidu' | 'amap'>; default_mode?: TravelMode }; days: Day[] }
type SharedBootstrap = { trip: TripDocument & { id?: string; status?: string }; browser_key?: string; amap_browser_key?: string; amap_security_proxy_path?: string; amap_security_js_code_configured?: boolean; default_map_provider?: 'baidu' | 'amap'; revision?: number }
type TripSummary = { id: string; title: string; status: string; start_date: string; end_date: string; timezone: string; revision: number; days?: number; stops?: number; updated_at?: string }
type TripHistoryEntry = { id: string; history_id?: string; trip_id: string; source_revision: number; title: string; start_date: string; end_date: string; label?: string; content_hash: string; created_at: string; read_only?: boolean }
type TripSortMode = 'updated' | 'date'
type Capabilities = { version?: string; default_map_provider?: 'baidu' | 'amap'; map_providers?: { baidu?: { browser_key_configured?: boolean; browser_key?: string }; amap?: { browser_key_configured?: boolean; browser_key?: string; security_proxy_path?: string; security_js_code_configured?: boolean } }; features?: { planning_point_edit?: boolean; coordinate_repair?: boolean }; mcp?: { http_endpoint?: string } }
type KeySettings = { map?: { default_provider?: 'baidu' | 'amap'; baidu?: { browser_key_configured?: boolean; server_key_configured?: boolean }; amap?: { js_key_configured?: boolean; server_key_configured?: boolean; security_js_code_configured?: boolean } }; poi?: { provider_priority?: 'amap' | 'baidu'; local_directory_count?: number } }
type PlaceCandidate = { id?: string; name: string; address?: string; location: Coord & { crs?: string }; provider?: string; citycode?: string; adcode?: string; typecode?: string }
type TravelMode = 'driving' | 'walking' | 'cycling' | 'transit'

const mapProviderOptions = [
  { value: 'baidu', label: '百度地图', description: 'Baidu Maps' },
  { value: 'amap', label: '高德地图', description: 'AMap' },
]
const travelModeOptions = [
  { value: 'walking', label: '步行' },
  { value: 'driving', label: '驾车' },
  { value: 'cycling', label: '骑行' },
  { value: 'transit', label: '公交' },
]
const drivingStrategyOptions = [
  { value: '32', label: '高德推荐' },
  { value: '33', label: '躲避拥堵' },
  { value: '34', label: '高速优先' },
  { value: '35', label: '不走高速' },
  { value: '36', label: '少收费' },
]
const searchCategoryOptions = [
  { value: 'all', label: '全部地点' },
  { value: '旅游景点', label: '景点' },
  { value: '酒店', label: '酒店' },
  { value: '餐饮', label: '餐饮' },
]
const poiPriorityOptions = [
  { value: 'amap', label: '高德优先' },
  { value: 'baidu', label: '百度优先' },
]
const tripSortOptions = [
  { value: 'updated', label: '最后修改' },
  { value: 'date', label: '行程日期' },
]

const APP_VERSION = import.meta.env.VITE_JOURNEYIN_VERSION
const APP_SLOGAN = '在地图上规划每一段旅程'
const GITHUB_URL = 'https://github.com/NevermindZZT/JourneyIn'
function formatDate(value?: string) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(raw)
  return match ? match[1] + '/' + match[2] + '/' + match[3] : raw.replaceAll('-', '/')
}
function formatDateTime(value?: string) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return formatDate(raw)
  const pad = (part: number) => String(part).padStart(2, '0')
  return date.getFullYear() + '/' + pad(date.getMonth() + 1) + '/' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
}
function formatDateRange(start?: string, end?: string) {
  return [formatDate(start), formatDate(end)].filter(Boolean).join(' — ')
}
function inclusiveDayCount(start: string, end: string) {
  if (!start || !end) return 0
  const startTime = Date.parse(start + 'T00:00:00Z')
  const endTime = Date.parse(end + 'T00:00:00Z')
  if (Number.isNaN(startTime) || Number.isNaN(endTime) || endTime < startTime) return 0
  return Math.floor((endTime - startTime) / 86400000) + 1
}
const markdownRenderer = new MarkdownIt({ html: false, breaks: true, linkify: false, typographer: false })
markdownRenderer.validateLink = (url: string) => /^https?:\/\//i.test(url.trim())
const markdownRendererConfig = {
  ALLOW_DATA_ATTR: false,
  ALLOWED_ATTR: ['alt', 'class', 'height', 'href', 'loading', 'referrerpolicy', 'rel', 'src', 'start', 'target', 'title', 'width'],
  ALLOWED_TAGS: ['a', 'blockquote', 'br', 'code', 'del', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', 'img', 'li', 'ol', 'p', 'pre', 's', 'strong', 'table', 'tbody', 'td', 'tfoot', 'th', 'thead', 'tr', 'ul'],
  ALLOWED_URI_REGEXP: /^https?:\/\//i,
}
markdownRenderer.renderer.rules.link_open = (tokens, index, options, _env, self) => {
  tokens[index].attrSet('target', '_blank')
  tokens[index].attrSet('rel', 'noopener noreferrer')
  return self.renderToken(tokens, index, options)
}
markdownRenderer.renderer.rules.image = (tokens, index, options, _env, self) => {
  tokens[index].attrSet('loading', 'lazy')
  tokens[index].attrSet('referrerpolicy', 'no-referrer')
  return self.renderToken(tokens, index, options)
}
const shareMode = window.location.pathname.startsWith('/s/') && !window.location.pathname.endsWith('.json')
const prototypeMode = new URLSearchParams(window.location.search).get('prototype') === '1'
const redesignMode = true

declare global { interface Window { BMap?: any; BMapGL?: any; BMAP_NORMAL_MAP?: any; BMAP_SATELLITE_MAP?: any; _AMapSecurityConfig?: { securityJsCode?: string; serviceHost?: string } } }

const trips = ref<TripSummary[]>([])
const savedTripSortMode = localStorage.getItem('journeyin.tripSort')
const tripSortMode = ref<TripSortMode>(savedTripSortMode === 'date' ? 'date' : 'updated')
const sortedTrips = computed(() => {
  const items = [...trips.value]
  return items.sort((a, b) => {
    if (tripSortMode.value === 'date') {
      const dateOrder = a.start_date.localeCompare(b.start_date) || a.end_date.localeCompare(b.end_date)
      if (dateOrder) return dateOrder
    }
    const updatedA = Date.parse(a.updated_at || '') || 0
    const updatedB = Date.parse(b.updated_at || '') || 0
    return updatedB - updatedA || a.title.localeCompare(b.title)
  })
})
watch(tripSortMode, value => localStorage.setItem('journeyin.tripSort', value))
const selected = ref<TripSummary | null>(null)
const tripDocument = ref<TripDocument | null>(null)
const capabilities = ref<Capabilities | null>(null)
const selectedStopId = ref('')
const selectedSubStopId = ref('')
const selectedLegId = ref('')
const searchParentStopId = ref('')
const weatherLoading = ref(false)
const descriptionEditing = ref(false)
const descriptionDraft = ref('')
const arrivalTimeDraft = ref('')
const departureTimeDraft = ref('')
const descriptionEditorMode = ref<MarkdownEditorMode>('edit')
const descriptionSaving = ref(false)
const pointEditorOpen = ref(false)
const pointEditorTargetID = ref('')
const pointEditorDayID = ref('')
const pointEditorTitleDraft = ref('')
const pointEditorAddressDraft = ref('')
const pointEditorSaving = ref(false)
const pointEditorTitleInput = ref<HTMLInputElement | null>(null)
const pointUpdateNotice = ref('')
const descriptionFullscreen = ref(false)
const tripDescriptionEditing = ref(false)
const tripDescriptionFullscreen = ref(false)
const tripDescriptionDraft = ref('')
const tripDescriptionEditorMode = ref<MarkdownEditorMode>('edit')
const tripDescriptionSaving = ref(false)
const tripDetailsEditing = ref(false)
const tripDetailsTitleDraft = ref('')
const tripDetailsStartDateDraft = ref('')
const tripDetailsEndDateDraft = ref('')
const tripDetailsSaving = ref(false)
const tripDetailsIdempotencyKey = ref('')
const tripDetailsTitleInput = ref<HTMLInputElement | null>(null)
const tripDetailsNotice = ref('')
const historyOpen = ref(false)
const historyLoading = ref(false)
const historySaving = ref(false)
const historyDeletingID = ref('')
const historyEntries = ref<TripHistoryEntry[]>([])
const historyLabelDraft = ref('')
const historyMessage = ref('')
const historyError = ref('')
const historyView = ref<TripHistoryEntry | null>(null)
const readOnlyView = computed(() => shareMode || Boolean(historyView.value))
const historyTitle = computed(() => historyView.value?.title || selected.value?.title || tripDocument.value?.title || '行程地图')
const historyDateRange = computed(() => historyView.value ? formatDateRange(historyView.value.start_date, historyView.value.end_date) : selected.value ? formatDateRange(selected.value.start_date, selected.value.end_date) : tripDateRangeFor(tripDocument.value).start ? formatDateRange(tripDateRangeFor(tripDocument.value).start, tripDateRangeFor(tripDocument.value).end) : '')
const stopDateEditing = ref(false)
const stopDateDraftDayID = ref('')
const stopDateSaving = ref(false)
const selectedDay = ref<number | 'all'>('all')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const mapError = ref('')
const mapWarning = ref('')
const mapReady = ref(false)
const mapContainer = ref<HTMLElement | null>(null)
const tripListScroll = ref<HTMLElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const shareURL = ref('')
const shareID = ref('')
const shareExpiresAt = ref('')
const shareCopyMessage = ref('')
const shareNoticeVisible = ref(false)
const actionLoading = ref(false)
const settingsOpen = ref(false)
type SettingsSection = 'appearance' | 'connection' | 'maps' | 'search' | 'sharing' | 'mcp' | 'about'
type MarkdownEditorMode = 'edit' | 'preview'
const settingsSection = ref<SettingsSection>('appearance')
const newTripOpen = ref(false)
const authOpen = ref(false)
const loginUsername = ref('')
const loginPassword = ref('')
const loginMessage = ref('')
const loginLoading = ref(false)
const authTokenInput = ref(localStorage.getItem('journeyin.apiToken') || '')
const serverURL = ref(window.location.origin)
const theme = ref<Theme>((localStorage.getItem('journeyin.theme') as Theme) || 'system')
const newTitle = ref('我的旅行规划')
const newStartDate = ref(new Date().toISOString().slice(0, 10))
const newEndDate = ref(new Date().toISOString().slice(0, 10))
const newTimezone = ref('Asia/Shanghai')
const newDescription = ref('')
const settingsMessage = ref('')
const settingsData = ref<KeySettings | null>(null)
const defaultMapProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.mapProvider') === 'amap' ? 'amap' : 'baidu')
const baiduBrowserKeyInput = ref('')
const baiduServerKeyInput = ref('')
const amapJSKeyInput = ref('')
const amapServerKeyInput = ref('')
const amapSecurityJSCodeInput = ref('')
const poiProviderPriority = ref<'amap' | 'baidu'>('amap')
const localDirectoryCount = ref(0)
const settingsSaving = ref(false)
const panelOpen = ref(localStorage.getItem('journeyin.panelOpen') !== 'false')
const panelCollapsed = ref(localStorage.getItem('journeyin.panelCollapsed') === 'true')
const mobileMapToolsOpen = ref(false)
const detailCollapsed = ref(localStorage.getItem('journeyin.detailCollapsed') === 'true')
const tripView = ref<'list' | 'detail'>('list')
type SheetBreakpoint = 'peek' | 'half' | 'expanded'
type JourneyLayer = 'list' | 'trip' | 'stop' | 'substop'
const sheetBreakpoint = ref<SheetBreakpoint>('half')
type JourneySection = 'overview' | 'itinerary'
const journeySection = ref<JourneySection>('itinerary')
const tripMenuID = ref('')
const detailMoreOpen = ref(false)
const sheetDragActive = ref(false)
const sheetDragHeight = ref<number | null>(null)
const sheetDragStyle = computed<Record<string, string> | undefined>(() => {
  if (sheetDragHeight.value === null) return undefined
  return {
    height: sheetDragHeight.value + 'px',
    maxHeight: 'none',
    top: sheetDragActive.value ? 'auto' : sheetBreakpoint.value === 'expanded' ? '60px' : 'auto',
  }
})
const navigationApplying = ref(false)
let navigationSequence = 0
const mapType = ref<'normal' | 'satellite'>((localStorage.getItem('journeyin.mapType') as 'normal' | 'satellite') || 'normal')
const showMapLabels = ref(localStorage.getItem('journeyin.mapLabels') !== 'false')
const mapPickMode = ref(false)
const mapPickOpen = ref(false)
const mapPickTitle = ref('')
const mapPickAddress = ref('')
const mapPickDayID = ref('')
const mapPickLocation = ref<Coord & { crs: string } | null>(null)
const mapPickTargetID = ref('')
const panelMode = ref<'journey' | 'search'>('journey')
const searchQuery = ref('')
const searchRegion = ref('')
const searchCategory = ref<'all' | '旅游景点' | '酒店' | '餐饮'>('all')
const searchResults = ref<PlaceCandidate[]>([])
const selectedSearchResultIndex = ref(-1)
const searchResultMarkers: any[] = []
const searchLoading = ref(false)
const searchMessage = ref('')
type LocationSearchMode = 'add' | 'repair'
const locationSearchMode = ref<LocationSearchMode>('add')
const locationSearchTargetID = ref('')
const locationSearchTargetDayID = ref('')
const locationSearchTitleDraft = ref('')
const planningMode = ref<TravelMode>('walking')
const planningStrategy = ref('32')
const planningProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.planningProvider') === 'amap' ? 'amap' : 'baidu')
const selectedMapProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.mapProvider') === 'amap' ? 'amap' : 'baidu')
const supportsDrivingStrategy = computed(() => planningProvider.value === 'amap')
const availableDrivingStrategyOptions = computed(() => supportsDrivingStrategy.value ? drivingStrategyOptions : [])
const planningLoading = ref(false)
watch([planningProvider, planningMode], () => {
  if (planningMode.value !== 'driving' || !supportsDrivingStrategy.value) planningStrategy.value = ''
  else if (!drivingStrategyOptions.some(option => option.value === planningStrategy.value)) planningStrategy.value = drivingStrategyOptions[0]?.value || ''
})
const reorderMessage = ref('')
const reorderMode = ref(false)
let mapInstance: any = null
let mapAPI: any = null
let mapScriptPromise: Promise<void> | null = null
let amapScriptPromise: Promise<void> | null = null
let mapOverlays: any[] = []
let amapSatelliteLayer: any = null
let mediaQuery: MediaQueryList | null = null
let mapReadyTimer: number | null = null
let loadedMapKey = ''
let loadedAMapKey = ''
let mapRenderVersion = 0
let mapFocusVersion = 0

const baiduKey = computed(() => capabilities.value?.map_providers?.baidu?.browser_key || '')
const amapKey = computed(() => capabilities.value?.map_providers?.amap?.browser_key || '')
const key = computed(() => selectedMapProvider.value === 'amap' ? amapKey.value : baiduKey.value)
const keyConfigured = computed(() => Boolean(key.value))
const mapProviderLabel = computed(() => selectedMapProvider.value === 'amap' ? '高德地图' : '百度地图')
function hasUsableRoute(document: TripDocument | null, provider: 'baidu' | 'amap', mode?: TravelMode) {
  return Boolean(document?.days.some(day => (day.legs || []).some(leg => (leg.snapshots || []).some(snapshot => snapshot.provider === provider && (!mode || !snapshot.mode || snapshot.mode === mode) && (snapshot.geometry?.length || 0) > 1))))
}
function firstRouteMode(document: TripDocument | null, provider: 'baidu' | 'amap'): TravelMode | null {
  for (const day of document?.days || []) for (const leg of day.legs || []) for (const snapshot of leg.snapshots || []) {
    if (snapshot.provider !== provider || (snapshot.geometry?.length || 0) < 2) continue
    if (snapshot.mode === 'driving' || snapshot.mode === 'walking' || snapshot.mode === 'cycling' || snapshot.mode === 'transit') return snapshot.mode
  }
  return null
}
function syncProviderFromDocument(document: TripDocument | null) {
  const preferred = document?.map?.preferred_provider
  let provider = preferred === 'baidu' || preferred === 'amap' ? preferred : defaultMapProvider.value
  if (!hasUsableRoute(document, provider)) {
    const alternate = provider === 'amap' ? 'baidu' : 'amap'
    if (hasUsableRoute(document, alternate)) provider = alternate
  }
  const configuredMode = document?.map?.default_mode
  let mode: TravelMode = configuredMode === 'driving' || configuredMode === 'walking' || configuredMode === 'cycling' || configuredMode === 'transit' ? configuredMode : planningMode.value
  if (!hasUsableRoute(document, provider, mode)) mode = firstRouteMode(document, provider) || mode
  const changed = selectedMapProvider.value !== provider
  selectedMapProvider.value = provider
  planningProvider.value = provider
  planningMode.value = mode
  if (changed) resetMapSDK()
}
const visibleDays = computed(() => {
  if (!tripDocument.value) return []
  return selectedDay.value === 'all' ? tripDocument.value.days : tripDocument.value.days.filter((_, index) => index + 1 === selectedDay.value)
})
const tripDayOptions = computed(() => (tripDocument.value?.days || []).map((day, index) => ({ value: day.id, label: 'D' + (index + 1) + ' · ' + formatDate(day.date), description: day.title || '第 ' + (index + 1) + ' 天' })))
function orderedStops(stops: Stop[]) { return [...stops].sort((a, b) => a.sequence - b.sequence) }
const visibleStops = computed(() => visibleDays.value.flatMap(day => orderedStops(day.stops || [])))
const carryOverStop = computed<Stop | null>(() => {
  if (!tripDocument.value || selectedDay.value === 'all' || selectedDay.value <= 1) return null
  const previousDay = tripDocument.value.days[selectedDay.value - 2]
  const previousStops = orderedStops(previousDay?.stops || [])
  return previousStops[previousStops.length - 1] || null
})
const mapStops = computed(() => {
  const carryOver = carryOverStop.value
  if (!carryOver || !visibleStops.value.length || visibleStops.value.some(stop => stop.id === carryOver.id)) return visibleStops.value
  return [carryOver, ...visibleStops.value]
})
const visibleRouteSummary = computed(() => {
  let distanceM = 0; let durationS = 0; let segments = 0; let zeroSegments = 0
  for (const day of visibleDays.value) for (const leg of day.legs || []) {
    const snapshot = chooseSnapshotMetadata(leg, selectedMapProvider.value, planningMode.value)
    if (!snapshot) continue
    if (!snapshot.geometry || snapshot.geometry.length < 2) {
      if (snapshot.source === 'journeyin-same-location') zeroSegments++
      continue
    }
    distanceM += snapshot.distance_m || 0; durationS += snapshot.duration_s || 0; segments++
  }
  return { distanceM, durationS, segments, zeroSegments }
})
const hasCarryOverRoute = computed(() => {
  const carryOver = carryOverStop.value
  return Boolean(carryOver && visibleDays.value.some(day => (day.legs || []).some(leg => leg.from_stop_id === carryOver.id && chooseSnapshot(leg, selectedMapProvider.value, planningMode.value))))
})
function canPlanDay(day: Day) {
  const stops = day.stops || []
  if (stops.length >= 2) return true
  if (stops.length !== 1 || !tripDocument.value) return false
  const dayIndex = tripDocument.value.days.indexOf(day)
  return dayIndex > 0 && orderedStops(tripDocument.value.days[dayIndex - 1].stops || []).length > 0
}
const plannableDays = computed(() => visibleDays.value.filter(canPlanDay))
const selectedStop = computed(() => visibleStops.value.find(stop => stop.id === selectedStopId.value) || null)
const selectedSubStop = computed(() => selectedStop.value?.children?.find(child => child.id === selectedSubStopId.value) || null)
const selectedTarget = computed<Stop | SubStop | null>(() => selectedSubStop.value || selectedStop.value || null)
const unlocatedMainStops = computed(() => (tripDocument.value?.days || []).flatMap(day => day.stops || []).filter(stop => !pointFor(stop)))
const unlocatedPlanningPoints = computed(() => (tripDocument.value?.days || []).flatMap(day => (day.stops || []).flatMap(stop => [stop, ...(stop.children || [])])).filter(stop => !pointFor(stop)))
const canPlanRoutes = computed(() => plannableDays.value.length > 0 && unlocatedMainStops.value.length === 0)
function tripDateRangeFor(document: TripDocument | null, summary: TripSummary | null = selected.value) {
  return {
    start: document?.date_range?.start || summary?.start_date || document?.days[0]?.date || '',
    end: document?.date_range?.end || summary?.end_date || document?.days[document.days.length - 1]?.date || '',
  }
}
function planningPointCount(day: Day) {
  return (day.stops || []).reduce((count, stop) => count + 1 + (stop.children?.length || 0), 0)
}
const tripDetailsTitleCount = computed(() => Array.from(tripDetailsTitleDraft.value).length)
const tripDetailsDayCount = computed(() => inclusiveDayCount(tripDetailsStartDateDraft.value, tripDetailsEndDateDraft.value))
const tripDetailsOriginalDateRange = computed(() => tripDateRangeFor(tripDocument.value, selected.value))
const tripDetailsDateChanged = computed(() => Boolean(tripDetailsStartDateDraft.value && tripDetailsEndDateDraft.value) && (tripDetailsStartDateDraft.value !== tripDetailsOriginalDateRange.value.start || tripDetailsEndDateDraft.value !== tripDetailsOriginalDateRange.value.end))
const tripDetailsRemovedDays = computed(() => {
  const days = tripDocument.value?.days || []
  const count = tripDetailsDayCount.value
  return count > 0 && count < days.length ? days.slice(count) : []
})
const tripDetailsBlockingDays = computed(() => tripDetailsRemovedDays.value.filter(day => planningPointCount(day) > 0))
const tripDetailsDateError = computed(() => {
  const start = tripDetailsStartDateDraft.value
  const end = tripDetailsEndDateDraft.value
  if (!start || !end) return '请选择开始日期和结束日期'
  const count = tripDetailsDayCount.value
  if (count <= 0) return '请输入有效的日期范围'
  if (count > 60) return '行程最多支持 60 天'
  return ''
})
const tripDetailsDateHint = computed(() => {
  if (!tripDetailsDateChanged.value || tripDetailsDateError.value) return ''
  const currentDays = tripDocument.value?.days.length || 0
  const nextDays = tripDetailsDayCount.value
  if (nextDays > currentDays) return '保存后会按 D1、D2 的顺序保留现有安排，并新增 ' + (nextDays - currentDays) + ' 个空白日。'
  if (nextDays < currentDays) return '保存后会移除末尾 ' + (currentDays - nextDays) + ' 个日期；包含规划点的日期不能被移除。'
  return '现有安排会按 D1、D2 的顺序对应到新的开始日期。'
})
const tripDetailsCanSave = computed(() => Boolean(selected.value && tripDocument.value && tripDetailsTitleDraft.value.trim() && !tripDetailsDateError.value && !tripDetailsBlockingDays.value.length && !tripDetailsSaving.value))
function stopDate(stop: Stop | SubStop) {
  const day = tripDocument.value?.days.find(item => item.stops.some(stopItem => stopItem.id === stop.id || stopItem.children?.some(child => child.id === stop.id)))
  return formatDate(day?.date) || '日期待定'
}
function dayForStop(stop: Stop | SubStop) { return tripDocument.value?.days.find(day => day.stops.some(item => item.id === stop.id || item.children?.some(child => child.id === stop.id))) || null }
function stopTime(stop: Stop | SubStop) {
  const arrival = stop.time_window?.arrival?.trim() || ''
  const departure = stop.time_window?.departure?.trim() || ''
  if (!arrival && !departure) return ''
  if (arrival && departure) return arrival + ' — ' + departure
  return arrival || departure
}

function beginEditStopDate() {
  if (readOnlyView.value || selectedSubStop.value || !selectedStop.value) return
  stopDateDraftDayID.value = dayForStop(selectedStop.value)?.id || ''
  stopDateEditing.value = Boolean(stopDateDraftDayID.value)
}
function cancelEditStopDate() {
  stopDateEditing.value = false
  stopDateDraftDayID.value = ''
}
async function saveStopDate() {
  if (!selected.value || !tripDocument.value || !selectedStop.value) return
  const stop = selectedStop.value
  const sourceDay = dayForStop(stop)
  const targetDay = tripDocument.value.days.find(day => day.id === stopDateDraftDayID.value)
  if (!sourceDay || !targetDay) { error.value = '无法确定规划点日期'; return }
  if (sourceDay.id === targetDay.id) { cancelEditStopDate(); return }
  const stopID = stop.id
  const previousDay = selectedDay.value
  const targetDayIndex = tripDocument.value.days.indexOf(targetDay)
  stopDateSaving.value = true
  error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(sourceDay.id) + '/stops/' + encodeURIComponent(stopID) + '/move', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ target_day_id: targetDay.id }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '修改规划点日期失败')
    applyTripPayload(payload)
    selectedStopId.value = stopID
    selectedSubStopId.value = ''
    if (previousDay !== 'all' && targetDayIndex >= 0) selectedDay.value = targetDayIndex + 1
    cancelEditStopDate()
    reorderMessage.value = '规划点已移动到 D' + (targetDayIndex + 1)
    syncNavigationURL('replace')
    await renderMap()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '修改规划点日期失败' } finally { stopDateSaving.value = false }
}
const themeLabel = computed(() => theme.value === 'system' ? '跟随系统' : theme.value === 'dark' ? '深色' : '浅色')
const displayVersion = computed(() => capabilities.value?.version || APP_VERSION)

function apiFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  const headers = new Headers(init.headers)
  const token = authTokenInput.value.trim()
  if (token) headers.set('Authorization', 'Bearer ' + token)
  return fetch(input, { ...init, headers, credentials: 'same-origin' }).then(response => {
    if (response.status === 401) { authOpen.value = true; settingsMessage.value = '当前服务需要登录令牌' }
    return response
  })
}

type NavigationURLState = {
  layer: JourneyLayer
  tripID?: string
  stopID?: string
  subStopID?: string
  day?: number | 'all'
  sheet?: SheetBreakpoint
}

type JourneyHistoryState = { journeyin?: NavigationURLState & { depth: number } }

function validSheet(value: string | null): SheetBreakpoint {
  return value === 'peek' || value === 'expanded' ? value : 'half'
}

function readNavigationURL(): NavigationURLState {
  const params = new URLSearchParams(window.location.search)
  const tripID = params.get('trip') || undefined
  const stopID = params.get('stop') || undefined
  const subStopID = params.get('substop') || undefined
  const dayValue = params.get('day')
  const parsedDay = dayValue && dayValue !== 'all' ? Number(dayValue) : 'all'
  const day = parsedDay === 'all' || Number.isInteger(parsedDay) && parsedDay > 0 ? parsedDay : 'all'
  const layer: JourneyLayer = subStopID ? 'substop' : stopID ? 'stop' : tripID ? 'trip' : 'list'
  return { layer, tripID, stopID, subStopID, day, sheet: validSheet(params.get('sheet')) }
}

function currentNavigationState(): NavigationURLState {
  if (tripView.value !== 'detail' || !selected.value) return { layer: 'list' }
  return {
    layer: selectedSubStopId.value ? 'substop' : selectedStopId.value ? 'stop' : 'trip',
    tripID: selected.value.id,
    stopID: selectedStopId.value || undefined,
    subStopID: selectedSubStopId.value || undefined,
    day: selectedDay.value,
    sheet: sheetBreakpoint.value,
  }
}

function syncNavigationURL(mode: 'push' | 'replace' = 'replace', state = currentNavigationState()) {
  if (readOnlyView.value || prototypeMode) return
  const url = new URL(window.location.href)
  for (const key of ['trip', 'stop', 'substop', 'day', 'sheet']) url.searchParams.delete(key)
  if (state.tripID && state.layer !== 'list') {
    url.searchParams.set('trip', state.tripID)
    if (state.stopID) url.searchParams.set('stop', state.stopID)
    if (state.subStopID) url.searchParams.set('substop', state.subStopID)
    if (state.day !== undefined) url.searchParams.set('day', String(state.day))
    if (state.sheet) url.searchParams.set('sheet', state.sheet)
  }
  const current = window.history.state as JourneyHistoryState | null
  const currentDepth = current?.journeyin?.depth || 0
  const nextState: JourneyHistoryState = { journeyin: { ...state, depth: mode === 'push' ? currentDepth + 1 : currentDepth } }
  if (mode === 'push') window.history.pushState(nextState, '', url.toString())
  else window.history.replaceState(nextState, '', url.toString())
}

function ensureNavigationHistory() {
  if (readOnlyView.value || prototypeMode || window.history.state?.journeyin) return
  syncNavigationURL('replace', readNavigationURL())
}

const SHEET_PEEK_HEIGHT = 154
const SHEET_TOP_OFFSET = 60
const SHEET_BOTTOM_OFFSET = 8
function sheetViewportHeight() { return window.visualViewport?.height || window.innerHeight }
function sheetHeightBounds() {
  const viewportHeight = sheetViewportHeight()
  const max = Math.max(SHEET_PEEK_HEIGHT, viewportHeight - SHEET_TOP_OFFSET - SHEET_BOTTOM_OFFSET)
  const half = Math.min(max, Math.max(SHEET_PEEK_HEIGHT, Math.round(viewportHeight * 0.53)))
  return { min: SHEET_PEEK_HEIGHT, half, max }
}
function sheetHeightForBreakpoint(breakpoint: SheetBreakpoint) {
  const bounds = sheetHeightBounds()
  return breakpoint === 'peek' ? bounds.min : breakpoint === 'half' ? bounds.half : bounds.max
}
function nearestSheetBreakpoint(height: number) {
  const bounds = sheetHeightBounds()
  const options: Array<[SheetBreakpoint, number]> = [['peek', bounds.min], ['half', bounds.half], ['expanded', bounds.max]]
  return options.reduce((closest, option) => Math.abs(option[1] - height) < Math.abs(closest[1] - height) ? option : closest)[0]
}

function setSheetBreakpoint(next: SheetBreakpoint, mode: 'push' | 'replace' = 'replace', sync = true) {
  sheetBreakpoint.value = next
  panelCollapsed.value = next === 'peek'
  detailCollapsed.value = next === 'peek'
  if (sync) syncNavigationURL(mode)
  void nextTick().then(() => {
    mapInstance?.resize?.()
    if (selectedTarget.value) focusSelectedMapTarget()
    else recenterMapToVisibleViewport()
  })
  if (sheetRecenterTimer !== null) { window.clearTimeout(sheetRecenterTimer); sheetRecenterTimer = null }
  sheetRecenterTimer = window.setTimeout(() => {
    sheetRecenterTimer = null
    mapInstance?.resize?.()
    if (selectedTarget.value) focusSelectedMapTarget()
    else fitVisibleMapContent()
  }, 260)
}

let sheetDragCleanup: (() => void) | null = null
let sheetDragReleaseTimer: number | null = null
let sheetRecenterTimer: number | null = null
let suppressSheetClick = false

type TouchGestureState = { pointerId: number; startX: number; startY: number; moved: boolean; target: EventTarget | null }
let touchGesture: TouchGestureState | null = null

function isEditableTouchTarget(target: EventTarget | null) {
  const element = target instanceof Element ? target : null
  return Boolean(element?.closest('input, textarea, select, [contenteditable="true"], [contenteditable=""]'))
}

function blurNonEditableTouchFocus() {
  const active = document.activeElement
  if (active instanceof HTMLElement && !isEditableTouchTarget(active)) active.blur()
}

function handleTouchPointerDown(event: PointerEvent) {
  if (event.pointerType === 'mouse') return
  document.documentElement.classList.add('journey-touch-gesture')
  touchGesture = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, moved: false, target: event.target }
}

function handleTouchPointerMove(event: PointerEvent) {
  if (!touchGesture || touchGesture.pointerId !== event.pointerId || touchGesture.moved) return
  if (Math.hypot(event.clientX - touchGesture.startX, event.clientY - touchGesture.startY) < 8) return
  touchGesture.moved = true
  if (!isEditableTouchTarget(touchGesture.target)) blurNonEditableTouchFocus()
}

function finishTouchPointer(event: PointerEvent) {
  if (!touchGesture || touchGesture.pointerId !== event.pointerId) return
  const gesture = touchGesture
  touchGesture = null
  document.documentElement.classList.remove('journey-touch-gesture')
  if (isEditableTouchTarget(gesture.target)) return
  if (gesture.moved) blurNonEditableTouchFocus()
  else window.setTimeout(blurNonEditableTouchFocus, 0)
}

function cycleSheetBreakpoint(event: MouseEvent) {
  if (suppressSheetClick) {
    suppressSheetClick = false
    event.preventDefault()
    return
  }
  setSheetBreakpoint(sheetBreakpoint.value === 'peek' ? 'half' : 'peek')
}

function toggleDetailSheet() {
  const next = sheetBreakpoint.value === 'peek' ? 'half' : sheetBreakpoint.value === 'half' ? 'expanded' : 'half'
  setSheetBreakpoint(next)
}

function toggleDetailMore() {
  detailMoreOpen.value = !detailMoreOpen.value
}
function editSelectedDescriptionFromMenu() {
  detailMoreOpen.value = false
  beginEditPoint()
}
function editSelectedContentFromMenu() {
  detailMoreOpen.value = false
  beginEditDescription()
}
function deleteSelectedPointFromMenu() {
  detailMoreOpen.value = false
  const target = selectedTarget.value
  if (target) void deletePlanningPoint(target)
}

function startSheetDrag(event: PointerEvent) {
  if (window.matchMedia('(min-width: 901px)').matches) return
  sheetDragCleanup?.()
  if (sheetDragReleaseTimer !== null) {
    window.clearTimeout(sheetDragReleaseTimer)
    sheetDragReleaseTimer = null
  }
  const panel = (event.currentTarget as HTMLElement | null)?.closest<HTMLElement>('.workspace-panel, .stop-detail-panel')
  const startHeight = panel?.getBoundingClientRect().height || sheetHeightForBreakpoint(sheetBreakpoint.value)
  const startY = event.clientY
  let moved = false
  sheetDragHeight.value = startHeight
  sheetDragActive.value = true
  const onMove = (moveEvent: PointerEvent) => {
    const delta = startY - moveEvent.clientY
    if (Math.abs(delta) > 8) moved = true
    if (!moved) return
    moveEvent.preventDefault()
    const bounds = sheetHeightBounds()
    sheetDragHeight.value = Math.min(bounds.max, Math.max(bounds.min, startHeight + delta))
  }
  const onUp = () => {
    if (moved) {
      const currentHeight = sheetDragHeight.value ?? startHeight
      const next = nearestSheetBreakpoint(currentHeight)
      suppressSheetClick = true
      sheetDragActive.value = false
      setSheetBreakpoint(next)
      sheetDragHeight.value = sheetHeightForBreakpoint(next)
      sheetDragReleaseTimer = window.setTimeout(() => {
        sheetDragHeight.value = null
        sheetDragReleaseTimer = null
      }, 220)
      window.setTimeout(() => { suppressSheetClick = false }, 0)
    } else {
      sheetDragActive.value = false
      sheetDragHeight.value = null
    }
    cleanup()
  }
  const onCancel = () => {
    sheetDragActive.value = false
    sheetDragHeight.value = null
    cleanup()
  }
  const cleanup = () => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    sheetDragCleanup = null
  }
  sheetDragCleanup = cleanup
  window.addEventListener('pointermove', onMove, { passive: false })
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onCancel)
}

function navigateToList(mode: 'push' | 'replace' = 'push') {
  historyOpen.value = false
  historyView.value = null
  cancelEditTripDetails()
  selected.value = null
  tripDocument.value = null
  tripView.value = 'list'
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  panelMode.value = 'journey'
  journeySection.value = 'itinerary'
  panelOpen.value = false
  mobileMapToolsOpen.value = false
  tripMenuID.value = ''
  detailMoreOpen.value = false
  cancelEditStopDate()
  resetMapSDK()
  syncNavigationURL(mode, { layer: 'list' })
}

function navigateToTrip(trip: TripSummary, mode: 'push' | 'replace' = 'push') {
  historyOpen.value = false
  historyView.value = null
  cancelEditTripDetails()
  selected.value = trip
  tripView.value = 'detail'
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  panelMode.value = 'journey'
  journeySection.value = 'itinerary'
  panelOpen.value = true
  mobileMapToolsOpen.value = false
  tripMenuID.value = ''
  detailMoreOpen.value = false
  cancelEditStopDate()
  setSheetBreakpoint('half', 'replace', false)
  syncNavigationURL(mode, { layer: 'trip', tripID: trip.id, day: 'all', sheet: 'half' })
  void loadDetail(trip)
}

function navigateToStop(stop: Stop | SubStop, parent: Stop | null = null, mode: 'push' | 'replace' = 'push') {
  const resolvedParent = parent || parentForStop(stop)
  if (!resolvedParent) return
  selectedStopId.value = resolvedParent.id
  selectedSubStopId.value = stop.id === resolvedParent.id ? '' : stop.id
  panelMode.value = 'journey'
  panelOpen.value = true
  mobileMapToolsOpen.value = false
  detailMoreOpen.value = false
  cancelEditStopDate()
  setSheetBreakpoint('half', 'replace', false)
  syncNavigationURL(mode)
  void renderMap()
}

async function applyNavigationRoute(route: NavigationURLState) {
  if (route.layer === 'list' || !route.tripID) {
    navigateToList('replace')
    return
  }
  const trip = trips.value.find(item => item.id === route.tripID)
  if (!trip) {
    navigateToList('replace')
    return
  }
  if (selected.value?.id !== trip.id || !tripDocument.value) await loadDetail(trip)
  selected.value = trip
  tripView.value = 'detail'
  panelMode.value = 'journey'
  journeySection.value = route.layer === 'trip' ? 'itinerary' : 'itinerary'
  panelOpen.value = true
  detailMoreOpen.value = false
  selectedDay.value = route.day && route.day !== 'all' && route.day <= (tripDocument.value?.days.length || 0) ? route.day : 'all'
  setSheetBreakpoint(route.sheet || (route.layer === 'trip' ? 'half' : 'expanded'), 'replace', false)
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  const targetID = route.subStopID || route.stopID
  const target = targetID ? findPlanningPoint(targetID) : null
  if (target) {
    const parent = parentForStop(target)
    if (parent) {
      selectedStopId.value = parent.id
      selectedSubStopId.value = target.id === parent.id ? '' : target.id
    }
  }
  await nextTick()
  await renderMap()
  syncNavigationURL('replace')
}

async function handleNavigationPopState() {
  if (readOnlyView.value || prototypeMode) return
  const sequence = ++navigationSequence
  navigationApplying.value = true
  try {
    await applyNavigationRoute(readNavigationURL())
  } finally {
    if (sequence === navigationSequence) navigationApplying.value = false
  }
}

function handleGlobalKeyDown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (descriptionFullscreen.value && descriptionEditing.value) { closeDescriptionFullscreen(); event.preventDefault(); return }
  if (tripDescriptionFullscreen.value && tripDescriptionEditing.value) { closeTripDescriptionFullscreen(); event.preventDefault(); return }
  if (pointEditorOpen.value) { cancelEditPoint(); event.preventDefault(); return }
  if (tripDetailsEditing.value) { cancelEditTripDetails(); event.preventDefault(); return }
  if (historyOpen.value) { historyOpen.value = false; event.preventDefault(); return }
  if (historyView.value) { void exitTripHistory(); event.preventDefault(); return }
  if (mapPickOpen.value || mapPickMode.value) { cancelMapPick(); event.preventDefault(); return }
  if (newTripOpen.value) { newTripOpen.value = false; event.preventDefault(); return }
  if (settingsOpen.value) { settingsOpen.value = false; event.preventDefault(); return }
  if (authOpen.value) { authOpen.value = false; event.preventDefault(); return }
  if (mobileMapToolsOpen.value) { mobileMapToolsOpen.value = false; event.preventDefault(); return }
  if (panelMode.value === 'search') { closeJourneySearch(); event.preventDefault(); return }
  if (detailMoreOpen.value) { detailMoreOpen.value = false; event.preventDefault(); return }
  if (selectedSubStopId.value) { navigateBackFromSubStop(); event.preventDefault(); return }
  if (selectedStopId.value) { navigateBackFromStop(); event.preventDefault(); return }
  if (tripView.value === 'detail') { navigateBackToList(); event.preventDefault() }
}

function focusSelectedMapTarget() {
  const target = selectedTarget.value
  if (!target || !mapInstance) return
  if (selectedMapProvider.value === 'amap') focusAMapPoint(target)
  else focusMapOnPoint(target)
}

function mapVisibleRect(): { left: number; top: number; right: number; bottom: number } | null {
  const container = mapContainer.value
  if (!container) return null
  const containerRect = container.getBoundingClientRect()
  const size = mapInstance?.getContainerSize?.()
  const width = Number(size?.width) || container.clientWidth || containerRect.width
  const height = Number(size?.height) || container.clientHeight || containerRect.height
  if (!(width > 0) || !(height > 0)) return null
  let left = 0; let top = 0; let right = width; let bottom = height
  document.querySelectorAll<HTMLElement>('.floating-panel, .details-drawer').forEach(overlay => {
    const rect = overlay.getBoundingClientRect()
    const overlapWidth = Math.min(containerRect.right, rect.right) - Math.max(containerRect.left, rect.left)
    const overlapHeight = Math.min(containerRect.bottom, rect.bottom) - Math.max(containerRect.top, rect.top)
    if (overlapWidth <= 0 || overlapHeight <= 0) return
    const touchesLeft = rect.left <= containerRect.left + 16
    const touchesRight = rect.right >= containerRect.right - 16
    const touchesTop = rect.top <= containerRect.top + 16
    const touchesBottom = rect.bottom >= containerRect.bottom - 16
    if (overlapWidth / width >= .75) {
      if (touchesBottom) bottom = Math.min(bottom, rect.top - containerRect.top)
      else if (touchesTop) top = Math.max(top, rect.bottom - containerRect.top)
    } else if (overlapHeight / height >= .75) {
      if (touchesLeft) left = Math.max(left, rect.right - containerRect.left)
      if (touchesRight) right = Math.min(right, rect.left - containerRect.left)
    }
  })
  if (right <= left || bottom <= top) return { left: 0, top: 0, right: width, bottom: height }
  return { left, top, right, bottom }
}

function visibleMapPoints(): any[] {
  if (!mapAPI) return []
  const provider = selectedMapProvider.value
  const points: any[] = []
  const pushPoint = (stop: Stop | SubStop) => {
    const point = pointForProvider(stop, provider)
    if (!point) return
    if (provider === 'amap') points.push([point.lng, point.lat])
    else if (typeof mapAPI.Point === 'function') points.push(new mapAPI.Point(point.lng, point.lat))
  }
  for (const stop of mapStops.value) pushPoint(stop)
  for (const child of selectedStop.value?.children || []) pushPoint(child)
  return points
}

function fitVisibleMapContent() {
  if (!mapInstance || !mapAPI) return
  const rect = mapVisibleRect()
  if (!rect) return
  const container = mapContainer.value
  const fullW = container?.clientWidth || 0
  const fullH = container?.clientHeight || 0
  const visibleW = rect.right - rect.left
  const visibleH = rect.bottom - rect.top
  const points = visibleMapPoints()
  if (!points.length) return
  const provider = selectedMapProvider.value
  const readPixel = (value: any): { x: number; y: number } | null => {
    try {
      if (provider === 'amap') {
        const pixel = mapInstance.lngLatToContainer?.(value)
        if (Array.isArray(pixel)) return { x: Number(pixel[0]), y: Number(pixel[1]) }
        return pixel && typeof pixel.x === 'number' ? { x: pixel.x, y: pixel.y } : null
      }
      const pixel = mapInstance.pointToPixel?.(value)
      return pixel && typeof pixel.x === 'number' ? { x: pixel.x, y: pixel.y } : null
    } catch { return null }
  }
  const measure = () => {
    let minX = Infinity; let minY = Infinity; let maxX = -Infinity; let maxY = -Infinity
    for (const point of points) {
      const pixel = readPixel(point)
      if (!pixel) continue
      minX = Math.min(minX, pixel.x); minY = Math.min(minY, pixel.y)
      maxX = Math.max(maxX, pixel.x); maxY = Math.max(maxY, pixel.y)
    }
    return Number.isFinite(minX) ? { minX, minY, maxX, maxY } : null
  }
  if (visibleW >= fullW - 2 && visibleH >= fullH - 2) {
    if (provider === 'amap') mapInstance.setFitView?.(points, false, [24, 24, 24, 24])
    else mapInstance.setViewport?.(points)
    return
  }
  const first = measure()
  if (!first) return
  const pad = 24
  const contentW = Math.max(1, first.maxX - first.minX)
  const contentH = Math.max(1, first.maxY - first.minY)
  const targetW = Math.max(1, visibleW - pad * 2)
  const targetH = Math.max(1, visibleH - pad * 2)
  const factor = Math.min(targetW / contentW, targetH / contentH)
  const currentZoom = mapInstance.getZoom?.() ?? 5
  const newZoom = Math.min(19, Math.max(3, Math.round(currentZoom + Math.log2(factor))))
  if (provider === 'amap') mapInstance.setZoom?.(newZoom, true)
  else mapInstance.setZoom?.(newZoom, { noAnimation: true })
  const second = measure()
  if (!second) return
  const centerX = (second.minX + second.maxX) / 2
  const centerY = (second.minY + second.maxY) / 2
  const visibleCenterX = (rect.left + rect.right) / 2
  const visibleCenterY = (rect.top + rect.bottom) / 2
  const newCenterX = fullW / 2 + centerX - visibleCenterX
  const newCenterY = fullH / 2 + centerY - visibleCenterY
  if (provider === 'amap') {
    const center = mapInstance.containerToLngLat?.([newCenterX, newCenterY])
    if (center) mapInstance.setCenter?.(center, true)
    return
  }
  const center = mapInstance.pixelToPoint?.(new mapAPI.Pixel(newCenterX, newCenterY))
  if (center) mapInstance.setCenter?.(center, { noAnimation: true })
}

function recenterMapToVisibleViewport() {
  if (!mapInstance || !mapAPI) return
  const viewport = mapFocusViewport()
  if (!viewport) return
  const offsetX = viewport.x - viewport.width / 2
  const offsetY = viewport.y - viewport.height / 2
  if (Math.abs(offsetX) < 1 && Math.abs(offsetY) < 1) return
  if (selectedMapProvider.value === 'amap') {
    if (typeof mapInstance.containerToLngLat !== 'function' || typeof mapInstance.setCenter !== 'function') return
    const shifted = mapInstance.containerToLngLat([viewport.width / 2 - offsetX, viewport.height / 2 - offsetY])
    if (shifted) mapInstance.setCenter(shifted, true)
    return
  }
  if (typeof mapAPI.Pixel !== 'function' || typeof mapInstance.pixelToPoint !== 'function' || typeof mapInstance.setCenter !== 'function') return
  const shiftedPixel = new mapAPI.Pixel(viewport.width / 2 - offsetX, viewport.height / 2 - offsetY)
  const center = mapInstance.pixelToPoint(shiftedPixel)
  if (center) mapInstance.setCenter(center, { noAnimation: true })
}

function handleViewportResize() {
  window.requestAnimationFrame(() => {
    mapInstance?.resize?.()
    if (selectedTarget.value) focusSelectedMapTarget()
    else fitVisibleMapContent()
  })
}

function navigateBackTo(fallback: NavigationURLState) {
  const state = window.history.state as JourneyHistoryState | null
  if (state?.journeyin && (state.journeyin.depth || 0) > 0) window.history.back()
  else void applyNavigationRoute(fallback).then(() => syncNavigationURL('replace'))
}

function navigateBackFromSubStop() {
  if (!selected.value) return
  navigateBackTo({ layer: 'stop', tripID: selected.value.id, stopID: selectedStopId.value, day: selectedDay.value, sheet: 'half' })
}

function closeDesktopStopDetail() {
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  detailMoreOpen.value = false
  descriptionEditing.value = false
  descriptionFullscreen.value = false
  descriptionDraft.value = ''
  descriptionEditorMode.value = 'edit'
  syncNavigationURL('replace')
  void renderMap()
}

function navigateBackFromStop() {
  if (!selected.value) return
  if (window.matchMedia('(min-width: 901px)').matches) {
    closeDesktopStopDetail()
    return
  }
  navigateBackTo({ layer: 'trip', tripID: selected.value.id, day: selectedDay.value, sheet: 'half' })
}

function navigateBackToList() {
  navigateBackTo({ layer: 'list' })
}

function selectJourneyDay(day: number | 'all') {
  selectedDay.value = day
  if (selectedStopId.value && !visibleStops.value.some(stop => stop.id === selectedStopId.value)) {
    selectedStopId.value = ''
    selectedSubStopId.value = ''
  }
  syncNavigationURL('replace')
  void renderMap()
}

function openJourneySearch(parentID = '') {
  if (readOnlyView.value) return
  locationSearchMode.value = 'add'
  locationSearchTargetID.value = ''
  locationSearchTargetDayID.value = ''
  locationSearchTitleDraft.value = ''
  searchParentStopId.value = parentID
  panelMode.value = 'search'
  panelOpen.value = true
  setSheetBreakpoint('expanded', 'replace')
  mobileMapToolsOpen.value = false
  searchMessage.value = parentID ? '为当前主规划点添加子规划点' : ''
}

function openPointSearch(target: Stop | SubStop) {
  if (readOnlyView.value || !tripDocument.value) return
  const day = dayForStop(target)
  if (!day) { error.value = '无法确定规划点所属日期'; return }
  locationSearchMode.value = 'repair'
  locationSearchTargetID.value = target.id
  locationSearchTargetDayID.value = day.id
  locationSearchTitleDraft.value = target.title
  searchParentStopId.value = ''
  searchQuery.value = target.title
  if (!searchRegion.value && target.address) searchRegion.value = target.address
  panelMode.value = 'search'
  panelOpen.value = true
  setSheetBreakpoint('expanded', 'replace')
  mobileMapToolsOpen.value = false
  searchMessage.value = '选择候选后会更新坐标；受影响的路线和天气会被清除。'
  error.value = ''
}

function closeJourneySearch() {
  panelMode.value = 'journey'
  searchParentStopId.value = ''
  locationSearchMode.value = 'add'
  locationSearchTargetID.value = ''
  locationSearchTargetDayID.value = ''
  locationSearchTitleDraft.value = ''
  searchMessage.value = ''
  searchResults.value = []
  selectedSearchResultIndex.value = -1
  clearSearchResultMarkers()
  setSheetBreakpoint('half', 'replace')
}

function focusTripListScroll() {
  if (!window.matchMedia('(max-width: 900px)').matches) return
  tripListScroll.value?.focus({ preventScroll: true })
}

function toggleTripMenu(tripID: string) {
  tripMenuID.value = tripMenuID.value === tripID ? '' : tripID
}

async function loadTrips() {
  loading.value = true
  error.value = ''
  try {
    const [tripResponse, capabilityResponse, settingsResponse] = await Promise.all([apiFetch('/api/v1/trips'), apiFetch('/api/v1/capabilities'), apiFetch('/api/v1/settings')])
    if (tripResponse.status === 401) return
    if (!tripResponse.ok) throw new Error('无法读取旅行规划')
    trips.value = ((await tripResponse.json()) as { items?: TripSummary[] }).items || []
    capabilities.value = capabilityResponse.ok ? await capabilityResponse.json() as Capabilities : null
    let configuredDefault = defaultMapProvider.value
    if (capabilities.value?.default_map_provider === 'baidu' || capabilities.value?.default_map_provider === 'amap') configuredDefault = capabilities.value.default_map_provider
    if (settingsResponse.ok) {
      const settings = await settingsResponse.json() as KeySettings
      settingsData.value = settings
      if (settings.map?.default_provider === 'baidu' || settings.map?.default_provider === 'amap') configuredDefault = settings.map.default_provider
      poiProviderPriority.value = settings.poi?.provider_priority === 'baidu' ? 'baidu' : 'amap'
      localDirectoryCount.value = settings.poi?.local_directory_count || 0
    }
    defaultMapProvider.value = configuredDefault
    if (!tripDocument.value?.map?.preferred_provider) {
      const changed = selectedMapProvider.value !== configuredDefault
      selectedMapProvider.value = configuredDefault
      planningProvider.value = configuredDefault
      if (changed) resetMapSDK()
    }
    const route = readNavigationURL()
    const nextTrip = route.tripID ? trips.value.find(trip => trip.id === route.tripID) : null
    if (nextTrip) await applyNavigationRoute(route)
    else {
      selected.value = null
      tripDocument.value = null
      tripView.value = 'list'
      selectedStopId.value = ''
      selectedSubStopId.value = ''
      panelMode.value = 'journey'
      resetMapSDK()
      syncNavigationURL('replace', { layer: 'list' })
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '网络请求失败'
  } finally {
    loading.value = false
  }
}

async function loadSharedTrip() {
  const bootstrap = (window as any).__JOURNEYIN_SHARE__ as SharedBootstrap | undefined
  if (!bootstrap?.trip) { error.value = '分享数据不可用或已过期'; return }
  const document = bootstrap.trip
  const stops = document.days.reduce((total, day) => total + (day.stops || []).length, 0)
  tripDocument.value = document
  selected.value = { id: document.id || 'shared', title: document.title, status: document.status || 'shared', start_date: document.days[0]?.date || '', end_date: document.days[document.days.length - 1]?.date || '', timezone: document.timezone, revision: bootstrap.revision || 1, days: document.days.length, stops }
  if (bootstrap.default_map_provider === 'baidu' || bootstrap.default_map_provider === 'amap') defaultMapProvider.value = bootstrap.default_map_provider
  capabilities.value = { version: APP_VERSION, default_map_provider: defaultMapProvider.value, map_providers: { baidu: { browser_key_configured: Boolean(bootstrap.browser_key), browser_key: bootstrap.browser_key || '' }, amap: { browser_key_configured: Boolean(bootstrap.amap_browser_key), browser_key: bootstrap.amap_browser_key || '', security_proxy_path: bootstrap.amap_security_proxy_path || '/_AMapService', security_js_code_configured: bootstrap.amap_security_js_code_configured } } }
  syncProviderFromDocument(document)
  selectedDay.value = 'all'; tripView.value = 'detail'; panelMode.value = 'journey'; panelOpen.value = true; panelCollapsed.value = false; mobileMapToolsOpen.value = false; selectedStopId.value = ''; selectedSubStopId.value = ''; reorderMode.value = false; descriptionEditing.value = false; tripDescriptionEditing.value = false
  await nextTick(); await renderMap()
}
function selectTrip(trip: TripSummary) { navigateToTrip(trip) }
async function deleteTrip(trip: TripSummary) {
  if (!window.confirm('确认删除“' + trip.title + '”吗？该行程及其规划点、路线和天气快照都会删除。')) return
  actionLoading.value = true; error.value = ''
  try { const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(trip.id), { method: 'DELETE', headers: { 'If-Match': 'revision-' + trip.revision } }); if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '删除行程失败') }; if (selected.value?.id === trip.id) { selected.value = null; tripDocument.value = null; tripView.value = 'list' }; await loadTrips() } catch (cause) { error.value = cause instanceof Error ? cause.message : '删除行程失败' } finally { actionLoading.value = false }
}
function deleteSelectedTrip() { if (selected.value) void deleteTrip(selected.value) }

async function loadDetail(trip: TripSummary) {
  historyOpen.value = false
  historyView.value = null
  selected.value = trip
  restoreShareState(trip.id)
  detailMoreOpen.value = false
  pointEditorOpen.value = false
  pointEditorTargetID.value = ''
  pointEditorDayID.value = ''
  pointUpdateNotice.value = ''
  cancelMapPick()
  selectedStopId.value = ''
  detailLoading.value = true
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(trip.id))
    if (response.status === 401) return
    if (!response.ok) throw new Error('无法读取行程详情')
    const payload = await response.json() as { document?: TripDocument }
    tripDocument.value = payload.document || null
    syncProviderFromDocument(tripDocument.value)
    selectedDay.value = 'all'
    await nextTick()
    void renderMap()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '行程详情读取失败'
    tripDocument.value = null
  } finally {
    detailLoading.value = false
  }
}

async function refreshTripHistoryList(tripID: string) {
  const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(tripID) + '/history?limit=100')
  const payload = await response.json() as { items?: TripHistoryEntry[]; error?: { message?: string } }
  if (response.status === 401) return
  if (!response.ok) throw new Error(payload.error?.message || '无法读取版本历史')
  historyEntries.value = payload.items || []
}
async function openTripHistory() {
  if (readOnlyView.value || !selected.value) return
  historyOpen.value = true
  historyLoading.value = true
  historyError.value = ''
  historyMessage.value = ''
  historyLabelDraft.value = ''
  try {
    await refreshTripHistoryList(selected.value.id)
  } catch (cause) {
    historyError.value = cause instanceof Error ? cause.message : '无法读取版本历史'
  } finally {
    historyLoading.value = false
  }
}
async function openTripHistoryFromList(trip: TripSummary) {
  tripMenuID.value = ''
  if (selected.value?.id === trip.id && tripDocument.value && !historyView.value) {
    await openTripHistory()
    return
  }
  selected.value = trip
  tripView.value = 'detail'
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  panelMode.value = 'journey'
  journeySection.value = 'itinerary'
  panelOpen.value = true
  mobileMapToolsOpen.value = false
  setSheetBreakpoint('half', 'replace', false)
  syncNavigationURL('push', { layer: 'trip', tripID: trip.id, day: 'all', sheet: 'half' })
  await loadDetail(trip)
  await openTripHistory()
}
async function saveTripHistory() {
  if (readOnlyView.value || !selected.value || historySaving.value) return
  const tripID = selected.value.id
  const revision = selected.value.revision
  historySaving.value = true
  historyError.value = ''
  historyMessage.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(tripID) + '/history', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision, 'Idempotency-Key': makeID('history-save') }, body: JSON.stringify({ label: historyLabelDraft.value.trim() }) })
    const payload = await response.json() as TripHistoryEntry & { already_saved?: boolean; idempotency_replay?: boolean; error?: { message?: string } }
    if (!response.ok) {
      if (response.status === 409) throw new Error(payload.error?.message || '行程已被其他操作更新，请重新加载后再保存历史版本')
      throw new Error(payload.error?.message || '保存历史版本失败')
    }
    await refreshTripHistoryList(tripID)
    historyMessage.value = payload.already_saved ? '当前版本已经保存过，已返回原历史版本。' : '当前版本已保存到历史。'
    historyLabelDraft.value = ''
  } catch (cause) {
    historyError.value = cause instanceof Error ? cause.message : '保存历史版本失败'
  } finally {
    historySaving.value = false
  }
}
async function viewTripHistory(entry: TripHistoryEntry) {
  if (readOnlyView.value || !selected.value) return
  historyLoading.value = true
  historyError.value = ''
  try {
    const historyID = entry.history_id || entry.id
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/history/' + encodeURIComponent(historyID))
    const payload = await response.json() as TripHistoryEntry & { document?: TripDocument; error?: { message?: string } }
    if (!response.ok || !payload.document) throw new Error(payload.error?.message || '无法读取历史版本')
    historyView.value = { ...entry, ...payload, id: payload.history_id || payload.id || historyID, history_id: payload.history_id || historyID }
    historyOpen.value = false
    selectedStopId.value = ''
    selectedSubStopId.value = ''
    selectedLegId.value = ''
    panelMode.value = 'journey'
    reorderMode.value = false
    descriptionEditing.value = false
    tripDescriptionEditing.value = false
    cancelEditStopDate()
    tripDocument.value = payload.document
    syncProviderFromDocument(tripDocument.value)
    selectedDay.value = 'all'
    await nextTick()
    await renderMap()
  } catch (cause) {
    historyError.value = cause instanceof Error ? cause.message : '无法读取历史版本'
  } finally {
    historyLoading.value = false
  }
}
async function exitTripHistory() {
  const trip = selected.value
  historyView.value = null
  historyOpen.value = false
  historyError.value = ''
  historyMessage.value = ''
  if (trip) await loadDetail(trip)
}
async function deleteTripHistory(entry: TripHistoryEntry) {
  if (!selected.value || historyDeletingID.value) return
  const historyID = entry.history_id || entry.id
  const label = entry.label || '保存于 ' + formatDateTime(entry.created_at)
  if (!window.confirm('确认删除“' + label + '”吗？删除后不可恢复，且不会影响当前行程。')) return
  historyDeletingID.value = historyID
  historyError.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/history/' + encodeURIComponent(historyID), { method: 'DELETE', headers: { 'Idempotency-Key': makeID('history-delete') } })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '删除历史版本失败') }
    historyEntries.value = historyEntries.value.filter(item => (item.history_id || item.id) !== historyID)
    historyMessage.value = '历史版本已删除。'
  } catch (cause) {
    historyError.value = cause instanceof Error ? cause.message : '删除历史版本失败'
  } finally {
    historyDeletingID.value = ''
  }
}

function makeID(prefix: string) {
  const uuid = crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2)
  return prefix + '_' + uuid.replaceAll('-', '')
}
function dateAfter(value: string, offset: number) {
  const date = new Date(value + 'T12:00:00')
  date.setDate(date.getDate() + offset)
  return date.toISOString().slice(0, 10)
}

async function createTrip() {
  if (!newTitle.value.trim()) { error.value = '请填写旅行规划名称'; return }
  const start = newStartDate.value
  const end = newEndDate.value < start ? start : newEndDate.value
  const days: Day[] = []
  const total = Math.min(60, Math.max(1, Math.floor((Date.parse(end) - Date.parse(start)) / 86400000) + 1))
  for (let index = 0; index < total; index++) days.push({ id: makeID('day'), date: dateAfter(start, index), title: '第 ' + (index + 1) + ' 天', notes_markdown: '', stops: [], legs: [] })
  actionLoading.value = true
  try {
    const response = await apiFetch('/api/v1/trips', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ schema_version: 1, title: newTitle.value.trim(), status: 'draft', locale: 'zh-CN', timezone: newTimezone.value, date_range: { start, end }, description_markdown: newDescription.value, links: [], map: { preferred_provider: defaultMapProvider.value, enabled_providers: ['baidu', 'amap'], default_mode: 'walking' }, days, metadata: { source: 'human' } }) })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '新建旅行规划失败') }
    newTripOpen.value = false
    await loadTrips()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '新建旅行规划失败' } finally { actionLoading.value = false }
}

function outOfChina(point: Coord) { return point.lng < 72.004 || point.lng > 137.8347 || point.lat < 0.8293 || point.lat > 55.8271 }
function transformLat(x: number, y: number) { let ret = -100 + 2 * x + 3 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x)); ret += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3; ret += (20 * Math.sin(y * Math.PI) + 40 * Math.sin(y / 3 * Math.PI)) * 2 / 3; ret += (160 * Math.sin(y / 12 * Math.PI) + 320 * Math.sin(y * Math.PI / 30)) * 2 / 3; return ret }
function transformLng(x: number, y: number) { let ret = 300 + x + 2 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x)); ret += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3; ret += (20 * Math.sin(x * Math.PI) + 40 * Math.sin(x / 3 * Math.PI)) * 2 / 3; ret += (150 * Math.sin(x / 12 * Math.PI) + 300 * Math.sin(x / 30 * Math.PI)) * 2 / 3; return ret }
function wgs84ToGcj02(point: Coord) {
  if (outOfChina(point)) return { ...point, crs: 'gcj02' }
  const a = 6378245; const ee = 0.00669342162296594323; const dLat = transformLat(point.lng - 105, point.lat - 35); const dLng = transformLng(point.lng - 105, point.lat - 35); const radLat = point.lat / 180 * Math.PI; let magic = Math.sin(radLat); magic = 1 - ee * magic * magic; const sqrtMagic = Math.sqrt(magic); return { lng: point.lng + dLng * 180 / (a / sqrtMagic * Math.cos(radLat) * Math.PI), lat: point.lat + dLat * 180 / ((a * (1 - ee)) / (magic * sqrtMagic) * Math.PI), crs: 'gcj02' }
}
function gcj02ToBd09(point: Coord) { const x = point.lng; const y = point.lat; const z = Math.sqrt(x * x + y * y) + 0.00002 * Math.sin(y * Math.PI * 3000 / 180); const theta = Math.atan2(y, x) + 0.000003 * Math.cos(x * Math.PI * 3000 / 180); return { lng: z * Math.cos(theta) + 0.0065, lat: z * Math.sin(theta) + 0.006, crs: 'bd09ll' } }
function bd09ToGcj02(point: Coord) { const x = point.lng - 0.0065; const y = point.lat - 0.006; const z = Math.sqrt(x * x + y * y) - 0.00002 * Math.sin(y * Math.PI * 3000 / 180); const theta = Math.atan2(y, x) - 0.000003 * Math.cos(x * Math.PI * 3000 / 180); return { lng: z * Math.cos(theta), lat: z * Math.sin(theta), crs: 'gcj02' } }
function normalizeCoordinateCRS(raw: string | undefined, fallback = '') { const value = String(raw || '').trim().toLowerCase().replaceAll('-', '').replaceAll('_', ''); if (value === 'wgs84' || value === 'wgs84ll' || value === 'gps') return 'wgs84'; if (value === 'gcj02' || value === 'gcj02ll' || value === 'amap' || value === 'autonavi') return 'gcj02'; if (value === 'bd09' || value === 'bd09ll' || value === 'baidu') return 'bd09ll'; return fallback }
function candidateCoordinateCRS(candidate: PlaceCandidate) { const raw = String(candidate.location?.crs || '').trim(); return raw ? normalizeCoordinateCRS(raw) : candidate.provider === 'amap' ? 'gcj02' : 'bd09ll' }
function savedLocationFor(candidate: PlaceCandidate): LocationData { const sourceCRS = candidateCoordinateCRS(candidate); const coordinates: Record<string, Coord & { crs?: string }> = { [sourceCRS]: { lat: candidate.location.lat, lng: candidate.location.lng, crs: sourceCRS } }; if (sourceCRS === 'gcj02') coordinates.bd09ll = gcj02ToBd09(candidate.location); const provider = candidate.provider === 'amap' ? 'amap' : 'baidu'; return { preferred: coordinates.bd09ll ? 'bd09ll' : sourceCRS, coordinates, source: provider + '-place-search', provider_refs: candidate.id ? { [provider + '_uid']: candidate.id } : {}, citycode: candidate.citycode, adcode: candidate.adcode, geocoded_at: new Date().toISOString(), precision: 'poi' } }
function locationForMapPoint(point: Coord & { crs: string }, provider: 'baidu' | 'amap'): LocationData { const crs = normalizeCoordinateCRS(point.crs, provider === 'amap' ? 'gcj02' : 'bd09ll'); return { preferred: crs, coordinates: { [crs]: { lat: point.lat, lng: point.lng, crs } }, source: provider + '-map-click', geocoded_at: new Date().toISOString(), precision: 'map-click' } }
function pointFor(stop: Stop | SubStop): (Coord & { crs: string }) | null { const coordinates = stop.location?.coordinates; if (!coordinates) return null; const preferredKey = stop.location?.preferred && coordinates[stop.location.preferred] ? stop.location.preferred : Object.keys(coordinates)[0]; const point = preferredKey ? coordinates[preferredKey] : null; const crs = normalizeCoordinateCRS(preferredKey); if (!point || !crs || !Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return null; return { ...point, crs } }
function locationStatus(stop: Stop | SubStop) { return pointFor(stop) ? '已定位' : '待定位' }
function locationSource(stop: Stop | SubStop) { return stop.location?.source?.trim() || '来源未记录' }
function pointForProvider(stop: Stop | SubStop, provider: 'baidu' | 'amap'): (Coord & { crs: string }) | null { const point = pointFor(stop); if (!point) return null; if (provider === 'amap') { if (point.crs === 'gcj02') return point; if (point.crs === 'bd09ll') return bd09ToGcj02(point); if (point.crs === 'wgs84') return wgs84ToGcj02(point) } else { if (point.crs === 'bd09ll') return point; if (point.crs === 'gcj02') return gcj02ToBd09(point); if (point.crs === 'wgs84') return gcj02ToBd09(wgs84ToGcj02(point)) } return null }
function navigationPointFor(stop: Stop | SubStop, provider: 'baidu' | 'amap'): (Coord & { crs: string }) | null { const coordinates = stop.location?.coordinates; if (!coordinates) return null; const order = provider === 'amap' ? ['gcj02', 'bd09ll', 'wgs84'] : ['bd09ll', 'gcj02', 'wgs84']; for (const crs of order) { const point = coordinates[crs]; if (point && Number.isFinite(point.lat) && Number.isFinite(point.lng)) return { ...point, crs } } return null }
function navigationPlatform(): 'android' | 'ios' | 'web' { const userAgent = navigator.userAgent; if (/Android/i.test(userAgent)) return 'android'; if (/iPhone|iPad|iPod/i.test(userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)) return 'ios'; return 'web' }
function reserveNavigationWindow(platform: 'android' | 'ios' | 'web') { if (platform !== 'web') return null; const opened = window.open('about:blank', '_blank'); if (opened) { try { opened.opener = null } catch { /* best effort */ } } return opened }
function openNavigationURL(url: string, platform: 'android' | 'ios' | 'web', reservedWindow: Window | null, fallbackURL = '') { if (platform === 'web' && reservedWindow && !reservedWindow.closed) { reservedWindow.location.replace(url); return } if (platform === 'web') { window.location.assign(url); return } let appOpened = false; const onPageHide = () => { appOpened = true }; const onVisibilityChange = () => { if (document.visibilityState === 'hidden') appOpened = true }; document.addEventListener('visibilitychange', onVisibilityChange); window.addEventListener('pagehide', onPageHide, { once: true }); window.location.assign(url); if (fallbackURL) window.setTimeout(() => { document.removeEventListener('visibilitychange', onVisibilityChange); window.removeEventListener('pagehide', onPageHide); if (!appOpened && document.visibilityState === 'visible') window.location.assign(fallbackURL) }, 1800) }
function mapPointFor(stop: Stop | SubStop): (Coord & { crs: string }) | null { return pointForProvider(stop, 'baidu') }
function routePoint(value: [number, number] | Coord, crs: string) { if (Array.isArray(value)) return { lng: value[0], lat: value[1], crs }; return { lng: value.lng, lat: value.lat, crs: (value as Coord & { crs?: string }).crs || crs } }
function mapRoutePointFor(value: [number, number] | Coord, crs: string, provider: 'baidu' | 'amap'): (Coord & { crs: string }) | null { const point = routePoint(value, crs); return pointForProvider({ location: { preferred: point.crs, coordinates: { [point.crs]: point } } } as unknown as Stop, provider) }
function mapRoutePoint(value: [number, number] | Coord, crs: string): (Coord & { crs: string }) | null { return mapRoutePointFor(value, crs, 'baidu') }
const SELECTED_STOP_ZOOM = 16
function defaultRouteStrategy(provider: 'baidu' | 'amap', mode: TravelMode) {
  return provider === 'amap' && mode === 'driving' ? planningStrategy.value : ''
}
function chooseSnapshotMetadata(leg: Leg, provider: 'baidu' | 'amap' = selectedMapProvider.value, mode: TravelMode = planningMode.value, strategy: string = defaultRouteStrategy(provider, mode)) {
  const candidates = (leg.snapshots || []).filter(snapshot => snapshot.provider === provider && (!snapshot.mode || snapshot.mode === mode))
  if (!strategy) return candidates[0] || null
  return candidates.find(snapshot => !snapshot.strategy || snapshot.strategy === strategy) || candidates[0] || null
}
function chooseSnapshot(leg: Leg, provider: 'baidu' | 'amap' = selectedMapProvider.value, mode: TravelMode = planningMode.value, strategy: string = defaultRouteStrategy(provider, mode)) { return chooseSnapshotMetadata(leg, provider, mode, strategy)?.geometry && chooseSnapshotMetadata(leg, provider, mode, strategy)!.geometry!.length > 1 ? chooseSnapshotMetadata(leg, provider, mode, strategy) : null }
function mapFocusViewport(): { width: number; height: number; x: number; y: number } | null {
  const container = mapContainer.value
  if (!container) return null
  const containerRect = container.getBoundingClientRect()
  const size = mapInstance?.getContainerSize?.()
  const width = Number(size?.width) || container.clientWidth || containerRect.width
  const height = Number(size?.height) || container.clientHeight || containerRect.height
  if (!(width > 0) || !(height > 0)) return null
  let left = 0; let top = 0; let right = width; let bottom = height
  document.querySelectorAll<HTMLElement>('.floating-panel, .details-drawer').forEach(overlay => {
    const rect = overlay.getBoundingClientRect()
    const overlapWidth = Math.min(containerRect.right, rect.right) - Math.max(containerRect.left, rect.left)
    const overlapHeight = Math.min(containerRect.bottom, rect.bottom) - Math.max(containerRect.top, rect.top)
    if (overlapWidth <= 0 || overlapHeight <= 0) return
    const touchesLeft = rect.left <= containerRect.left + 16
    const touchesRight = rect.right >= containerRect.right - 16
    const touchesTop = rect.top <= containerRect.top + 16
    const touchesBottom = rect.bottom >= containerRect.bottom - 16
    if (overlapWidth / width >= .75) {
      if (touchesBottom) bottom = Math.min(bottom, rect.top - containerRect.top)
      else if (touchesTop) top = Math.max(top, rect.bottom - containerRect.top)
    } else if (overlapHeight / height >= .75) {
      if (touchesLeft) left = Math.max(left, rect.right - containerRect.left)
      if (touchesRight) right = Math.min(right, rect.left - containerRect.left)
    }
  })
  if (right <= left || bottom <= top) return { width, height, x: width / 2, y: height / 2 }
  return { width, height, x: (left + right) / 2, y: (top + bottom) / 2 }
}
function focusMapOnPoint(stop: Stop | SubStop) {
  if (!mapInstance || !mapAPI || typeof mapAPI.Point !== 'function') return
  const point = mapPointFor(stop)
  if (!point || point.crs !== 'bd09ll') return
  const mapPoint = new mapAPI.Point(point.lng, point.lat)
  const focusVersion = ++mapFocusVersion
  mapInstance.resize?.()
  if (typeof mapInstance.centerAndZoom === 'function') mapInstance.centerAndZoom(mapPoint, SELECTED_STOP_ZOOM, { noAnimation: true })
  else { mapInstance.setCenter?.(mapPoint); mapInstance.setZoom?.(SELECTED_STOP_ZOOM, { zoomCenter: mapPoint }) }
  const alignVisibleCenter = () => {
    if (focusVersion !== mapFocusVersion || typeof mapAPI.Pixel !== 'function' || typeof mapInstance.pixelToPoint !== 'function' || typeof mapInstance.setCenter !== 'function') return
    const viewport = mapFocusViewport()
    if (!viewport) return
    const offsetX = viewport.x - viewport.width / 2
    const offsetY = viewport.y - viewport.height / 2
    if (Math.abs(offsetX) < 1 && Math.abs(offsetY) < 1) return
    const shiftedPixel = new mapAPI.Pixel(viewport.width / 2 - offsetX, viewport.height / 2 - offsetY)
    const adjustedCenter = mapInstance.pixelToPoint(shiftedPixel)
    if (adjustedCenter) mapInstance.setCenter(adjustedCenter, { noAnimation: true })
  }
  window.requestAnimationFrame(alignVisibleCenter)
}
function selectStop(stop: Stop) { navigateToStop(stop) }
function selectSubStop(child: SubStop, parent: Stop) { navigateToStop(child, parent) }
function openChildSearch(parent: Stop) {
  if (readOnlyView.value) return
  selectedStopId.value = parent.id; selectedSubStopId.value = ''; openJourneySearch(parent.id); searchMessage.value = '为“' + parent.title + '”添加子规划点' }

type PlanningPointPatch = { title?: string; address?: string; location?: LocationData }
type PlanningPointUpdatePayload = { document?: TripDocument; revision?: number; stops?: number; days?: number; updated_at?: string; changes?: { changed?: boolean; title_changed?: boolean; address_changed?: boolean; location_changed?: boolean; route_invalidated?: boolean; weather_cleared?: boolean }; error?: { message?: string } }

function pointEditorTarget() {
  return pointEditorTargetID.value ? findPlanningPoint(pointEditorTargetID.value) : selectedTarget.value
}

function pointEditorPoint() {
  const target = pointEditorTarget()
  return target ? pointFor(target) : null
}

function pointEditorLocationStatus() {
  const target = pointEditorTarget()
  return target ? locationStatus(target) : '待定位'
}

function pointEditorLocationSource() {
  const target = pointEditorTarget()
  return target ? locationSource(target) : '来源未记录'
}

function restoreSelectedPoint(pointID: string) {
  const point = findPlanningPoint(pointID)
  if (!point) return
  const parent = parentForStop(point)
  if (!parent) return
  selectedStopId.value = parent.id
  selectedSubStopId.value = point.id === parent.id ? '' : point.id
}

async function persistPlanningPointUpdate(target: Stop | SubStop, patch: PlanningPointPatch) {
  if (readOnlyView.value || !selected.value || !tripDocument.value) throw new Error('当前行程不可编辑')
  const day = dayForStop(target)
  if (!day) throw new Error('无法确定规划点所属日期')
  const body: Record<string, unknown> = {}
  if (patch.title !== undefined) body.title = patch.title
  if (patch.address !== undefined) body.address = patch.address
  if (patch.location !== undefined) body.location = patch.location
  const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(target.id), { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(body) })
  const payload = await response.json() as PlanningPointUpdatePayload
  if (!response.ok) {
    if (response.status === 409 && selected.value) {
      const pointID = target.id
      const reopenEditor = pointEditorOpen.value
      const titleDraft = pointEditorTitleDraft.value
      const addressDraft = pointEditorAddressDraft.value
      await loadDetail(selected.value)
      restoreSelectedPoint(pointID)
      if (reopenEditor) {
        pointEditorTargetID.value = pointID
        pointEditorTitleDraft.value = titleDraft
        pointEditorAddressDraft.value = addressDraft
        pointEditorOpen.value = true
      }
      throw new Error(payload.error?.message || '行程已被其他操作更新，请重新确认规划点修改')
    }
    throw new Error(payload.error?.message || '保存规划点信息失败')
  }
  applyTripPayload(payload)
  restoreSelectedPoint(target.id)
  if (payload.changes?.location_changed) {
    pointUpdateNotice.value = '位置已更新；受影响的路线和天气已清除，请重新生成路线或刷新天气。'
  } else if (payload.changes?.title_changed || payload.changes?.address_changed) {
    pointUpdateNotice.value = '规划点信息已更新。'
  }
  await nextTick()
  await renderMap()
  return payload
}

function beginEditPoint() {
  if (readOnlyView.value || !selectedTarget.value) return
  const target = selectedTarget.value
  pointEditorTargetID.value = target.id
  pointEditorDayID.value = dayForStop(target)?.id || ''
  pointEditorTitleDraft.value = target.title
  pointEditorAddressDraft.value = target.address || ''
  pointUpdateNotice.value = ''
  pointEditorOpen.value = true
  detailMoreOpen.value = false
  error.value = ''
  void nextTick(() => pointEditorTitleInput.value?.focus())
}

function cancelEditPoint() {
  if (pointEditorSaving.value) return
  pointEditorOpen.value = false
  pointEditorTargetID.value = ''
  pointEditorDayID.value = ''
  pointEditorTitleDraft.value = ''
  pointEditorAddressDraft.value = ''
}

function openPointSearchFromEditor() {
  const target = pointEditorTarget()
  if (!target) return
  pointEditorOpen.value = false
  openPointSearch(target)
}

function startMapPickFromEditor() {
  const target = pointEditorTarget()
  if (!target) return
  pointEditorOpen.value = false
  startMapPickForPoint(target)
}

function beginEditDescriptionFromPointEditor() {
  pointEditorOpen.value = false
  beginEditDescription()
}

async function savePointDetails() {
  const target = pointEditorTarget()
  if (!target || !selected.value || !tripDocument.value) return
  const title = pointEditorTitleDraft.value.trim()
  if (!title) { error.value = '请填写规划点名称'; return }
  const address = pointEditorAddressDraft.value.trim()
  if (title === target.title.trim() && address === (target.address || '').trim()) { cancelEditPoint(); return }
  pointEditorSaving.value = true
  error.value = ''
  try {
    await persistPlanningPointUpdate(target, { title, address })
    cancelEditPoint()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存规划点信息失败'
  } finally {
    pointEditorSaving.value = false
  }
}

function beginEditDescription() {
  if (readOnlyView.value) return
  const target = selectedTarget.value
  descriptionDraft.value = target?.description_markdown || ''
  arrivalTimeDraft.value = target?.time_window?.arrival || ''
  departureTimeDraft.value = target?.time_window?.departure || ''
  descriptionEditorMode.value = 'edit'
  descriptionEditing.value = true
}
function cancelEditDescription() {
  descriptionEditing.value = false
  descriptionDraft.value = ''
  arrivalTimeDraft.value = ''
  departureTimeDraft.value = ''
  descriptionEditorMode.value = 'edit'
  descriptionFullscreen.value = false
}
function openDescriptionFullscreen() { descriptionFullscreen.value = true }
function closeDescriptionFullscreen() { descriptionFullscreen.value = false }
function beginEditTripDescription() {
  if (readOnlyView.value) return
  tripDescriptionDraft.value = tripDocument.value?.description_markdown || ''; tripDescriptionEditorMode.value = 'edit'; tripDescriptionEditing.value = true }
function openTripDescriptionFullscreen() { tripDescriptionFullscreen.value = true }
function closeTripDescriptionFullscreen() { tripDescriptionFullscreen.value = false }
function cancelEditTripDescription() { tripDescriptionEditing.value = false; tripDescriptionDraft.value = ''; tripDescriptionEditorMode.value = 'edit'; tripDescriptionFullscreen.value = false }
function beginEditTripDetails() {
  if (readOnlyView.value || !selected.value || !tripDocument.value) return
  const range = tripDateRangeFor(tripDocument.value, selected.value)
  tripDetailsTitleDraft.value = tripDocument.value.title || selected.value.title
  tripDetailsStartDateDraft.value = range.start
  tripDetailsEndDateDraft.value = range.end
  tripDetailsIdempotencyKey.value = makeID('trip-details')
  tripDetailsNotice.value = ''
  tripDetailsEditing.value = true
  error.value = ''
  void nextTick(() => tripDetailsTitleInput.value?.focus())
}
function cancelEditTripDetails() {
  if (tripDetailsSaving.value) return
  tripDetailsEditing.value = false
  tripDetailsTitleDraft.value = ''
  tripDetailsStartDateDraft.value = ''
  tripDetailsEndDateDraft.value = ''
  tripDetailsIdempotencyKey.value = ''
}
function tripDetailsConflictMessage(days: Array<{ day_id?: string; date?: string; stop_count?: number }>) {
  if (!days.length) return '不能移除仍包含规划点的日期，请先移动规划点或恢复结束日期。'
  const detail = days.map(day => {
    const count = day.stop_count || 0
    return (day.date ? formatDate(day.date) : day.day_id || '目标日期') + '（' + count + ' 个规划点）'
  }).join('、')
  return '不能缩短日期范围：' + detail + '仍有规划点，请先移动规划点或恢复结束日期。'
}
async function editTripDetailsFromList(trip: TripSummary) {
  tripMenuID.value = ''
  if (selected.value?.id === trip.id && tripDocument.value) {
    beginEditTripDetails()
    return
  }
  selected.value = trip
  tripView.value = 'detail'
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  panelMode.value = 'journey'
  journeySection.value = 'itinerary'
  panelOpen.value = true
  mobileMapToolsOpen.value = false
  setSheetBreakpoint('half', 'replace', false)
  syncNavigationURL('push', { layer: 'trip', tripID: trip.id, day: 'all', sheet: 'half' })
  await loadDetail(trip)
  if (selected.value?.id === trip.id && tripDocument.value) beginEditTripDetails()
}
async function saveTripDetails() {
  if (readOnlyView.value || !selected.value || !tripDocument.value || tripDetailsSaving.value) return
  const title = tripDetailsTitleDraft.value.trim()
  if (!title) { error.value = '请填写行程名称'; return }
  if (tripDetailsDateError.value) { error.value = tripDetailsDateError.value; return }
  if (tripDetailsBlockingDays.value.length) { error.value = tripDetailsConflictMessage(tripDetailsBlockingDays.value.map(day => ({ day_id: day.id, date: day.date, stop_count: planningPointCount(day) }))); return }
  const previousDayID = selectedDay.value !== 'all' ? tripDocument.value.days[selectedDay.value - 1]?.id || '' : ''
  const tripID = selected.value.id
  const revision = selected.value.revision
  tripDetailsSaving.value = true
  error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(tripID), { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision, 'Idempotency-Key': tripDetailsIdempotencyKey.value || makeID('trip-details') }, body: JSON.stringify({ title, date_range: { start: tripDetailsStartDateDraft.value, end: tripDetailsEndDateDraft.value } }) })
    const payload = await response.json() as { document?: TripDocument; title?: string; start_date?: string; end_date?: string; revision?: number; stops?: number; days?: number; updated_at?: string; changes?: { added_days?: number; removed_days?: number; cleared_weather_stops?: number }; error?: { code?: string; message?: string; details?: { days?: Array<{ day_id?: string; date?: string; stop_count?: number }> } } }
    if (!response.ok) {
      if (response.status === 409 && payload.error?.code === 'date_range_conflict') throw new Error(tripDetailsConflictMessage(payload.error.details?.days || []))
      if (response.status === 409) {
        const current = selected.value
        if (current) await loadDetail(current)
        throw new Error('行程已被其他操作更新，请重新编辑后再保存')
      }
      throw new Error(payload.error?.message || '保存行程信息失败')
    }
    applyTripPayload(payload)
    if (previousDayID && tripDocument.value) {
      const dayIndex = tripDocument.value.days.findIndex(day => day.id === previousDayID)
      selectedDay.value = dayIndex >= 0 ? dayIndex + 1 : 'all'
    } else if (selectedDay.value !== 'all' && selectedDay.value > (tripDocument.value?.days.length || 0)) {
      selectedDay.value = 'all'
    }
    const changes = payload.changes || {}
    const notices: string[] = []
    if ((changes.added_days || 0) > 0) notices.push('已新增 ' + changes.added_days + ' 个空白日')
    if ((changes.removed_days || 0) > 0) notices.push('已移除 ' + changes.removed_days + ' 个空白日')
    if ((changes.cleared_weather_stops || 0) > 0) notices.push('日期变化，已清除 ' + changes.cleared_weather_stops + ' 个天气快照')
    tripDetailsNotice.value = notices.join('；')
    tripDetailsSaving.value = false
    cancelEditTripDetails()
    syncNavigationURL('replace')
    await renderMap()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存行程信息失败'
  } finally {
    tripDetailsSaving.value = false
  }
}
async function saveTripDescription() {
  if (readOnlyView.value || !selected.value || !tripDocument.value) return
  const previous = tripDocument.value.description_markdown || ''; tripDocument.value.description_markdown = tripDescriptionDraft.value.trim(); tripDescriptionSaving.value = true; error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(tripDocument.value) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存行程说明失败')
    applyTripPayload(payload); tripDescriptionEditing.value = false; tripDescriptionDraft.value = ''; tripDescriptionEditorMode.value = 'edit'
  } catch (cause) { if (tripDocument.value) tripDocument.value.description_markdown = previous; error.value = cause instanceof Error ? cause.message : '保存行程说明失败' } finally { tripDescriptionSaving.value = false }
}
async function saveDescription() {
  if (readOnlyView.value || !selected.value || !tripDocument.value || !selectedTarget.value) return
  const target = selectedTarget.value
  const previousDescription = target.description_markdown || ''
  const previousTimeWindow = target.time_window ? { ...target.time_window } : undefined
  const arrival = arrivalTimeDraft.value.trim()
  const departure = departureTimeDraft.value.trim()
  if (arrival && departure && arrival > departure) {
    error.value = '到达时间不能晚于离开时间'
    return
  }
  target.description_markdown = descriptionDraft.value.trim()
  target.time_window = arrival || departure ? { ...(arrival ? { arrival } : {}), ...(departure ? { departure } : {}) } : undefined
  descriptionSaving.value = true
  error.value = ''
  const parentID = selectedStop.value?.id || ''
  const childID = selectedSubStop.value?.id || ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(tripDocument.value) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; updated_at?: string; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存规划点信息失败')
    applyTripPayload(payload)
    selectedStopId.value = parentID
    selectedSubStopId.value = childID
    cancelEditDescription()
  } catch (cause) {
    target.description_markdown = previousDescription
    target.time_window = previousTimeWindow
    error.value = cause instanceof Error ? cause.message : '保存规划点信息失败'
  } finally { descriptionSaving.value = false }
}


async function renderBaiduMap() {
  if (!baiduKey.value || !mapContainer.value || !tripDocument.value) return
  const renderVersion = ++mapRenderVersion
  ++mapFocusVersion
  try {
    await loadBaiduMap()
    if (renderVersion !== mapRenderVersion) return
    if (!mapAPI || typeof mapAPI.Map !== 'function' || !mapContainer.value) throw new Error('百度 JSAPI 未提供可用的 Map 构造器；请检查浏览器端 AK、服务权限、域名白名单和当前浏览器环境')
    if (!mapInstance) {
      mapReady.value = false
      mapInstance = new mapAPI.Map(mapContainer.value, { enableIconClick: false, fixCenterWhenResize: true })
      mapInstance.enableScrollWheelZoom()
      mapInstance.addEventListener?.('click', handleMapClick)
      mapInstance.addEventListener?.('tilesloaded', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
      if (mapReadyTimer !== null) window.clearTimeout(mapReadyTimer)
      mapReadyTimer = window.setTimeout(() => {
        if (!mapReady.value) mapWarning.value = '百度地图底图加载较慢；请检查浏览器端 AK、' + window.location.hostname + ' 域名白名单和网络连接。地图仍可继续尝试加载。'
      }, 8000)
    }
    mapInstance.clearOverlays()
    const points: any[] = []
    for (const stop of mapStops.value) {
      const point = mapPointFor(stop)
      if (!point || point.crs !== 'bd09ll') continue
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      points.push(mapPoint)
      const marker = new mapAPI.Marker(mapPoint)
      const carryOver = carryOverStop.value?.id === stop.id && selectedDay.value !== 'all'
      marker.__journeyinStopId = stop.id
      marker.__journeyinCarryOver = carryOver
      marker.addEventListener?.('click', () => {
        if (mapPickMode.value) { handleMapClick({ point: mapPoint }); return }
        if (carryOver) {
          const carryOverDayIndex = tripDocument.value?.days.findIndex(day => day.stops.some(item => item.id === stop.id)) ?? -1
          if (carryOverDayIndex >= 0) { selectedDay.value = carryOverDayIndex + 1; selectStop(stop) }
          return
        }
        selectStop(stop)
      })
      attachMapLabel(marker, carryOver ? '前日终点 · ' + stop.title : stop.title, stopDate(stop))
      mapInstance.addOverlay(marker)
    }
    if (selectedStop.value?.children?.length) for (const child of selectedStop.value.children) {
      const point = mapPointFor(child)
      if (!point || point.crs !== 'bd09ll') continue
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      const marker = new mapAPI.Marker(mapPoint)
      marker.__journeyinSubStopId = child.id
      marker.addEventListener?.('click', () => { if (mapPickMode.value) { handleMapClick({ point: mapPoint }); return }; selectSubStop(child, selectedStop.value!) })
      attachMapLabel(marker, child.title, stopDate(child))
      mapInstance.addOverlay(marker)
    }
    for (const day of visibleDays.value) for (const leg of day.legs || []) {
      const snapshot = chooseSnapshot(leg, 'baidu', planningMode.value)
      if (!snapshot || !snapshot.geometry) continue
      const line = snapshot.geometry.map(value => mapRoutePoint(value, snapshot.coordinate_system || 'bd09ll')).filter((point): point is Coord & { crs: string } => Boolean(point)).map(point => new mapAPI.Point(point.lng, point.lat))
      if (line.length > 1) {
        const polyline = new mapAPI.Polyline(line, { strokeColor: '#24695c', strokeWeight: 5, strokeOpacity: .82 })
        polyline.__journeyinLegId = leg.id
        polyline.addEventListener?.('click', () => { selectedLegId.value = leg.id })
        mapInstance.addOverlay(polyline)
        attachRouteLabel(snapshot)
      }
    }
    const focusTarget = selectedTarget.value
    if (focusTarget) {
      await nextTick()
      if (renderVersion !== mapRenderVersion) return
      focusMapOnPoint(focusTarget)
    } else if (points.length) fitVisibleMapContent()
    else mapInstance.centerAndZoom('中国', 5)
    applyMapType()
    mapError.value = ''
    renderSearchResultMarkers()
  } catch (cause) { mapReady.value = false; mapWarning.value = ''; mapError.value = safeMapError(cause, '地图初始化失败') }
}
function resetMapSDK() {
  try { mapInstance?.destroy?.() } catch { /* SDK cleanup is best effort */ }
  mapInstance = null
  mapAPI = null
  mapOverlays = []
  amapSatelliteLayer = null
  mapReady.value = false
  mapWarning.value = ''
  if (mapReadyTimer !== null) { window.clearTimeout(mapReadyTimer); mapReadyTimer = null }
  try { BMapLoader.reset() } catch { /* loader reset is best effort */ }
  mapScriptPromise = null
  amapScriptPromise = null
  loadedMapKey = ''
  loadedAMapKey = ''
}
async function loadBaiduMap() {
  const currentKey = baiduKey.value.trim()
  if (!currentKey) return
  if (loadedMapKey && loadedMapKey !== currentKey) resetMapSDK()
  if (mapAPI && typeof mapAPI.Map === 'function') return
  if (!mapScriptPromise) {
    mapScriptPromise = BMapLoader.load({ ak: currentKey, version: '4.0', timeout: 8000 }).then(namespace => {
      mapAPI = namespace
      loadedMapKey = currentKey
    })
  }
  try { await mapScriptPromise } catch (cause) { resetMapSDK(); throw cause }
}
async function loadAMap() {
  const currentKey = amapKey.value.trim()
  if (!currentKey) return
  if (loadedAMapKey && loadedAMapKey !== currentKey) resetMapSDK()
  if (mapAPI && typeof mapAPI.Map === 'function' && loadedAMapKey === currentKey) return
  if (!amapScriptPromise) {
    const proxyPath = capabilities.value?.map_providers?.amap?.security_proxy_path || '/_AMapService'
    if (capabilities.value?.map_providers?.amap?.security_js_code_configured !== false && proxyPath) {
      window._AMapSecurityConfig = { serviceHost: new URL(proxyPath, window.location.origin).toString().replace(/\/$/, '') }
    } else {
      delete (window as any)._AMapSecurityConfig
    }
    amapScriptPromise = AMapLoader.load({ key: currentKey, version: '2.0', plugins: ['AMap.Scale'] }).then(namespace => {
      mapAPI = namespace
      loadedAMapKey = currentKey
    })
  }
  try { await amapScriptPromise } catch (cause) { resetMapSDK(); throw cause }
}
function safeMapError(cause: unknown, fallback: string) { const message = cause instanceof Error ? cause.message : String(cause || ''); return (message || fallback).replace(/([?&](?:ak|key|jscode)=)[^&\s'\"]+/gi, '$1<redacted>') }
function amapPointToArray(point: Coord) { return [point.lng, point.lat] }
function addAMapOverlay(overlay: any) { mapInstance?.add?.(overlay); mapOverlays.push(overlay); return overlay }
function clearAMapOverlays() { mapInstance?.clearMap?.(); mapOverlays = [] }
function attachAMapLabel(marker: any, title: string, date: string) {
  if (!showMapLabels.value || typeof marker.setLabel !== 'function') return
  const content = '<span style="display:inline-block;padding:5px 9px;border:1px solid #0b2f35;border-radius:8px;color:#ffffff;background:#173f47ee;box-shadow:0 3px 10px #0006;font-size:12px;font-weight:700;line-height:16px;white-space:nowrap;text-shadow:0 1px 2px #0008;">' + escapeHTML(title + ' · ' + date) + '</span>'
  marker.setLabel({ content, direction: 'right', offset: new mapAPI.Pixel(12, -6) })
}
function attachAMapRouteLabel(snapshot: { geometry?: Array<[number, number]> | Array<Coord>; coordinate_system?: string; distance_m?: number; duration_s?: number }) {
  if (!showMapLabels.value || !snapshot.geometry?.length || typeof mapAPI?.Text !== 'function') return
  const text = [formatDistance(snapshot.distance_m), formatDuration(snapshot.duration_s)].filter(Boolean).join(' · ')
  if (!text) return
  const middle = mapRoutePointFor(snapshot.geometry[Math.floor(snapshot.geometry.length / 2)], snapshot.coordinate_system || 'gcj02', 'amap')
  if (!middle) return
  addAMapOverlay(new mapAPI.Text({ text, position: amapPointToArray(middle), style: { backgroundColor: '#24695cdd', color: '#ffffff', border: '0', borderRadius: '999px', padding: '4px 8px', fontSize: '12px', lineHeight: '16px', whiteSpace: 'nowrap', boxShadow: '0 3px 10px #0003' } }))
}
function focusAMapPoint(stop: Stop | SubStop) {
  if (!mapInstance || selectedMapProvider.value !== 'amap') return
  const point = pointForProvider(stop, 'amap')
  if (!point) return
  mapInstance.resize?.()
  mapInstance.setCenter?.(amapPointToArray(point), true)
  mapInstance.setZoom?.(SELECTED_STOP_ZOOM, true)
}

function searchResultMapPoint(result: PlaceCandidate, provider: 'baidu' | 'amap'): (Coord & { crs: string }) | null {
  const location = result.location
  if (!location || !Number.isFinite(location.lat) || !Number.isFinite(location.lng)) return null
  const crs = candidateCoordinateCRS(result)
  if (!crs) return null
  return mapRoutePointFor({ lat: location.lat, lng: location.lng } as Coord, crs, provider)
}

function searchResultBaiduIcon(selected: boolean, label: string): any {
  if (typeof mapAPI?.Icon !== 'function' || typeof mapAPI?.Size !== 'function') return undefined
  const color = selected ? '#e56a4d' : '#0e7490'
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="26" height="26" viewBox="0 0 26 26"><circle cx="13" cy="13" r="11" fill="' + color + '" stroke="#ffffff" stroke-width="2"/><text x="13" y="16.5" text-anchor="middle" font-size="11" font-weight="700" fill="#ffffff" font-family="sans-serif">' + label + '</text></svg>'
  return new mapAPI.Icon('data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg), new mapAPI.Size(26, 26))
}

function clearSearchResultMarkers() {
  for (const overlay of searchResultMarkers) {
    try {
      if (selectedMapProvider.value === 'amap') mapInstance?.remove?.(overlay)
      else mapInstance?.removeOverlay?.(overlay)
    } catch { /* best effort */ }
  }
  searchResultMarkers.length = 0
}

function focusMapOnSearchResult(result: PlaceCandidate) {
  if (!mapInstance || !mapAPI) return
  const point = searchResultMapPoint(result, selectedMapProvider.value)
  if (!point) return
  mapInstance.resize?.()
  if (selectedMapProvider.value === 'amap') {
    mapInstance.setCenter?.(amapPointToArray(point), true)
    mapInstance.setZoom?.(SELECTED_STOP_ZOOM, true)
  } else {
    const mapPoint = new mapAPI.Point(point.lng, point.lat)
    if (typeof mapInstance.centerAndZoom === 'function') mapInstance.centerAndZoom(mapPoint, SELECTED_STOP_ZOOM, { noAnimation: true })
    else { mapInstance.setCenter?.(mapPoint); mapInstance.setZoom?.(SELECTED_STOP_ZOOM, { zoomCenter: mapPoint }) }
  }
  window.requestAnimationFrame(() => recenterMapToVisibleViewport())
}

function renderSearchResultMarkers() {
  if (!mapInstance || !mapAPI || panelMode.value !== 'search' || !searchResults.value.length) { clearSearchResultMarkers(); return }
  clearSearchResultMarkers()
  searchResults.value.forEach((result, index) => {
    const point = searchResultMapPoint(result, selectedMapProvider.value)
    if (!point) return
    const selected = index === selectedSearchResultIndex.value
    const label = String(index + 1)
    let marker: any = null
    if (selectedMapProvider.value === 'amap') {
      marker = new mapAPI.Marker({ position: amapPointToArray(point), content: '<div class="search-result-pin' + (selected ? ' selected' : '') + '"><span>' + label + '</span></div>', offset: new mapAPI.Pixel(-13, -13), zIndex: 120 })
      marker.on?.('click', () => selectSearchResult(index))
      mapInstance.add?.(marker)
    } else {
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      const icon = searchResultBaiduIcon(selected, label)
      marker = new mapAPI.Marker(mapPoint, icon ? { icon } : undefined)
      marker.addEventListener?.('click', () => selectSearchResult(index))
      mapInstance.addOverlay(marker)
    }
    searchResultMarkers.push(marker)
  })
}

function selectSearchResult(index: number) {
  const result = searchResults.value[index]
  if (!result) return
  selectedSearchResultIndex.value = index
  focusMapOnSearchResult(result)
  renderSearchResultMarkers()
}

async function renderAMapMap() {
  if (!amapKey.value || !mapContainer.value || !tripDocument.value) return
  const renderVersion = ++mapRenderVersion
  ++mapFocusVersion
  try {
    await loadAMap()
    if (renderVersion !== mapRenderVersion) return
    if (!mapAPI || typeof mapAPI.Map !== 'function' || !mapContainer.value) throw new Error('高德 JS API 2.0 未提供可用的 Map 构造器；请检查 JS Key、安全密钥、域名白名单和网络连接')
    if (!mapInstance || loadedMapKey) {
      if (mapInstance) { try { mapInstance.destroy?.() } catch { /* best effort */ } }
      mapReady.value = false
      const first = mapStops.value.map(stop => pointForProvider(stop, 'amap')).find(Boolean)
      mapInstance = new mapAPI.Map(mapContainer.value, { viewMode: '2D', zoom: first ? 12 : 5, center: first ? amapPointToArray(first) : [116.397428, 39.90923], resizeEnable: true })
      mapInstance.on?.('click', (event: any) => {
        const lnglat = event?.lnglat
        const lng = Number(lnglat?.lng ?? lnglat?.getLng?.())
        const lat = Number(lnglat?.lat ?? lnglat?.getLat?.())
        if (Number.isFinite(lng) && Number.isFinite(lat)) handleMapClick({ point: { lng, lat, crs: 'gcj02' } })
      })
      mapInstance.on?.('complete', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
      if (mapReadyTimer !== null) window.clearTimeout(mapReadyTimer)
      mapReadyTimer = window.setTimeout(() => { if (!mapReady.value) mapWarning.value = '高德地图底图加载较慢；请检查 JS Key、安全密钥、域名白名单和网络连接。地图仍可继续尝试加载。' }, 8000)
      loadedMapKey = ''
    }
    clearAMapOverlays()
    const points: any[] = []
    for (const stop of mapStops.value) {
      const point = pointForProvider(stop, 'amap')
      if (!point) continue
      const mapPoint = amapPointToArray(point)
      points.push(mapPoint)
      const marker = addAMapOverlay(new mapAPI.Marker({ position: mapPoint, title: stop.title, anchor: 'bottom-center' }))
      const carryOver = carryOverStop.value?.id === stop.id && selectedDay.value !== 'all'
      marker.__journeyinStopId = stop.id
      marker.__journeyinCarryOver = carryOver
      marker.on?.('click', () => {
        if (mapPickMode.value) { handleMapClick({ point: { lng: point.lng, lat: point.lat, crs: 'gcj02' } }); return }
        if (carryOver) {
          const carryOverDayIndex = tripDocument.value?.days.findIndex(day => day.stops.some(item => item.id === stop.id)) ?? -1
          if (carryOverDayIndex >= 0) { selectedDay.value = carryOverDayIndex + 1; selectStop(stop) }
          return
        }
        selectStop(stop)
      })
      attachAMapLabel(marker, carryOver ? '前日终点 · ' + stop.title : stop.title, stopDate(stop))
    }
    if (selectedStop.value?.children?.length) for (const child of selectedStop.value.children) {
      const point = pointForProvider(child, 'amap')
      if (!point) continue
      const mapPoint = amapPointToArray(point)
      const marker = addAMapOverlay(new mapAPI.Marker({ position: mapPoint, title: child.title, anchor: 'bottom-center' }))
      marker.__journeyinSubStopId = child.id
      marker.on?.('click', () => { if (mapPickMode.value) { handleMapClick({ point: { lng: point.lng, lat: point.lat, crs: 'gcj02' } }); return }; selectSubStop(child, selectedStop.value!) })
      attachAMapLabel(marker, child.title, stopDate(child))
    }
    for (const day of visibleDays.value) for (const leg of day.legs || []) {
      const snapshot = chooseSnapshot(leg, 'amap', planningMode.value)
      if (!snapshot?.geometry?.length) continue
      const line = snapshot.geometry.map(value => mapRoutePointFor(value, snapshot.coordinate_system || 'gcj02', 'amap')).filter((point): point is Coord & { crs: string } => Boolean(point)).map(point => amapPointToArray(point))
      if (line.length < 2) continue
      const polyline = addAMapOverlay(new mapAPI.Polyline({ path: line, strokeColor: '#24695c', strokeWeight: 5, strokeOpacity: .82, lineJoin: 'round', showDir: true, zIndex: 50 }))
      polyline.__journeyinLegId = leg.id
      polyline.on?.('click', () => { selectedLegId.value = leg.id })
      attachAMapRouteLabel(snapshot)
    }
    const focusTarget = selectedTarget.value
    if (focusTarget) { await nextTick(); if (renderVersion !== mapRenderVersion) return; focusAMapPoint(focusTarget) }
    else if (points.length) fitVisibleMapContent()
    else mapInstance.setCenter?.([116.397428, 39.90923])
    applyMapType()
    mapError.value = ''
    renderSearchResultMarkers()
  } catch (cause) {
    mapReady.value = false
    mapWarning.value = ''
    mapError.value = safeMapError(cause, '高德地图初始化失败')
  }
}
async function renderMap() {
  if (!mapContainer.value || !tripDocument.value) return
  if (!key.value) { resetMapSDK(); return }
  if (selectedMapProvider.value === 'amap') return renderAMapMap()
  return renderBaiduMap()
}
function setMapProvider(provider: 'baidu' | 'amap') {
  if (provider !== 'baidu' && provider !== 'amap') return
  if (selectedMapProvider.value === provider) return
  selectedMapProvider.value = provider
  planningProvider.value = provider
  localStorage.setItem('journeyin.mapProvider', provider)
  localStorage.setItem('journeyin.planningProvider', provider)
  resetMapSDK()
  mapError.value = ''
  mapWarning.value = ''
  void nextTick().then(() => renderMap())
}
function retryMap() { mapError.value = ''; mapWarning.value = ''; resetMapSDK(); void nextTick().then(() => renderMap()) }

function togglePanel() {
  panelOpen.value = !panelOpen.value
  mobileMapToolsOpen.value = false
  localStorage.setItem('journeyin.panelOpen', String(panelOpen.value))
}
function togglePanelCollapsed() {
  setSheetBreakpoint(sheetBreakpoint.value === 'peek' ? 'half' : 'peek')
  localStorage.setItem('journeyin.panelCollapsed', String(sheetBreakpoint.value === 'peek'))
}
function toggleMobileMapTools() { mobileMapToolsOpen.value = !mobileMapToolsOpen.value }
function toggleDetailCollapsed() {
  setSheetBreakpoint(sheetBreakpoint.value === 'peek' ? 'expanded' : 'peek')
  localStorage.setItem('journeyin.detailCollapsed', String(sheetBreakpoint.value === 'peek'))
}
function applyMapType() {
  if (!mapInstance || !mapAPI) return
  if (selectedMapProvider.value === 'amap') {
    if (typeof mapAPI.TileLayer?.Satellite !== 'function') return
    if (!amapSatelliteLayer) amapSatelliteLayer = new mapAPI.TileLayer.Satellite({ zIndex: 10 })
    if (mapType.value === 'satellite') mapInstance.add?.(amapSatelliteLayer)
    else mapInstance.remove?.(amapSatelliteLayer)
    return
  }
  if (typeof mapInstance.setMapType !== 'function') return
  const type = mapType.value === 'satellite' ? mapAPI.BMAP_SATELLITE_MAP || (window as any).BMAP_SATELLITE_MAP : mapAPI.BMAP_NORMAL_MAP || (window as any).BMAP_NORMAL_MAP
  if (type) mapInstance.setMapType(type)
}
function setMapType(type: 'normal' | 'satellite') {
  if (mapType.value === type) return
  mapType.value = type
  localStorage.setItem('journeyin.mapType', mapType.value)
  applyMapType()
}
function toggleMapLabels() { showMapLabels.value = !showMapLabels.value; localStorage.setItem('journeyin.mapLabels', String(showMapLabels.value)); void renderMap() }
function toggleMapPick() {
  if (readOnlyView.value) return
  if (!mapReady.value || !tripDocument.value) { error.value = '地图加载完成后才能使用地图选点'; return }
  mapPickTargetID.value = ''
  mapPickMode.value = !mapPickMode.value
  if (!mapPickMode.value) mapPickLocation.value = null
  error.value = ''
}

function startMapPickForPoint(target: Stop | SubStop) {
  if (readOnlyView.value) return
  if (!mapReady.value || !tripDocument.value) { error.value = '地图加载完成后才能使用地图选点'; return }
  const day = dayForStop(target)
  if (!day) { error.value = '无法确定规划点所属日期'; return }
  mapPickTargetID.value = target.id
  mapPickDayID.value = day.id
  mapPickTitle.value = target.title
  mapPickAddress.value = target.address || ''
  mapPickLocation.value = null
  mapPickOpen.value = false
  mapPickMode.value = true
  pointEditorOpen.value = false
  error.value = ''
  if (window.matchMedia('(max-width: 900px)').matches) setSheetBreakpoint('peek', 'replace')
}

function handleMapClick(event: any) {
  if (readOnlyView.value || !mapPickMode.value || !event?.point || !tripDocument.value) return
  const point = { lat: Number(event.point.lat), lng: Number(event.point.lng), crs: selectedMapProvider.value === 'amap' ? 'gcj02' : 'bd09ll' }
  if (!Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return
  const target = mapPickTargetID.value ? findPlanningPoint(mapPickTargetID.value) : null
  mapPickLocation.value = point
  mapPickTitle.value = target?.title || ''
  mapPickAddress.value = target?.address || ''
  const day = target ? dayForStop(target) : selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]
  mapPickDayID.value = day?.id || tripDocument.value.days[0]?.id || ''
  mapPickMode.value = false
  mapPickOpen.value = true
}

function cancelMapPick() {
  mapPickOpen.value = false
  mapPickMode.value = false
  mapPickTargetID.value = ''
  mapPickDayID.value = ''
  mapPickLocation.value = null
  mapPickTitle.value = ''
  mapPickAddress.value = ''
}

async function saveMapPick() {
  if (readOnlyView.value || !selected.value || !tripDocument.value || !mapPickLocation.value || !mapPickTitle.value.trim()) { error.value = '请填写地点名称'; return }
  actionLoading.value = true
  error.value = ''
  const point = mapPickLocation.value
  const provider = selectedMapProvider.value
  try {
    if (mapPickTargetID.value) {
      const target = findPlanningPoint(mapPickTargetID.value)
      if (!target) throw new Error('找不到要更新的规划点')
      await persistPlanningPointUpdate(target, { title: mapPickTitle.value.trim(), address: mapPickAddress.value.trim(), location: locationForMapPoint(point, provider) })
      cancelMapPick()
      return
    }
    if (!mapPickDayID.value) { error.value = '请选择行程日期'; return }
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(mapPickDayID.value) + '/stops', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ stop: { title: mapPickTitle.value.trim(), address: mapPickAddress.value.trim(), location: locationForMapPoint(point, provider) } }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地图选点失败')
    applyTripPayload(payload)
    selectedDay.value = tripDocument.value.days.findIndex(day => day.id === mapPickDayID.value) + 1
    cancelMapPick()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存地图选点失败'
  } finally {
    actionLoading.value = false
  }
}
function attachMapLabel(marker: any, title: string, date: string) {
  if (!showMapLabels.value || !mapAPI?.Label || !mapAPI?.Size || typeof marker.setLabel !== 'function') return
  const label = new mapAPI.Label(title + ' · ' + date, { offset: new mapAPI.Size(16, -20) })
  label.setStyle?.({ color: '#172624', backgroundColor: '#ffffffdd', border: '1px solid #6f797a', borderRadius: '8px', padding: '4px 7px', fontSize: '12px', lineHeight: '16px', whiteSpace: 'nowrap', boxShadow: '0 3px 10px #0003' })
  marker.setLabel(label)
}
function formatDistance(meters?: number) { if (!meters || meters < 0) return ''; return meters < 1000 ? Math.round(meters) + ' m' : (Math.round(meters / 100) / 10).toFixed(1).replace(/\.0$/, '') + ' km' }
function formatDuration(seconds?: number) { if (!seconds || seconds < 0) return ''; const minutes = Math.max(1, Math.round(seconds / 60)); return minutes < 60 ? minutes + ' 分钟' : Math.floor(minutes / 60) + ' 小时' + (minutes % 60 ? ' ' + minutes % 60 + ' 分钟' : '') }
function attachRouteLabel(snapshot: { geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; coordinate_system?: string }) {
  if (!showMapLabels.value || !mapAPI?.Label || !mapAPI?.Size || !snapshot.geometry?.length) return
  const text = [formatDistance(snapshot.distance_m), formatDuration(snapshot.duration_s)].filter(Boolean).join(' · '); if (!text) return
  const middle = mapRoutePoint(snapshot.geometry[Math.floor(snapshot.geometry.length / 2)], snapshot.coordinate_system || 'bd09ll'); if (!middle) return
  const label = new mapAPI.Label(text, { offset: new mapAPI.Size(-24, -10) })
  label.setStyle?.({ color: '#ffffff', backgroundColor: '#24695cdd', border: '0', borderRadius: '999px', padding: '4px 8px', fontSize: '12px', lineHeight: '16px', whiteSpace: 'nowrap', boxShadow: '0 3px 10px #0003' })
  label.setPosition?.(new mapAPI.Point(middle.lng, middle.lat)); mapInstance.addOverlay(label)
}
function applyTripPayload(payload: { document?: TripDocument; title?: string; start_date?: string; end_date?: string; revision?: number; stops?: number; days?: number; updated_at?: string }) {
  const previousStopID = selectedStopId.value
  const previousSubStopID = selectedSubStopId.value
  const previousSelected = selected.value
  if (payload.document) tripDocument.value = payload.document
  if (selected.value) {
    const range = tripDateRangeFor(payload.document || tripDocument.value, previousSelected)
    const nextTitle = payload.title || payload.document?.title || selected.value.title
    const nextStartDate = payload.start_date || range.start || selected.value.start_date
    const nextEndDate = payload.end_date || range.end || selected.value.end_date
    selected.value = { ...selected.value, title: nextTitle, start_date: nextStartDate, end_date: nextEndDate, revision: payload.revision ?? selected.value.revision, stops: payload.stops ?? selected.value.stops, days: payload.days ?? selected.value.days, updated_at: payload.updated_at ?? selected.value.updated_at }
    const index = trips.value.findIndex(trip => trip.id === selected.value?.id)
    if (index >= 0) trips.value[index] = { ...trips.value[index], title: nextTitle, start_date: nextStartDate, end_date: nextEndDate, revision: selected.value.revision, stops: selected.value.stops, days: selected.value.days, updated_at: payload.updated_at ?? trips.value[index].updated_at }
  }
  const previousStop = previousStopID ? findPlanningPoint(previousStopID) : null
  selectedStopId.value = previousStop && !isChildStop(previousStop) ? previousStop.id : previousStop ? parentForStop(previousStop)?.id || '' : ''
  selectedSubStopId.value = previousSubStopID && selectedStopId.value ? previousSubStopID : ''
  syncNavigationURL('replace')
}
async function searchPlaces() {
  if (!searchQuery.value.trim()) { searchMessage.value = '请输入景点、酒店、餐厅或地址'; return }
  searchLoading.value = true; searchMessage.value = ''; searchResults.value = []; selectedSearchResultIndex.value = -1; clearSearchResultMarkers()
  try {
    const response = await apiFetch('/api/v1/maps/pois/search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider: poiProviderPriority.value, query: searchQuery.value.trim(), region: searchRegion.value.trim(), category: searchCategory.value === 'all' ? undefined : searchCategory.value, page: 1, page_size: 10 }) })
    const payload = await response.json() as { items?: PlaceCandidate[]; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '地点搜索失败')
    searchResults.value = payload.items || []
    if (!searchResults.value.length) searchMessage.value = '没有找到结果，请补充城市或更换关键词'
    renderSearchResultMarkers()
  } catch (cause) { searchMessage.value = cause instanceof Error ? cause.message : '地点搜索失败' } finally { searchLoading.value = false }
}
async function addPlaceToTrip(candidate: PlaceCandidate) {
  if (readOnlyView.value || !selected.value || !tripDocument.value) { searchMessage.value = '请先创建或选择一条旅行规划'; return }
  const day = selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]
  if (!day) { searchMessage.value = '当前规划没有可用日期'; return }
  const location = candidate.location
  if (!location || !Number.isFinite(location.lat) || !Number.isFinite(location.lng) || !candidateCoordinateCRS(candidate)) { searchMessage.value = '搜索结果没有可靠坐标或 CRS，未添加'; return }
  actionLoading.value = true
  const parentID = searchParentStopId.value
  const endpoint = parentID ? '/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(parentID) + '/children' : '/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops'
  try {
    const response = await apiFetch(endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ stop: { title: candidate.name, address: candidate.address, location: savedLocationFor(candidate) } }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '添加规划点失败')
    applyTripPayload(payload)
    if (parentID) { selectedStopId.value = parentID; selectedSubStopId.value = ''; searchMessage.value = '已添加“' + candidate.name + '”为子规划点' } else { searchMessage.value = '已添加“' + candidate.name + '”，路线尚未生成' }
    locationSearchMode.value = 'add'; locationSearchTargetID.value = ''; locationSearchTargetDayID.value = ''; locationSearchTitleDraft.value = ''
    searchParentStopId.value = ''; panelMode.value = 'journey'; searchResults.value = []
    setSheetBreakpoint('half', 'replace')
  } catch (cause) { searchMessage.value = cause instanceof Error ? cause.message : '添加规划点失败' } finally { actionLoading.value = false }
}

async function updatePlanningPointFromCandidate(candidate: PlaceCandidate) {
  if (readOnlyView.value || !selected.value || !tripDocument.value) { searchMessage.value = '当前行程不可编辑'; return }
  const target = locationSearchTargetID.value ? findPlanningPoint(locationSearchTargetID.value) : null
  if (!target) { searchMessage.value = '找不到要重新定位的规划点'; return }
  const location = candidate.location
  if (!location || !Number.isFinite(location.lat) || !Number.isFinite(location.lng) || !candidateCoordinateCRS(candidate)) { searchMessage.value = '搜索结果没有可靠坐标或 CRS，未更新'; return }
  const title = locationSearchTitleDraft.value.trim() || candidate.name.trim()
  if (!title) { searchMessage.value = '请填写规划点名称'; return }
  const address = candidate.address?.trim() || target.address || ''
  actionLoading.value = true
  searchMessage.value = ''
  try {
    await persistPlanningPointUpdate(target, { title, address, location: savedLocationFor(candidate) })
    locationSearchMode.value = 'add'; locationSearchTargetID.value = ''; locationSearchTargetDayID.value = ''; locationSearchTitleDraft.value = ''
    searchParentStopId.value = ''; panelMode.value = 'journey'; searchResults.value = []
    searchMessage.value = '已更新“' + title + '”的位置；受影响路线和天气已清除。'
    setSheetBreakpoint('half', 'replace')
  } catch (cause) {
    searchMessage.value = cause instanceof Error ? cause.message : '更新规划点位置失败'
  } finally {
    actionLoading.value = false
  }
}

function applySearchResult(candidate: PlaceCandidate) {
  if (locationSearchMode.value === 'repair') void updatePlanningPointFromCandidate(candidate)
  else void addPlaceToTrip(candidate)
}

async function planRoutes() {
  if (readOnlyView.value || planningLoading.value) return
  if (!selected.value || !tripDocument.value) { error.value = '请先选择一条旅行规划'; return }
  if (unlocatedMainStops.value.length) { error.value = '还有 ' + unlocatedMainStops.value.length + ' 个主规划点待定位，请先重新搜索或使用地图选点'; return }
  if (!plannableDays.value.length) { error.value = '至少有两个相邻的带坐标规划点后才能生成路线'; return }
  planningLoading.value = true; error.value = ''
  try {
    const day = selectedDay.value === 'all' ? undefined : tripDocument.value.days[selectedDay.value - 1]?.id
    const provider = planningProvider.value
    localStorage.setItem('journeyin.planningProvider', provider)
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/plan', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ provider, mode: planningMode.value, strategy: planningMode.value === 'driving' && supportsDrivingStrategy.value ? planningStrategy.value : undefined, day_id: day }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '路线生成失败')
    applyTripPayload(payload)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '路线生成失败' } finally { planningLoading.value = false }
}

function openImportPicker() { fileInput.value?.click() }
function importErrorMessage(payload: { error?: { message?: string; details?: { issues?: Array<{ path?: string; message?: string }> } } }, fallback: string) {
  const issues = payload.error?.details?.issues || []
  const detail = issues.map(issue => [issue.path, issue.message].filter(Boolean).join(': ')).filter(Boolean).join('；')
  return detail ? fallback + '：' + detail : payload.error?.message || fallback
}
async function importTrip(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  actionLoading.value = true; error.value = ''
  try {
    const document = await file.text()
    try { JSON.parse(document) } catch { throw new Error('导入文件不是有效的 JSON') }
    const validation = await apiFetch('/api/v1/validate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: document })
    const validationPayload = await validation.json() as { error?: { message?: string; details?: { issues?: Array<{ path?: string; message?: string }> } } }
    if (!validation.ok) throw new Error(importErrorMessage(validationPayload, 'Trip 校验失败'))
    const response = await apiFetch('/api/v1/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: document })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string; details?: { issues?: Array<{ path?: string; message?: string }> } } }; throw new Error(importErrorMessage(payload, '导入失败')) }
    await loadTrips()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导入失败' } finally { actionLoading.value = false; input.value = '' }
}
async function downloadTrip() {
  if (!selected.value) return
  actionLoading.value = true; error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/export.json')
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '导出失败') }
    const blob = await response.blob(); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = (selected.value.title || 'journeyin-trip').replace(/[\\/:*?"<>|]/g, '_') + '.json'; document.body.appendChild(anchor); anchor.click(); anchor.remove(); URL.revokeObjectURL(url)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导出失败' } finally { actionLoading.value = false }
}
function shareStorageKey(tripID: string) { return 'journeyin.share.' + tripID }
function shareTokenFromURL(url: string) { try { const parsed = new URL(url, window.location.origin); const match = parsed.pathname.match(/^\/s\/([^/]+)$/); return match?.[1] || '' } catch { return '' } }
function saveShareState(tripID: string) { if (shareURL.value) localStorage.setItem(shareStorageKey(tripID), JSON.stringify({ id: shareID.value, url: shareURL.value, expires_at: shareExpiresAt.value })) }
function restoreShareState(tripID: string) {
  shareURL.value = ''; shareID.value = ''; shareExpiresAt.value = ''; shareCopyMessage.value = ''; shareNoticeVisible.value = false
  try {
    const saved = JSON.parse(localStorage.getItem(shareStorageKey(tripID)) || 'null') as { id?: string; url?: string; expires_at?: string } | null
    if (!saved?.url || (saved.expires_at && Date.parse(saved.expires_at) <= Date.now())) { localStorage.removeItem(shareStorageKey(tripID)); return }
    shareID.value = saved.id || ''; shareURL.value = saved.url; shareExpiresAt.value = saved.expires_at || ''
  } catch { localStorage.removeItem(shareStorageKey(tripID)) }
}
async function createShare() {
  if (readOnlyView.value || !selected.value) return
  const tripID = selected.value.id; const existingToken = shareTokenFromURL(shareURL.value) || (() => { try { const saved = JSON.parse(localStorage.getItem(shareStorageKey(tripID)) || 'null') as { url?: string } | null; return saved?.url ? shareTokenFromURL(saved.url) : '' } catch { return '' } })()
  actionLoading.value = true; error.value = ''; shareCopyMessage.value = ''
  try {
    const response = await apiFetch('/api/v1/shares', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ trip_id: tripID, existing_token: existingToken || undefined }) })
    const payload = await response.json() as { id?: string; url?: string; expires_at?: string; error?: { message?: string } }
    if (!response.ok || !payload.url) throw new Error(payload.error?.message || '分享链接创建失败')
    shareID.value = payload.id || ''; shareURL.value = payload.url; shareExpiresAt.value = payload.expires_at || ''; shareNoticeVisible.value = true; saveShareState(tripID)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '分享链接创建失败' } finally { actionLoading.value = false }
}
async function copyText(value: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return
    }
  } catch { /* HTTP pages may expose clipboard but reject it as insecure. */ }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)
  let copied = false
  try { copied = document.execCommand('copy') } finally { textarea.remove() }
  if (!copied) throw new Error('clipboard_unavailable')
}
async function copyShareURL() {
  if (!shareURL.value) return
  shareCopyMessage.value = ''
  error.value = ''
  try { await copyText(shareURL.value); shareCopyMessage.value = '分享链接已复制' } catch { error.value = '当前浏览器禁止自动复制，请长按或手动复制分享链接' }
}
function dismissShareNotice() {
  shareNoticeVisible.value = false
}
async function revokeShare() {
  if (!shareID.value || !window.confirm('确认撤销当前在线分享吗？撤销后链接将无法访问。')) return
  actionLoading.value = true
  try {
    const response = await apiFetch('/api/v1/shares/' + encodeURIComponent(shareID.value) + '/revoke', { method: 'POST' })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '撤销分享失败') }
    shareURL.value = ''; shareID.value = ''; shareExpiresAt.value = ''; shareCopyMessage.value = ''; shareNoticeVisible.value = false; if (selected.value) localStorage.removeItem(shareStorageKey(selected.value.id)); settingsMessage.value = '在线分享已撤销'
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '撤销分享失败' } finally { actionLoading.value = false }
}
function safeURL(raw: string) { try { const parsed = new URL(raw); return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : '#' } catch { return '#' } }
function escapeHTML(value: string) { return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('\"', '&quot;').replaceAll("'", '&#39;') }
function renderMarkdown(source: string) {
  return DOMPurify.sanitize(markdownRenderer.render(source), markdownRendererConfig)
}
async function openNavigation(provider: 'baidu' | 'amap') {
  const stop = selectedTarget.value
  const point = stop && navigationPointFor(stop, provider)
  if (!stop || !point) { error.value = '该规划点没有可靠坐标，无法生成导航链接'; return }
  const platform = navigationPlatform()
  const reservedWindow = reserveNavigationWindow(platform)
  try {
    const response = await apiFetch('/api/v1/maps/navigation', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider, target: { name: stop.title, address: stop.address, location: point }, mode: 'walking', platform }) })
    const payload = await response.json() as { url?: string; fallback_url?: string; error?: { message?: string } }
    if (!response.ok || !payload.url) throw new Error(payload.error?.message || '导航链接生成失败')
    openNavigationURL(payload.url, platform, reservedWindow, payload.fallback_url || '')
  } catch (cause) {
    if (reservedWindow && !reservedWindow.closed) reservedWindow.close()
    error.value = cause instanceof Error ? cause.message : '导航失败'
  }
}
function weatherText(stop: Stop | SubStop) { const weather = stop.weather || {}; const condition = weather.condition || weather.text_day || weather.text || '暂无天气快照'; const temperature = weather.temperature_c ?? weather.temp; return temperature === undefined ? String(condition) : String(condition) + ' · ' + String(temperature) + '°C' }
function weatherUpdatedAt(stop: Stop | SubStop) { const value = stop.weather?.fetched_at; return value ? formatDateTime(String(value)) : '' }
async function refreshWeather() {
  if (readOnlyView.value || !selected.value || !tripDocument.value || !selectedTarget.value) { error.value = '请先选择一个有坐标的规划点'; return }
  const day = dayForStop(selectedTarget.value); const parent = selectedStop.value; if (!day || !parent) { error.value = '无法确定天气对应日期'; return }
  weatherLoading.value = true; error.value = ''
  const childID = selectedSubStop.value?.id || ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(selectedTarget.value.id) + '/weather', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ provider: selectedMapProvider.value, local_date: day.date }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '天气查询失败')
    applyTripPayload(payload); selectedStopId.value = parent.id; selectedSubStopId.value = childID
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '天气查询失败' } finally { weatherLoading.value = false }
}
function closeDetail() {
  if (selectedSubStopId.value) navigateBackFromSubStop()
  else if (selectedStopId.value) navigateBackFromStop()
  else navigateBackToList()
}
function findPlanningPoint(id: string): Stop | SubStop | null {
  if (!tripDocument.value) return null
  for (const day of tripDocument.value.days) for (const stop of day.stops || []) {
    if (stop.id === id) return stop
    const child = stop.children?.find(item => item.id === id)
    if (child) return child
  }
  return null
}

function parentForStop(stop: Stop | SubStop, day: Day | null = dayForStop(stop)) {
  if (!day) return null
  return day.stops.find(parent => parent.id === stop.id) || day.stops.find(parent => parent.children?.some(child => child.id === stop.id)) || null
}
function isChildStop(stop: Stop | SubStop) {
  const parent = parentForStop(stop)
  return Boolean(parent && parent.id !== stop.id)
}
function selectPlanningPointFromList(stop: Stop | SubStop) {
  const day = dayForStop(stop)
  const dayIndex = day && tripDocument.value ? tripDocument.value.days.findIndex(item => item.id === day.id) : -1
  if (selectedDay.value !== 'all' && dayIndex >= 0) selectedDay.value = dayIndex + 1
  selectStop(stop)
}



function toggleReorderMode() {
  if (readOnlyView.value) return
  reorderMode.value = !reorderMode.value
  reorderMessage.value = reorderMode.value ? '点击规划点右侧的上移或下移按钮调整顺序。' : ''
}
type PlanningPointMoveTarget = { day: Day; sequence: number }

function planningPointNeighbors(stop: Stop) {
  const days = tripDocument.value?.days || []
  const day = dayForStop(stop)
  const dayIndex = day ? days.findIndex(item => item.id === day.id) : -1
  if (!day || dayIndex < 0) return { day: null, index: -1, previous: null as PlanningPointMoveTarget | null, next: null as PlanningPointMoveTarget | null }
  const stops = orderedStops(day.stops || [])
  const index = stops.findIndex(item => item.id === stop.id)
  if (index < 0) return { day, index, previous: null as PlanningPointMoveTarget | null, next: null as PlanningPointMoveTarget | null }
  const previousDay = days[dayIndex - 1]
  const nextDay = days[dayIndex + 1]
  const previousStops = orderedStops(previousDay?.stops || [])
  return {
    day,
    index,
    previous: index > 0 ? { day, sequence: index } : previousDay ? { day: previousDay, sequence: previousStops.length + 1 } : null,
    next: index < stops.length - 1 ? { day, sequence: index + 2 } : nextDay ? { day: nextDay, sequence: 1 } : null,
  }
}

function canMovePlanningPoint(stop: Stop, direction: -1 | 1) {
  const neighbors = planningPointNeighbors(stop)
  return Boolean(direction < 0 ? neighbors.previous : neighbors.next)
}

async function movePlanningPoint(stop: Stop, direction: -1 | 1) {
  if (readOnlyView.value || actionLoading.value) return
  const neighbors = planningPointNeighbors(stop)
  const target = direction < 0 ? neighbors.previous : neighbors.next
  if (!neighbors.day || !target) return
  if (target.day.id !== neighbors.day.id) {
    await movePlanningPointToDay(stop, target)
    return
  }
  await reorderPlanningPointTo(stop, target.sequence)
}

async function movePlanningPointToDay(stop: Stop, target: PlanningPointMoveTarget) {
  if (readOnlyView.value || !selected.value) return
  const sourceDay = dayForStop(stop)
  const targetDayIndex = tripDocument.value?.days.findIndex(day => day.id === target.day.id) ?? -1
  if (!sourceDay || targetDayIndex < 0) return
  const revision = selected.value.revision
  actionLoading.value = true; error.value = ''; reorderMessage.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(sourceDay.id) + '/stops/' + encodeURIComponent(stop.id) + '/move', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision }, body: JSON.stringify({ target_day_id: target.day.id, target_sequence: target.sequence }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; updated_at?: string; error?: { message?: string } }
    if (!response.ok) {
      if (response.status === 409 && selected.value) { await loadDetail(selected.value); throw new Error('行程已被其他操作更新，请重新选择后再排序') }
      throw new Error(payload.error?.message || '调整规划点日期失败')
    }
    applyTripPayload(payload)
    if (selectedDay.value !== 'all') selectedDay.value = targetDayIndex + 1
    reorderMessage.value = '规划点已移动到 D' + (targetDayIndex + 1) + '，路线已清除，请点击“生成路线”重新规划'
    syncNavigationURL('replace')
    await renderMap()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '调整规划点日期失败' } finally { actionLoading.value = false }
}
async function reorderPlanningPointTo(stop: Stop | SubStop, targetSequence: number) {
  if (readOnlyView.value || !selected.value) return
  const day = dayForStop(stop); const parent = parentForStop(stop, day); if (!day || !parent) return
  const child = isChildStop(stop); const stopID = stop.id; const revision = selected.value.revision
  actionLoading.value = true; error.value = ''; reorderMessage.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(stopID) + '/move', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision }, body: JSON.stringify({ target_sequence: targetSequence }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) {
      if (response.status === 409 && selected.value) { await loadDetail(selected.value); throw new Error('行程已被其他操作更新，请重新选择后再排序') }
      throw new Error(payload.error?.message || '调整规划点顺序失败')
    }
    applyTripPayload(payload)
    reorderMessage.value = child ? '子规划点顺序已更新' : '规划点顺序已更新，路线已清除，请点击“生成路线”重新规划'
    await renderMap()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '调整规划点顺序失败' } finally { actionLoading.value = false }
}
async function deletePlanningPoint(stop: Stop | SubStop) {
  if (readOnlyView.value || !selected.value) return
  const day = dayForStop(stop); if (!day) { error.value = '无法确定规划点所属日期'; return }
  const isChild = isChildStop(stop); const label = isChild ? '子规划点' : '规划点'; if (!window.confirm('确认删除“' + stop.title + '”这个' + label + '吗？')) return
  actionLoading.value = true; error.value = ''
  try { const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(stop.id), { method: 'DELETE', headers: { 'If-Match': 'revision-' + selected.value.revision } }); const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }; if (!response.ok) throw new Error(payload.error?.message || '删除规划点失败'); applyTripPayload(payload); closeDetail() } catch (cause) { error.value = cause instanceof Error ? cause.message : '删除规划点失败' } finally { actionLoading.value = false }
}
async function openSettings() {
  settingsOpen.value = true
  try {
    const response = await apiFetch('/api/v1/settings')
    if (!response.ok) throw new Error('无法读取设置')
    settingsData.value = await response.json() as KeySettings
    if (settingsData.value.map?.default_provider === 'baidu' || settingsData.value.map?.default_provider === 'amap') defaultMapProvider.value = settingsData.value.map.default_provider
    poiProviderPriority.value = settingsData.value.poi?.provider_priority === 'baidu' ? 'baidu' : 'amap'
    localDirectoryCount.value = settingsData.value.poi?.local_directory_count || 0
    baiduBrowserKeyInput.value = ''
    baiduServerKeyInput.value = ''
    amapJSKeyInput.value = ''
    amapServerKeyInput.value = ''
    amapSecurityJSCodeInput.value = ''
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '无法读取设置' }
}

async function saveDefaultMapProvider() {
  settingsSaving.value = true
  try {
    const response = await apiFetch('/api/v1/settings/map', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ default_provider: defaultMapProvider.value }) })
    const payload = await response.json() as { default_provider?: 'baidu' | 'amap'; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存默认地图失败')
    const provider = payload.default_provider === 'amap' ? 'amap' : 'baidu'
    defaultMapProvider.value = provider
    if (settingsData.value) settingsData.value = { ...settingsData.value, map: { ...settingsData.value.map, default_provider: provider } }
    if (capabilities.value) capabilities.value = { ...capabilities.value, default_map_provider: provider }
    if (!tripDocument.value?.map?.preferred_provider) {
      const changed = selectedMapProvider.value !== provider
      selectedMapProvider.value = provider
      planningProvider.value = provider
      localStorage.setItem('journeyin.mapProvider', provider)
      localStorage.setItem('journeyin.planningProvider', provider)
      if (changed) {
        resetMapSDK()
        await nextTick()
        await renderMap()
      }
    }
    settingsMessage.value = '默认地图已保存：' + (provider === 'amap' ? '高德地图' : '百度地图')
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '保存默认地图失败' } finally { settingsSaving.value = false }
}

async function savePOIPreferences() {
  try {
    const response = await apiFetch('/api/v1/settings/poi', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider_priority: poiProviderPriority.value }) })
    const payload = await response.json() as { error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地点检索优先级失败')
    settingsMessage.value = '地点检索优先级已保存：' + (poiProviderPriority.value === 'amap' ? '高德优先' : '百度优先')
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '保存地点检索优先级失败' }
}
async function clearLocalDirectory() {
  if (!window.confirm('确认清除本地地点检索记录吗？已保存到 Trip 的规划点不会被删除。')) return
  try {
    const response = await apiFetch('/api/v1/settings/place-directory', { method: 'DELETE' })
    const payload = await response.json() as { error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '清除本地地点记录失败')
    localDirectoryCount.value = 0; settingsMessage.value = '本地地点检索记录已清除；Trip 中已保存的规划点不受影响。'
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '清除本地地点记录失败' }
}
async function saveMapKeys() {
  settingsSaving.value = true
  try {
    const body: Record<string, string> = {}
    if (baiduBrowserKeyInput.value.trim()) body.baidu_browser_key = baiduBrowserKeyInput.value.trim()
    if (baiduServerKeyInput.value.trim()) body.baidu_server_key = baiduServerKeyInput.value.trim()
    if (amapJSKeyInput.value.trim()) body.amap_js_key = amapJSKeyInput.value.trim()
    if (amapServerKeyInput.value.trim()) body.amap_server_key = amapServerKeyInput.value.trim()
    if (amapSecurityJSCodeInput.value.trim()) body.amap_security_js_code = amapSecurityJSCodeInput.value.trim()
    if (!Object.keys(body).length) { settingsMessage.value = '未填写新的 Key，现有配置保持不变。'; return }
    const response = await apiFetch('/api/v1/settings/map-keys', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    const payload = await response.json() as { error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地图 Key 失败')
    settingsMessage.value = '地图 Key 已保存到 SQLite；浏览器端 Key 已立即生效。'
    baiduServerKeyInput.value = ''; amapJSKeyInput.value = ''; amapServerKeyInput.value = ''; amapSecurityJSCodeInput.value = ''
    await loadTrips(); await openSettings()
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '保存地图 Key 失败' } finally { settingsSaving.value = false }
}
async function login() {
  if (!loginUsername.value.trim() || !loginPassword.value) { loginMessage.value = '请输入账号和密码'; return }
  loginLoading.value = true; loginMessage.value = ''
  try {
    const response = await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin', body: JSON.stringify({ username: loginUsername.value, password: loginPassword.value }) })
    const payload = await response.json() as { error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '登录失败')
    authOpen.value = false; loginPassword.value = ''; loginMessage.value = ''; settingsMessage.value = '登录成功'; await loadTrips()
  } catch (cause) { loginMessage.value = cause instanceof Error ? cause.message : '登录失败' } finally { loginLoading.value = false }
}
function saveAuth() { authTokenInput.value = authTokenInput.value.trim(); if (authTokenInput.value) localStorage.setItem('journeyin.apiToken', authTokenInput.value); else localStorage.removeItem('journeyin.apiToken'); authOpen.value = false; settingsMessage.value = '兼容 API Token 已保存'; loadTrips() }
async function logout() { authTokenInput.value = ''; localStorage.removeItem('journeyin.apiToken'); await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' }).catch(() => undefined); authOpen.value = false; settingsMessage.value = '已退出登录'; await loadTrips() }
function applyTheme() { const actual = theme.value === 'system' ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme.value; document.documentElement.dataset.theme = actual; localStorage.setItem('journeyin.theme', theme.value) }
function setTheme(value: Theme) { theme.value = value; applyTheme() }
function systemThemeChanged() { if (theme.value === 'system') applyTheme() }
watch([selectedDay, tripDocument], () => { renderMap() }, { deep: true })
onMounted(() => {
  if (prototypeMode) return
  ensureNavigationHistory()
  window.addEventListener('popstate', handleNavigationPopState)
  window.addEventListener('keydown', handleGlobalKeyDown)
  window.addEventListener('resize', handleViewportResize)
  window.addEventListener('pointerdown', handleTouchPointerDown, true)
  window.addEventListener('pointermove', handleTouchPointerMove, true)
  window.addEventListener('pointerup', finishTouchPointer, true)
  window.addEventListener('pointercancel', finishTouchPointer, true)
  applyTheme(); mediaQuery = window.matchMedia('(prefers-color-scheme: dark)'); mediaQuery.addEventListener?.('change', systemThemeChanged); if (shareMode) void loadSharedTrip(); else loadTrips()
})
onUnmounted(() => {
  mediaQuery?.removeEventListener?.('change', systemThemeChanged)
  window.removeEventListener('popstate', handleNavigationPopState)
  window.removeEventListener('keydown', handleGlobalKeyDown)
  window.removeEventListener('resize', handleViewportResize)
  window.removeEventListener('pointerdown', handleTouchPointerDown, true)
  window.removeEventListener('pointermove', handleTouchPointerMove, true)
  window.removeEventListener('pointerup', finishTouchPointer, true)
  window.removeEventListener('pointercancel', finishTouchPointer, true)
  touchGesture = null
  document.documentElement.classList.remove('journey-touch-gesture')
  sheetDragCleanup?.()
  if (sheetDragReleaseTimer !== null) { window.clearTimeout(sheetDragReleaseTimer); sheetDragReleaseTimer = null }
  sheetDragActive.value = false
  sheetDragHeight.value = null
})
</script>

<template>
  <PrototypePreview v-if="prototypeMode" />
  <IonApp v-else>
    <div class="journey-page redesign-page">
      <div class="redesign-content">
        <main class="journey-redesign" :class="{ 'is-list-view': tripView === 'list', 'is-detail-view': tripView === 'detail', 'has-stop-selection': Boolean(selectedStop), 'is-shared-view': shareMode, 'is-history-view': Boolean(historyView) }">
          <input ref="fileInput" class="visually-hidden" type="file" accept="application/json,.json" aria-hidden="true" tabindex="-1" @change="importTrip" />
          <aside class="journey-rail" aria-label="JourneyIn 主导航">
            <button class="rail-brand" type="button" aria-label="返回行程列表" @click="navigateToList()"><span class="rail-brand-mark">✦</span><span class="rail-brand-name">JourneyIn</span></button>
            <nav class="rail-nav" aria-label="工作区">
              <button class="rail-nav-item" :class="{ selected: tripView === 'list' }" type="button" @click="navigateToList()"><IonIcon :icon="menuOutline" /><span>行程</span></button>
              <button class="rail-nav-item" :class="{ selected: tripView === 'detail' }" type="button" :disabled="!selected" @click="selected ? navigateToTrip(selected, 'replace') : undefined"><IonIcon :icon="mapOutline" /><span>地图</span></button>
            </nav>
            <div class="rail-spacer"></div>
            <a v-if="!readOnlyView" class="rail-nav-item rail-link" :href="GITHUB_URL" target="_blank" rel="noopener noreferrer"><IonIcon :icon="linkOutline" /><span>项目</span></a>
            <button v-if="!readOnlyView" class="rail-nav-item" type="button" @click="openSettings()"><IonIcon :icon="settingsOutline" /><span>设置</span></button>
          </aside>

          <section v-if="tripView === 'list'" class="trip-list-view" aria-labelledby="trip-list-title">
            <div ref="tripListScroll" class="trip-list-scroll" tabindex="0" role="region" aria-label="行程列表" @pointerdown="focusTripListScroll">
            <header class="list-page-header">
              <div>
                <p class="eyebrow">JOURNEYIN WORKSPACE</p>
                <h1 id="trip-list-title">你的行程</h1>
                <p class="list-page-subtitle">把下一段旅程放在地图上</p>
              </div>
              <div class="list-page-actions">
                <button class="secondary-action" type="button" @click="openImportPicker"><IonIcon :icon="linkOutline" /> 导入 Trip</button>
                <button class="primary-action" type="button" @click="newTripOpen = true"><IonIcon :icon="addOutline" /> 新建行程</button>
              </div>
            </header>

            <section class="list-summary-bar" aria-label="行程筛选">
              <span>全部行程 <strong>{{ trips.length }}</strong></span>
              <div class="list-summary-controls"><span class="list-summary-note">{{ loading ? '正在同步…' : tripSortMode === 'date' ? '按行程开始日期排序' : '按最后修改时间排序' }}</span><label class="list-sort-control"><span>排序</span><UiSelect v-model="tripSortMode" aria-label="行程列表排序方式" :options="tripSortOptions" /></label></div>
            </section>

            <div v-if="error" class="inline-error"><IonIcon :icon="cloudOfflineOutline" /><span>{{ error }}</span><button type="button" aria-label="关闭错误" @click="error = ''">×</button></div>
            <div v-if="loading" class="list-loading"><span class="loading-dot"></span><span>正在加载行程…</span></div>
            <div v-else-if="!trips.length" class="list-empty-state"><div class="empty-mark"><IonIcon :icon="mapOutline" /></div><h2>还没有旅行规划</h2><p>创建一条行程，开始把地点和路线整理到地图上。</p><div class="empty-actions"><button class="primary-action" type="button" @click="newTripOpen = true"><IonIcon :icon="addOutline" /> 新建规划</button><button class="secondary-action" type="button" @click="openImportPicker">导入 Trip JSON</button></div></div>
            <div v-else class="trip-list-grid">
              <article v-for="(trip, index) in sortedTrips" :key="trip.id" class="trip-list-card" :class="{ active: selected?.id === trip.id }">
                <button class="trip-card-main" type="button" @click="selectTrip(trip)">
                  <span class="trip-card-visual" :class="'trip-card-visual-' + (index % 4)" aria-hidden="true"><span>{{ trip.title.slice(0, 2) }}</span></span>
                  <span class="trip-card-copy"><strong>{{ trip.title }}</strong><span>{{ formatDateRange(trip.start_date, trip.end_date) }}</span><small>{{ trip.days ?? '—' }} 天 · {{ trip.stops ?? '—' }} 个规划点 · revision {{ trip.revision }}</small><small v-if="trip.updated_at" class="trip-card-updated">最后修改 {{ formatDateTime(trip.updated_at) }}</small></span>
                  <span class="trip-card-arrow">›</span>
                </button>
                <button class="trip-card-menu-button" type="button" :aria-expanded="tripMenuID === trip.id" :aria-label="'打开 ' + trip.title + ' 更多操作'" @click.stop="toggleTripMenu(trip.id)">⋯</button>
                <div v-if="tripMenuID === trip.id" class="trip-card-menu" role="menu"><button type="button" role="menuitem" @click="editTripDetailsFromList(trip)"><IonIcon :icon="createOutline" /> 编辑行程信息</button><button type="button" role="menuitem" @click="openTripHistoryFromList(trip)"><span aria-hidden="true">↶</span> 版本历史</button><button type="button" role="menuitem" class="danger-menu-item" @click="tripMenuID = ''; deleteTrip(trip)"><IonIcon :icon="closeOutline" /> 删除行程</button></div>
              </article>
            </div>
            </div>
          </section>

          <section v-else class="journey-workspace" aria-label="地图工作区">
            <div class="map-canvas redesign-map-canvas" :class="{ 'map-pick-active': mapPickMode }">
              <div v-if="keyConfigured && tripDocument && !mapError" ref="mapContainer" id="map"></div>
              <div v-if="!tripDocument || !keyConfigured || mapError" class="map-fallback">
                <IonIcon :icon="mapOutline" />
                <strong>{{ !tripDocument ? '正在读取行程地图' : mapError || (mapProviderLabel + '未配置') }}</strong>
                <span>{{ !tripDocument ? '地图工作区即将准备完成。' : mapError ? '请确认浏览器端 Key、域名白名单和网络连接。当前页面：' + serverURL : '配置' + mapProviderLabel + '浏览器端 Key 后显示真实地图；已保存的行程数据仍然可查看。' }}</span>
                <IonChip color="warning"><IonIcon :icon="cloudOfflineOutline" /> {{ !tripDocument ? '读取中' : '降级模式' }}</IonChip>
              </div>
              <div v-if="keyConfigured && tripDocument && !mapError && !mapReady && !mapWarning" class="map-loading"><IonIcon :icon="mapOutline" /><span>正在加载{{ mapProviderLabel }}…</span></div>
              <div v-if="mapWarning" class="map-warning"><span>{{ mapWarning }}</span><button type="button" @click="retryMap">重新加载</button></div>

              <header class="workspace-topbar">
                <button v-if="!readOnlyView" class="workspace-back" type="button" aria-label="返回行程列表" @click="navigateBackToList"><span>‹</span><small>行程</small></button>
                <div class="workspace-title"><strong>{{ historyTitle }}</strong><span>{{ historyDateRange || '地图工作区' }}</span></div>
                <div class="workspace-top-actions">
                  <button class="workspace-tool-trigger" type="button" :aria-expanded="mobileMapToolsOpen" aria-label="打开地图选项" @click="toggleMobileMapTools"><IonIcon :icon="mapOutline" /><span>地图选项</span></button>
                  <button v-if="!readOnlyView" class="workspace-more" type="button" aria-label="打开更多操作" @click="openSettings"><span>⋯</span></button>
                </div>
              </header>

              <div class="workspace-status"><span class="status-dot" :class="{ ready: keyConfigured && mapReady && !mapError }"></span><span>{{ !tripDocument ? '读取行程…' : !keyConfigured ? '离线数据可用' : mapError ? mapProviderLabel + '不可用' : mapReady ? mapProviderLabel + '已连接' : mapProviderLabel + '加载中' }} · {{ visibleStops.length }} 个规划点<span v-if="unlocatedMainStops.length"> · 待定位 {{ unlocatedMainStops.length }}</span></span></div>
              <div v-if="mobileMapToolsOpen" class="map-tools-card" role="dialog" aria-label="地图选项">
                <div class="map-tools-heading"><div><span class="eyebrow">MAP OPTIONS</span><strong>地图选项</strong></div><button type="button" aria-label="关闭地图选项" @click="toggleMobileMapTools"><IonIcon :icon="closeOutline" /></button></div>
                <div class="map-tool-row"><span>底图 Provider</span><div class="provider-segment"><button type="button" :class="{ active: selectedMapProvider === 'baidu' }" @click="setMapProvider('baidu')">百度</button><button type="button" :class="{ active: selectedMapProvider === 'amap' }" @click="setMapProvider('amap')">高德</button></div></div>
                <div class="map-tool-row"><span>图层</span><div class="provider-segment layer-segment" role="group" aria-label="地图图层"><button type="button" :class="{ active: mapType === 'normal' }" :aria-pressed="mapType === 'normal'" @click="setMapType('normal')">标准图</button><button type="button" :class="{ active: mapType === 'satellite' }" :aria-pressed="mapType === 'satellite'" @click="setMapType('satellite')">卫星图</button></div></div>
                <div class="map-tool-row"><span>地图标签</span><button class="tool-value-button" type="button" :class="{ active: showMapLabels }" @click="toggleMapLabels">{{ showMapLabels ? '已显示' : '已隐藏' }}</button></div>
                <button v-if="!readOnlyView" class="map-pick-action" type="button" :disabled="!mapReady || !tripDocument" @click="toggleMapPick"><IonIcon :icon="mapOutline" /> {{ mapPickMode ? mapPickTargetID ? '取消更新选点' : '取消地图选点' : '地图选点' }}</button>
              </div>

              <section v-if="error || tripDetailsNotice || historyView || (shareNoticeVisible && shareURL)" class="map-notices redesign-notices">
                <div v-if="historyView" class="history-readonly-banner"><span><strong>历史版本 · 只读</strong><small>{{ historyView.label || '保存于 ' + formatDateTime(historyView.created_at) }} · 工作版本 {{ historyView.source_revision }}</small></span><button type="button" @click="exitTripHistory">返回当前版本</button></div>
                 <div v-if="error" class="global-error"><IonIcon :icon="cloudOfflineOutline" /><span>{{ error }}</span><button aria-label="关闭错误" @click="error = ''">×</button></div>
                <div v-if="tripDetailsNotice" class="global-notice"><IonIcon :icon="createOutline" /><span>{{ tripDetailsNotice }}</span><button aria-label="关闭行程更新提示" @click="tripDetailsNotice = ''">×</button></div>
                <div v-if="shareNoticeVisible && shareURL" class="share-banner"><span><strong>只读分享已创建</strong><a :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a><small v-if="shareExpiresAt">有效期至 {{ formatDateTime(shareExpiresAt) }}</small><small v-if="shareCopyMessage" class="share-copy-feedback">{{ shareCopyMessage }}</small></span><div class="share-actions"><button type="button" @click="copyShareURL">复制链接</button><button v-if="shareID" type="button" @click="revokeShare">撤销</button><button type="button" aria-label="关闭分享提示" @click="dismissShareNotice">×</button></div></div>
              </section>
            </div>

            <aside v-if="tripDocument" class="floating-panel workspace-panel itinerary-panel" :class="['sheet-' + sheetBreakpoint, { 'panel-search-mode': panelMode === 'search', 'is-sheet-dragging': sheetDragActive }]" :style="sheetDragStyle" aria-label="行程时间线">
              <button class="sheet-handle" type="button" :aria-label="sheetBreakpoint === 'peek' ? '展开行程' : '收起行程'" @pointerdown="startSheetDrag" @click="cycleSheetBreakpoint"><span></span></button>
              <header class="workspace-panel-head">
                <div><div class="workspace-panel-kicker"><p class="eyebrow">{{ historyView ? 'HISTORY VERSION' : shareMode ? 'SHARED JOURNEY' : 'CURRENT JOURNEY' }}</p><span v-if="historyView" class="history-status-tag">只读</span><span v-else-if="shareURL" class="share-status-tag">已分享</span></div><h1>{{ historyTitle }}</h1><p>{{ historyDateRange || '选择一条行程查看详情' }}</p></div>
                <div class="panel-head-actions"><button v-if="!readOnlyView" class="panel-action-button" type="button" aria-label="返回行程列表" @click="navigateBackToList"><span>‹</span><small>行程</small></button><button v-if="!readOnlyView" class="panel-action-button history-action" type="button" aria-label="打开版本历史" @click="openTripHistory"><span>↶</span><small>版本</small></button><button v-if="!readOnlyView" class="panel-action-button edit-trip-action" type="button" aria-label="编辑行程信息" @click="beginEditTripDetails"><IonIcon :icon="createOutline" /><small>编辑</small></button><button class="panel-action-button collapse-action" type="button" :aria-label="sheetBreakpoint === 'peek' ? '展开行程' : '收起到 Peek'" @click="setSheetBreakpoint(sheetBreakpoint === 'peek' ? 'half' : 'peek')"><IonIcon :icon="sheetBreakpoint === 'peek' ? chevronUpOutline : chevronDownOutline" /></button></div>
              </header>
              <nav v-if="panelMode === 'journey'" class="journey-view-tabs" aria-label="行程内容"><button type="button" :class="{ selected: journeySection === 'itinerary' }" @click="journeySection = 'itinerary'">规划点 <small>{{ visibleStops.length }}</small></button><button type="button" :class="{ selected: journeySection === 'overview' }" @click="journeySection = 'overview'">说明</button></nav>
              <div v-if="panelMode === 'journey'" class="journey-day-tabs" aria-label="行程日期"><button type="button" :class="{ selected: selectedDay === 'all' }" @click="selectJourneyDay('all')">全程</button><button v-for="(day, index) in tripDocument.days" :key="day.id" type="button" :class="{ selected: selectedDay === index + 1 }" @click="selectJourneyDay(index + 1)">D{{ index + 1 }} <small>{{ formatDate(day.date).slice(5) }}</small></button></div>

              <div v-if="panelMode === 'search'" class="panel-scroll panel-search-scroll">
                <div class="search-panel-heading"><button class="inline-back-button" type="button" @click="closeJourneySearch"><span>‹</span> 返回行程</button><span class="eyebrow">{{ locationSearchMode === 'repair' ? 'RELOCATE POINT' : searchParentStopId ? 'ADD CHILD POINT' : 'ADD A PLACE' }}</span><h2>{{ locationSearchMode === 'repair' ? '重新定位规划点' : searchParentStopId ? '添加子规划点' : '搜索地点' }}</h2><p>{{ locationSearchMode === 'repair' ? '为“' + (findPlanningPoint(locationSearchTargetID)?.title || '当前规划点') + '”查找新的坐标；名称可以一起调整。' : searchParentStopId && selectedStop ? '添加到：' + selectedStop.title : '搜索结果会保留名称、地址、坐标系和 Provider 引用。' }}</p></div>
                <form class="redesign-search-form" @submit.prevent="searchPlaces"><label>{{ locationSearchMode === 'repair' ? '搜索关键词' : '地点或关键词' }}<input v-model="searchQuery" :placeholder="locationSearchMode === 'repair' ? '可改用别名、完整地址或附近地标' : '例如：西湖、咖啡馆、观景台'" autocomplete="off" /></label><label v-if="locationSearchMode === 'repair'" class="location-search-title-field">保存名称<input v-model="locationSearchTitleDraft" maxlength="200" placeholder="保留当前名称或改成更准确的名称" autocomplete="off" /></label><label>城市/区域（可选）<input v-model="searchRegion" placeholder="例如：杭州市西湖区" autocomplete="address-level2" /></label><label class="select-field">搜索类型<UiSelect v-model="searchCategory" aria-label="搜索类型" :options="searchCategoryOptions" /></label><button class="primary-action search-submit" type="submit" :disabled="searchLoading"><IonIcon :icon="searchOutline" /> {{ searchLoading ? '搜索中…' : locationSearchMode === 'repair' ? '重新搜索候选' : '搜索地点' }}</button></form>
                <p class="search-help">{{ locationSearchMode === 'repair' ? '只会在你点击“替换位置”后写入；候选会显示完整地址、Provider 和 CRS。位置变更会清除受影响路线与该点天气。' : '先查询本地地点目录，未命中后调用当前优先 Provider；选择结果后才会保存到 Trip。' }}</p><p v-if="searchMessage" class="inline-message">{{ searchMessage }}</p>
                <div class="search-results"><article v-for="(result, index) in searchResults" :key="result.id || result.name + index" class="search-result" :class="{ selected: selectedSearchResultIndex === index }" @click="selectSearchResult(index)"><div><strong>{{ result.name }}</strong><span>{{ result.address || '地址待补充' }}</span><small v-if="result.location">{{ candidateCoordinateCRS(result) }} · {{ result.location.lat.toFixed(5) }}, {{ result.location.lng.toFixed(5) }} · {{ result.provider || 'Provider 未知' }}</small></div><div class="search-result-actions"><button class="text-action search-locate-button" type="button" @click.stop="selectSearchResult(index)">定位</button><button class="secondary-action compact-action" type="button" :disabled="actionLoading" @click.stop="applySearchResult(result)">{{ locationSearchMode === 'repair' ? '替换位置' : searchParentStopId ? '添加子点' : '添加' }}</button></div></article></div>
              </div>

              <div v-else class="panel-scroll itinerary-scroll">
                <div class="peek-summary"><div><span class="eyebrow">{{ selectedDay === 'all' ? 'FULL JOURNEY' : 'DAY ' + selectedDay }}</span><strong>{{ visibleStops.length }} 个规划点</strong></div><span>{{ formatDistance(visibleRouteSummary.distanceM) || '距离待生成' }} · {{ formatDuration(visibleRouteSummary.durationS) || '时间待生成' }}</span></div>
                <div v-if="journeySection === 'overview'" class="trip-overview redesign-overview"><div class="section-title-row"><div><span class="eyebrow">JOURNEY NOTE</span><h2>行程说明</h2></div><div v-if="!readOnlyView" class="section-actions"><button class="text-action" type="button" @click="beginEditTripDescription">{{ tripDescriptionEditing ? '编辑中' : '编辑' }}</button><button v-if="tripDescriptionEditing" class="text-action" type="button" @click="openTripDescriptionFullscreen">全屏</button></div></div><template v-if="tripDescriptionEditing && !readOnlyView"><MarkdownEditor v-model="tripDescriptionDraft" v-model:mode="tripDescriptionEditorMode" :preview-html="renderMarkdown(tripDescriptionDraft)" :rows="4" editor-label="MARKDOWN" preview-label="行程说明预览" editor-aria-label="行程说明 Markdown 原始文本" placeholder="补充整个行程的背景、节奏和注意事项" /><div class="editor-actions"><button class="secondary-action compact-action" type="button" @click="cancelEditTripDescription">取消</button><button class="primary-action compact-action" type="button" :disabled="tripDescriptionSaving" @click="saveTripDescription">{{ tripDescriptionSaving ? '保存中…' : '保存说明' }}</button></div></template><div v-else-if="tripDocument.description_markdown" class="markdown" v-html="renderMarkdown(tripDocument.description_markdown)"></div><p v-else class="muted">{{ shareMode ? '暂无行程总体说明。' : '暂无行程总体说明，点击“编辑”添加。' }}</p></div>
                <div v-else class="itinerary-section"><div class="section-title-row"><div><span class="eyebrow">ITINERARY</span><h2>规划点</h2></div><div class="section-actions"><span>{{ visibleStops.length }} 个</span><button v-if="!readOnlyView" class="text-action" type="button" :class="{ selected: reorderMode }" @click="toggleReorderMode">{{ reorderMode ? '完成排序' : '调整顺序' }}</button></div></div>
                  <div v-if="!readOnlyView" class="redesign-plan-controls"><label class="select-field">路线 Provider<UiSelect v-model="planningProvider" aria-label="路线 Provider" :options="mapProviderOptions" /></label><label class="select-field">出行方式<UiSelect v-model="planningMode" aria-label="出行方式" :options="travelModeOptions" /></label><label v-if="planningMode === 'driving' && supportsDrivingStrategy" class="select-field">驾车策略<UiSelect v-model="planningStrategy" aria-label="驾车策略" :options="availableDrivingStrategyOptions" /></label><button class="primary-action compact-action plan-button" type="button" :disabled="planningLoading || !canPlanRoutes" @click="planRoutes"><IonIcon :icon="navigateOutline" /> {{ planningLoading ? '规划中…' : '生成路线' }}</button></div>
                  <div v-if="unlocatedPlanningPoints.length" class="location-readiness-banner"><div><strong>{{ unlocatedPlanningPoints.length }} 个规划点待定位</strong><span v-if="unlocatedMainStops.length">主规划点没有可靠坐标前，路线和导航不会启用。</span><span v-else>子规划点不参与主路线，但仍建议补充坐标。</span></div><button v-if="!readOnlyView" class="text-action" type="button" @click="selectPlanningPointFromList(unlocatedMainStops[0] || unlocatedPlanningPoints[0])">去定位</button><span v-else class="location-readiness-readonly">只读</span></div><div class="redesign-route-summary"><div><span>{{ selectedDay === 'all' ? '全程路线' : 'D' + selectedDay + ' 当天路线' }}</span><strong v-if="visibleRouteSummary.segments">{{ formatDistance(visibleRouteSummary.distanceM) || '距离未知' }} · {{ formatDuration(visibleRouteSummary.durationS) || '时间未知' }}</strong><em v-else-if="visibleRouteSummary.zeroSegments">有 {{ visibleRouteSummary.zeroSegments }} 段为同一地点</em><em v-else>尚未生成路线</em></div><small v-if="visibleRouteSummary.segments">{{ visibleRouteSummary.segments }} 段 · {{ mapProviderLabel }}</small></div>
                  <p v-if="hasCarryOverRoute" class="route-hint">路线从前一天最后一个规划点“{{ carryOverStop?.title }}”开始。</p><p v-if="reorderMessage" class="inline-message">{{ reorderMessage }}</p><p v-if="pointUpdateNotice" class="inline-message">{{ pointUpdateNotice }}</p><p v-if="!plannableDays.length" class="muted">{{ shareMode ? '当前选择范围暂无可生成的路线。' : '添加至少两个相邻的带坐标规划点后，可以生成路线。' }}</p>
                  <div v-if="visibleStops.length" class="redesign-stop-list"><article v-for="stop in visibleStops" :key="stop.id" class="redesign-stop-row" :class="{ selected: selectedStopId === stop.id, 'reorder-active': reorderMode, 'location-missing': !pointFor(stop) }"><button class="redesign-stop-main" type="button" @click="selectPlanningPointFromList(stop)"><span class="stop-number">{{ stop.sequence }}</span><span><strong>{{ stop.title }}</strong><small>{{ stopDate(stop) }} · {{ stop.address || '地址待补充' }}</small><em class="stop-location-badge" :class="{ missing: !pointFor(stop) }">{{ locationStatus(stop) }}</em></span><span class="row-chevron">›</span></button><div v-if="reorderMode && !readOnlyView" class="reorder-actions" @click.stop><button class="reorder-move-button" type="button" :disabled="actionLoading || !canMovePlanningPoint(stop, -1)" :aria-label="'上移规划点 ' + stop.title" @click="movePlanningPoint(stop, -1)"><IonIcon :icon="chevronUpOutline" /></button><button class="reorder-move-button" type="button" :disabled="actionLoading || !canMovePlanningPoint(stop, 1)" :aria-label="'下移规划点 ' + stop.title" @click="movePlanningPoint(stop, 1)"><IonIcon :icon="chevronDownOutline" /></button></div><button v-if="!readOnlyView" class="stop-delete-button" type="button" :aria-label="'删除规划点 ' + stop.title" @click.stop="deletePlanningPoint(stop)">×</button></article></div><p v-else class="muted compact-empty">当前日期还没有规划点。</p>
                  <button v-if="!readOnlyView" class="add-place-action" type="button" @click="openJourneySearch()"><IonIcon :icon="searchOutline" /> 搜索并添加规划点</button>
                </div>
                <div v-if="!readOnlyView" class="panel-data-actions"><button type="button" @click="openImportPicker">导入</button><button type="button" :disabled="actionLoading" @click="downloadTrip">导出 JSON</button><button type="button" :disabled="actionLoading" @click="createShare">在线分享</button></div>
              </div>
            </aside>

            <aside v-if="selectedStop" class="details-drawer stop-detail-panel" :class="['sheet-' + sheetBreakpoint, { 'is-child-detail': Boolean(selectedSubStop), 'is-sheet-dragging': sheetDragActive }]" :style="sheetDragStyle" aria-label="规划点详情">
              <div class="detail-sheet-handle"><button type="button" :aria-label="sheetBreakpoint === 'expanded' ? '收起规划点详情到最低' : sheetBreakpoint === 'peek' ? '展开规划点详情到半屏' : '收起规划点详情到最低'" @pointerdown="startSheetDrag" @click="cycleSheetBreakpoint"><span></span></button></div>
              <div class="detail-scroll redesign-detail-scroll">
                <header class="detail-topbar"><button type="button" class="detail-back-button" @click="selectedSubStop ? navigateBackFromSubStop() : navigateBackFromStop()"><span>‹</span>{{ selectedSubStop ? '主规划点' : selectedDay === 'all' ? '行程' : 'D' + selectedDay + ' 行程' }}</button><div class="detail-topbar-actions"><button class="detail-sheet-toggle" type="button" :aria-label="sheetBreakpoint === 'expanded' ? '收起到半屏' : sheetBreakpoint === 'peek' ? '展开到半屏' : '展开规划点详情'" @click="toggleDetailSheet"><IonIcon :icon="sheetBreakpoint === 'expanded' ? chevronDownOutline : chevronUpOutline" /></button><div v-if="!readOnlyView" class="detail-more-wrap"><button type="button" class="detail-more-button" :aria-expanded="detailMoreOpen" aria-label="规划点更多操作" @click.stop="toggleDetailMore">⋯</button><div v-if="detailMoreOpen" class="detail-more-menu" role="menu"><button type="button" role="menuitem" @click="editSelectedDescriptionFromMenu">编辑规划点</button><button type="button" role="menuitem" @click="editSelectedContentFromMenu">编辑说明与时间</button><button type="button" role="menuitem" class="danger-menu-item" @click="deleteSelectedPointFromMenu">删除{{ selectedSubStop ? '子规划点' : '规划点' }}</button></div></div></div></header>
                <p class="detail-kicker"><span>{{ selectedSubStop ? 'SUB-STOP ' + selectedSubStop.sequence : 'STOP ' + selectedStop.sequence }}</span><span>{{ selectedTarget?.kind || '规划点' }}</span></p>
                <h1>{{ selectedTarget?.title }}</h1><p class="detail-address">{{ selectedTarget?.address || '地址待解析' }}</p><div class="detail-date-row"><p class="detail-date">{{ stopDate(selectedTarget || selectedStop) }}<span v-if="stopTime(selectedTarget || selectedStop)"> · {{ stopTime(selectedTarget || selectedStop) }}</span></p><button v-if="!readOnlyView && !selectedSubStop" class="text-action detail-date-edit" type="button" @click="beginEditStopDate">修改日期</button><span v-if="selectedSubStop" class="detail-date-follow-note">跟随主规划点</span></div><div v-if="stopDateEditing && !selectedSubStop && !readOnlyView" class="detail-date-editor"><label class="select-field">移动到日期<UiSelect v-model="stopDateDraftDayID" aria-label="规划点目标日期" :options="tripDayOptions" /></label><div class="editor-actions"><button class="secondary-action compact-action" type="button" @click="cancelEditStopDate">取消</button><button class="primary-action compact-action" type="button" :disabled="stopDateSaving" @click="saveStopDate">{{ stopDateSaving ? '保存中…' : '保存日期' }}</button></div></div>
                <div class="detail-location" :class="{ 'location-missing': !pointFor(selectedTarget || selectedStop) }"><div class="detail-location-heading"><span><span class="location-status-icon" :class="{ missing: !pointFor(selectedTarget || selectedStop) }" aria-hidden="true">{{ pointFor(selectedTarget || selectedStop) ? '●' : '!' }}</span>{{ locationStatus(selectedTarget || selectedStop) }}</span><span class="location-state-label">{{ pointFor(selectedTarget || selectedStop) ? '可用于路线与导航' : '需要处理' }}</span></div><small v-if="pointFor(selectedTarget || selectedStop)">{{ pointFor(selectedTarget || selectedStop)?.crs }} · {{ pointFor(selectedTarget || selectedStop)?.lat.toFixed(6) }}, {{ pointFor(selectedTarget || selectedStop)?.lng.toFixed(6) }}</small><small v-else>暂无可靠坐标，路线和导航暂不可用。</small><small v-if="pointFor(selectedTarget || selectedStop)">来源：{{ locationSource(selectedTarget || selectedStop) }}</small><div v-if="!readOnlyView" class="detail-location-actions"><button class="text-action" type="button" @click="beginEditPoint">编辑名称/地址</button><button class="text-action" type="button" @click="openPointSearch(selectedTarget || selectedStop)">重新搜索</button><button class="text-action" type="button" :disabled="!mapReady" @click="startMapPickForPoint(selectedTarget || selectedStop)">地图选点</button></div></div>
                <div class="detail-primary-actions"><button class="detail-navigation-button" type="button" :disabled="!pointFor(selectedTarget || selectedStop)" @click="openNavigation('amap')"><IonIcon :icon="navigateOutline" /> 高德导航</button><button class="detail-navigation-button" type="button" :disabled="!pointFor(selectedTarget || selectedStop)" @click="openNavigation('baidu')"><IonIcon :icon="navigateOutline" /> 百度导航</button></div>
                <div class="detail-weather"><IonIcon :icon="sunnyOutline" /><span><strong>{{ weatherText(selectedTarget || selectedStop) }}</strong><small v-if="weatherUpdatedAt(selectedTarget || selectedStop)">更新于 {{ weatherUpdatedAt(selectedTarget || selectedStop) }}</small></span><button v-if="!readOnlyView" type="button" :disabled="weatherLoading || !pointFor(selectedTarget || selectedStop)" @click="refreshWeather">{{ weatherLoading ? '查询中…' : '刷新' }}</button></div>
                <section v-if="!selectedSubStop" class="detail-section"><div class="section-title-row"><h2>子规划点 <span>{{ selectedStop.children?.length || 0 }}</span></h2><button v-if="!readOnlyView" class="text-action" type="button" @click="openChildSearch(selectedStop)">添加</button></div><p v-if="selectedStop.children?.length" class="detail-section-help">点击子点进入下一层，返回箭头会回到主规划点。</p><div v-if="selectedStop.children?.length" class="detail-child-list"><button v-for="child in selectedStop.children" :key="child.id" type="button" class="detail-child-row" @click="selectSubStop(child, selectedStop)"><span class="child-number">{{ child.sequence }}</span><span><strong>{{ child.title }}</strong><small>{{ stopDate(child) }} · {{ child.address || '地址待补充' }}</small><em class="stop-location-badge" :class="{ missing: !pointFor(child) }">{{ locationStatus(child) }}</em></span><span>›</span></button></div><button v-if="!readOnlyView" class="add-place-action" type="button" @click="openChildSearch(selectedStop)"><IonIcon :icon="searchOutline" /> 添加子规划点</button></section>
                <button v-else class="detail-parent-button" type="button" @click="navigateBackFromSubStop">‹ 返回主规划点：{{ selectedStop.title }}</button>
                <section class="detail-section"><div class="section-title-row"><h2>地点说明与时间</h2><div v-if="!readOnlyView" class="section-actions"><button class="text-action" type="button" @click="beginEditDescription">{{ descriptionEditing ? '编辑中' : '编辑规划点' }}</button><button v-if="descriptionEditing" class="text-action" type="button" @click="openDescriptionFullscreen">全屏</button></div></div><template v-if="descriptionEditing && !readOnlyView"><div class="detail-time-editor"><div class="detail-time-editor-heading"><strong>时间窗口</strong><small>到达和离开时间均为可选，留空表示未设置</small></div><div class="detail-time-fields"><label>到达<input v-model="arrivalTimeDraft" type="time" /></label><label>离开<input v-model="departureTimeDraft" type="time" /></label></div></div><MarkdownEditor v-model="descriptionDraft" v-model:mode="descriptionEditorMode" :preview-html="renderMarkdown(descriptionDraft)" :rows="7" editor-label="MARKDOWN" preview-label="地点说明预览" editor-aria-label="地点说明 Markdown 原始文本" placeholder="补充门票、开放时间、行程备注等信息" /><div class="editor-actions"><button class="secondary-action compact-action" type="button" @click="cancelEditDescription">取消</button><button class="primary-action compact-action" type="button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存规划点' }}</button></div></template><div v-else-if="selectedTarget?.description_markdown" class="markdown" v-html="renderMarkdown(selectedTarget.description_markdown)"></div><p v-else class="muted">{{ shareMode ? '暂无地点说明。' : '暂无地点说明，点击“编辑说明与时间”添加。' }}</p></section>
                <button v-if="!readOnlyView" class="detail-danger-button" type="button" @click="deletePlanningPoint(selectedTarget || selectedStop)">删除{{ selectedSubStop ? '子规划点' : '规划点' }}</button>
              </div>
            </aside>
          </section>
        </main>
      </div>

      <div v-if="pointEditorOpen && !readOnlyView" class="modal-backdrop point-editor-backdrop" @click.self="cancelEditPoint"><section class="modal-panel point-editor-panel" role="dialog" aria-modal="true" aria-labelledby="point-editor-title" aria-describedby="point-editor-description"><header class="point-editor-header"><div><p class="eyebrow">POINT EDITOR</p><h2 id="point-editor-title">编辑规划点</h2><p id="point-editor-description">名称、地址和坐标分开处理；不会因为改名而替换位置。</p></div><button class="modal-close" type="button" :disabled="pointEditorSaving" aria-label="关闭规划点编辑" @click="cancelEditPoint">×</button></header><form id="point-editor-form" class="point-editor-form" @submit.prevent="savePointDetails"><label>规划点名称<input ref="pointEditorTitleInput" v-model="pointEditorTitleDraft" maxlength="200" required placeholder="例如：西湖断桥" /></label><label>地址或补充定位线索<input v-model="pointEditorAddressDraft" maxlength="500" placeholder="用于确认候选，不会自动猜坐标" /></label><section class="point-editor-location" :class="{ missing: !pointEditorPoint() }"><div class="point-editor-location-head"><div><strong>{{ pointEditorLocationStatus() }}</strong><small v-if="pointEditorPoint()">{{ pointEditorPoint()?.crs }} · {{ pointEditorPoint()?.lat.toFixed(6) }}, {{ pointEditorPoint()?.lng.toFixed(6) }}</small><small v-else>没有可靠坐标，路线与导航暂不可用。</small></div><span class="location-state-label">{{ pointEditorPoint() ? '已保存' : '待处理' }}</span></div><small v-if="pointEditorPoint()">来源：{{ pointEditorLocationSource() }}</small><p v-else>请从候选中选择一个明确地点，或在地图上点击准确位置。不会根据数字外观补造 CRS。</p><div class="point-editor-location-actions"><button class="secondary-action compact-action" type="button" @click="openPointSearchFromEditor">重新搜索候选</button><button class="secondary-action compact-action" type="button" :disabled="!mapReady" @click="startMapPickFromEditor">地图选点更新</button></div></section><p class="point-editor-note">重新搜索或地图选点会清除受影响的路线和该点天气；保存名称和地址本身不会改变路线。</p><p v-if="error" class="point-editor-error" role="alert">{{ error }}</p><div class="point-editor-related"><span><strong>更多编辑</strong><small>说明、时间窗口、日期、顺序和删除仍在详情页中管理。</small></span><button class="text-action" type="button" @click="beginEditDescriptionFromPointEditor">编辑说明与时间</button></div></form><div class="modal-actions point-editor-actions"><button type="button" :disabled="pointEditorSaving" @click="cancelEditPoint">取消</button><button class="primary" type="submit" form="point-editor-form" :disabled="pointEditorSaving || !pointEditorTitleDraft.trim()">{{ pointEditorSaving ? '保存中…' : '保存名称和地址' }}</button></div></section></div>
      <div v-if="descriptionFullscreen && descriptionEditing && !readOnlyView" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-description-title"><header><h2 id="fullscreen-description-title">编辑规划点信息</h2><button class="modal-close" type="button" aria-label="退出全屏编辑" @click="closeDescriptionFullscreen">×</button></header><MarkdownEditor class="fullscreen-markdown-editor" v-model="descriptionDraft" v-model:mode="descriptionEditorMode" :preview-html="renderMarkdown(descriptionDraft)" :rows="12" editor-label="MARKDOWN" preview-label="地点说明预览" editor-aria-label="地点说明 Markdown 原始文本" placeholder="补充门票、开放时间、行程备注等信息" /><div class="description-actions"><button class="text-button" type="button" @click="cancelEditDescription">取消</button><button class="primary-text-button" type="button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存规划点' }}</button></div></section></div>
      <div v-if="tripDescriptionFullscreen && tripDescriptionEditing && !readOnlyView" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-trip-description-title"><header><h2 id="fullscreen-trip-description-title">编辑行程总体说明</h2><button class="modal-close" type="button" aria-label="退出全屏编辑" @click="closeTripDescriptionFullscreen">×</button></header><MarkdownEditor class="fullscreen-markdown-editor" v-model="tripDescriptionDraft" v-model:mode="tripDescriptionEditorMode" :preview-html="renderMarkdown(tripDescriptionDraft)" :rows="12" editor-label="MARKDOWN" preview-label="行程说明预览" editor-aria-label="行程说明 Markdown 原始文本" placeholder="补充整个行程的背景、节奏和注意事项" /><div class="description-actions"><button class="text-button" type="button" @click="cancelEditTripDescription">取消</button><button class="primary-text-button" type="button" :disabled="tripDescriptionSaving" @click="saveTripDescription">{{ tripDescriptionSaving ? '保存中…' : '保存说明' }}</button></div></section></div>
      <div v-if="mapPickOpen" class="modal-backdrop" @click.self="cancelMapPick"><section class="modal-panel map-pick-panel" role="dialog" aria-modal="true" aria-labelledby="map-pick-title"><button class="modal-close" aria-label="取消地图选点" @click="cancelMapPick">×</button><p class="eyebrow">MAP PICK</p><h2 id="map-pick-title">{{ mapPickTargetID ? '更新规划点位置' : '保存地图选点' }}</h2><p class="map-pick-coordinate">{{ mapPickLocation?.crs }} · {{ mapPickLocation?.lat.toFixed(6) }}, {{ mapPickLocation?.lng.toFixed(6) }}</p><p class="map-pick-context">{{ mapPickTargetID ? '点击保存后会替换当前坐标，并清除受影响的路线和天气。' : '点击地图得到坐标后，再确认名称和日期。' }}</p><label>地点名称<input v-model="mapPickTitle" required autofocus placeholder="例如：临时观景点" /></label><label>地址或备注（可选）<input v-model="mapPickAddress" placeholder="补充位置说明" /></label><label v-if="!mapPickTargetID" class="select-field">加入日期<UiSelect v-model="mapPickDayID" aria-label="加入日期" :options="tripDayOptions" /></label><p v-if="error" class="modal-form-error" role="alert">{{ error }}</p><div class="modal-actions"><button type="button" @click="cancelMapPick">取消</button><button type="button" class="primary" :disabled="actionLoading || !mapPickTitle.trim()" @click="saveMapPick">{{ actionLoading ? '保存中…' : mapPickTargetID ? '更新规划点' : '保存规划点' }}</button></div></section></div>
      <div v-if="tripDetailsEditing && !readOnlyView" class="modal-backdrop trip-details-backdrop" @click.self="cancelEditTripDetails">
        <section class="modal-panel trip-details-panel" role="dialog" aria-modal="true" aria-labelledby="trip-details-title">
          <header class="trip-details-header"><div><p class="eyebrow">TRIP DETAILS</p><h2 id="trip-details-title">编辑行程信息</h2><p>名称和日期会作为一次更改保存。</p></div><button class="modal-close" type="button" :disabled="tripDetailsSaving" aria-label="关闭编辑行程信息" @click="cancelEditTripDetails">×</button></header>
          <form class="trip-details-form" @submit.prevent="saveTripDetails">
            <div class="trip-details-field-head"><label>行程名称<input ref="tripDetailsTitleInput" v-model="tripDetailsTitleDraft" maxlength="120" required placeholder="例如：杭州春日慢游" /></label><span>{{ tripDetailsTitleCount }}/120</span></div>
            <div class="trip-details-date-grid"><label>开始日期<input v-model="tripDetailsStartDateDraft" type="date" required /></label><label>结束日期<input v-model="tripDetailsEndDateDraft" type="date" :min="tripDetailsStartDateDraft" required /></label></div>
            <div class="trip-details-duration"><strong v-if="tripDetailsDayCount > 0">共 {{ tripDetailsDayCount }} 天</strong><strong v-else>日期范围待确认</strong><span>日期按本地日历计算，最多支持 60 天</span></div>
            <p v-if="tripDetailsDateError" class="trip-details-error" role="alert">{{ tripDetailsDateError }}</p>
            <div v-if="tripDetailsBlockingDays.length" class="trip-details-error" role="alert"><strong>不能缩短到当前日期范围</strong><span v-for="day in tripDetailsBlockingDays" :key="day.id">{{ formatDate(day.date) }} 仍有 {{ planningPointCount(day) }} 个规划点。</span><small>请先移动这些规划点，或恢复结束日期。</small></div>
            <p v-if="tripDetailsDateHint" class="trip-details-hint"><IonIcon :icon="createOutline" /> {{ tripDetailsDateHint }}</p>
            <p v-if="tripDetailsDateChanged" class="trip-details-note">日期变化后，受影响规划点的天气快照会清除；已有路线不会自动重新规划。</p>
            <div class="modal-actions"><button type="button" :disabled="tripDetailsSaving" @click="cancelEditTripDetails">取消</button><button class="primary" type="submit" :disabled="!tripDetailsCanSave">{{ tripDetailsSaving ? '保存中…' : '保存更改' }}</button></div>
          </form>
        </section>
      </div>
      <div v-if="historyOpen && !shareMode" class="modal-backdrop trip-history-backdrop" @click.self="historyOpen = false">
        <section class="modal-panel trip-history-panel" role="dialog" aria-modal="true" aria-labelledby="trip-history-title">
          <header class="trip-history-header"><div><p class="eyebrow">VERSION HISTORY</p><h2 id="trip-history-title">版本历史</h2><p>普通编辑不会自动记录，只有你主动保存的当前版本才会出现在这里。</p></div><button class="modal-close" type="button" aria-label="关闭版本历史" @click="historyOpen = false">×</button></header>
          <div class="trip-history-current"><div><span class="eyebrow">CURRENT VERSION</span><strong>{{ selected?.title || tripDocument?.title || '当前行程' }}</strong><small>{{ historyDateRange || '当前日期范围' }} · 工作版本 {{ selected?.revision || '—' }}</small></div><span class="history-current-tag">当前</span></div>
          <form class="trip-history-save-form" @submit.prevent="saveTripHistory"><label>版本说明 <span>可选</span><input v-model="historyLabelDraft" maxlength="120" placeholder="例如：出发前最终版" /></label><button class="primary-action" type="submit" :disabled="historySaving || !selected"><span v-if="historySaving">保存中…</span><span v-else>保存当前版本</span></button></form>
          <p v-if="historyError" class="trip-history-error" role="alert">{{ historyError }}</p>
          <p v-if="historyMessage" class="trip-history-message" role="status">{{ historyMessage }}</p>
          <div v-if="historyLoading" class="trip-history-loading"><span class="loading-dot"></span><span>正在读取版本历史…</span></div>
          <div v-else-if="!historyEntries.length" class="trip-history-empty"><span class="history-empty-mark">↶</span><strong>还没有历史版本</strong><p>保存当前状态后，可以随时回来查看它。删除历史版本不会影响当前行程。</p></div>
          <div v-else class="trip-history-list">
            <article v-for="version in historyEntries" :key="version.history_id || version.id" class="trip-history-item"><div class="trip-history-item-main"><strong>{{ version.label || '保存于 ' + formatDateTime(version.created_at) }}</strong><span>{{ formatDateRange(version.start_date, version.end_date) }} · 工作版本 {{ version.source_revision }}</span><small>{{ version.title }} · {{ formatDateTime(version.created_at) }}</small></div><div class="trip-history-item-actions"><button class="secondary-action compact-action" type="button" :disabled="historyLoading" @click="viewTripHistory(version)">查看</button><button class="danger-text-action" type="button" :disabled="historyDeletingID === (version.history_id || version.id)" @click="deleteTripHistory(version)">{{ historyDeletingID === (version.history_id || version.id) ? '删除中…' : '删除' }}</button></div></article>
          </div>
        </section>
      </div>
      <div v-if="newTripOpen" class="new-trip-backdrop" @click.self="newTripOpen = false">
        <section class="new-trip-window" role="dialog" aria-modal="true" aria-labelledby="new-trip-title">
          <aside class="new-trip-hero"><span class="new-trip-mark">✦</span><p class="eyebrow">START A JOURNEY</p><h2>把下一段路，<br />放到地图上。</h2><p>先建立一个轻量的行程容器，之后再逐日添加地点、说明和路线。</p><div class="new-trip-hero-orbit"></div></aside>
          <div class="new-trip-form-area"><header class="new-trip-header"><div><p class="eyebrow">NEW JOURNEY</p><h2 id="new-trip-title">新建旅行规划</h2><p>创建后会先保存为草稿，你可以稍后继续完善。</p></div><button class="settings-close" type="button" aria-label="关闭新建行程" @click="newTripOpen = false">×</button></header><div class="new-trip-steps"><span class="active"><b>01</b> 基本信息</span><span><b>02</b> 规划地点</span><span><b>03</b> 生成路线</span></div><form class="new-trip-form" @submit.prevent="createTrip"><label>规划名称<input v-model="newTitle" maxlength="120" required placeholder="例如：甘南自驾" /></label><div class="new-trip-date-grid"><label>开始日期<input v-model="newStartDate" type="date" required /></label><label>结束日期<input v-model="newEndDate" type="date" required /></label></div><label>时区<input v-model="newTimezone" placeholder="Asia/Shanghai" required /></label><label>总体说明 <span class="optional-label">可选 · 支持 Markdown</span><textarea v-model="newDescription" rows="5" placeholder="写下这次旅行的背景、节奏和注意事项"></textarea></label><div class="new-trip-actions"><button class="secondary-action" type="button" @click="newTripOpen = false">取消</button><button class="primary-action" type="submit" :disabled="actionLoading"><IonIcon :icon="addOutline" /> {{ actionLoading ? '创建中…' : '创建草稿' }}</button></div></form></div>
        </section>
      </div>
      <div v-if="false && newTripOpen" class="modal-backdrop" @click.self="newTripOpen = false"><section class="modal-panel" role="dialog" aria-modal="true" aria-labelledby="new-trip-title"><button class="modal-close" aria-label="关闭" @click="newTripOpen = false">×</button><p class="eyebrow">NEW JOURNEY</p><h2 id="new-trip-title">新建旅行规划</h2><form @submit.prevent="createTrip"><label>规划名称<input v-model="newTitle" maxlength="120" required /></label><div class="form-grid"><label>开始日期<input v-model="newStartDate" type="date" required /></label><label>结束日期<input v-model="newEndDate" type="date" required /></label></div><label>时区<input v-model="newTimezone" placeholder="Asia/Shanghai" required /></label><label>总体说明（Markdown）<textarea v-model="newDescription" rows="5" placeholder="写下这次旅行的总体说明"></textarea></label><div class="modal-actions"><button type="button" @click="newTripOpen = false">取消</button><button class="primary" type="submit" :disabled="actionLoading">创建草稿</button></div></form></section></div>
      <div v-if="settingsOpen" class="settings-backdrop" @click.self="settingsOpen = false">
        <section class="settings-window" role="dialog" aria-modal="true" aria-labelledby="settings-title">
          <header class="settings-header">
            <div><p class="eyebrow">JOURNEYIN / SETTINGS</p><h2 id="settings-title">设置</h2><p>把服务连接、地图能力和外观偏好集中到一个设置工作区。</p></div>
            <button class="settings-close" type="button" aria-label="关闭设置" @click="settingsOpen = false">×</button>
          </header>
          <div class="settings-layout">
            <nav class="settings-nav" aria-label="设置分区">
              <button type="button" :class="{ active: settingsSection === 'appearance' }" @click="settingsSection = 'appearance'"><span class="settings-nav-icon">☼</span><span>外观</span><small>主题与阅读</small></button>
              <button type="button" :class="{ active: settingsSection === 'connection' }" @click="settingsSection = 'connection'"><span class="settings-nav-icon">↗</span><span>连接</span><small>服务与令牌</small></button>
              <button type="button" :class="{ active: settingsSection === 'maps' }" @click="settingsSection = 'maps'"><span class="settings-nav-icon">⌖</span><span>地图</span><small>Provider 与 Key</small></button>
              <button type="button" :class="{ active: settingsSection === 'search' }" @click="settingsSection = 'search'"><span class="settings-nav-icon">⌕</span><span>地点检索</span><small>搜索优先级</small></button>
              <button type="button" :class="{ active: settingsSection === 'sharing' }" @click="settingsSection = 'sharing'"><span class="settings-nav-icon">↗</span><span>分享</span><small>链接与权限</small></button>
              <button type="button" :class="{ active: settingsSection === 'mcp' }" @click="settingsSection = 'mcp'"><span class="settings-nav-icon">◇</span><span>MCP</span><small>Agent 连接</small></button>
              <button type="button" :class="{ active: settingsSection === 'about' }" @click="settingsSection = 'about'"><span class="settings-nav-icon">ⓘ</span><span>关于</span><small>版本与项目</small></button>
            </nav>
            <div class="settings-body">
              <section v-if="settingsSection === 'appearance'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">APPEARANCE</span><h3>外观与主题</h3><p>主题默认跟随系统，也可以在这里固定为浅色或深色。</p></div>
                <div class="settings-choice-grid"><button type="button" :class="{ selected: theme === 'system' }" @click="setTheme('system')"><strong>跟随系统</strong><small>根据设备的浅色/深色偏好自动切换</small></button><button type="button" :class="{ selected: theme === 'light' }" @click="setTheme('light')"><strong>浅色</strong><small>适合白天规划和桌面编辑</small></button><button type="button" :class="{ selected: theme === 'dark' }" @click="setTheme('dark')"><strong>深色</strong><small>降低夜间地图工作区的视觉亮度</small></button></div>
                <div class="settings-note"><span class="settings-note-mark">i</span><span>当前主题：<strong>{{ themeLabel }}</strong>。地图 Provider 的底图会保持各自的官方样式，JourneyIn 只对周围的工作区控件做主题适配。</span></div>
              </section>

              <section v-else-if="settingsSection === 'connection'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">CONNECTION</span><h3>服务端连接</h3><p>管理当前 JourneyIn 服务地址和兼容 API Token。</p></div>
                <div class="settings-form-grid"><label>当前服务地址<input v-model="serverURL" readonly /></label><label>兼容 REST API Token<input v-model="authTokenInput" type="password" placeholder="仅用于兼容旧客户端，可留空" autocomplete="off" /></label></div>
                <div class="settings-actions"><button class="secondary-action" type="button" @click="logout">清除令牌</button><button class="primary-action" type="button" @click="saveAuth">保存令牌</button></div>
                <p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>

              <section v-else-if="settingsSection === 'maps'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">MAP PROVIDERS</span><h3>地图与路线</h3><p>选择默认 Provider，并分别管理浏览器端和服务端能力。</p></div>
                <div class="settings-card"><div class="settings-card-heading"><div><strong>默认地图 Provider</strong><small>用于没有单独地图偏好的新行程</small></div><span class="settings-status-dot"></span></div><label class="select-field">默认地图 Provider<UiSelect v-model="defaultMapProvider" aria-label="默认地图 Provider" :options="mapProviderOptions" /></label><p class="settings-help">单个行程已保存的地图偏好不会被覆盖；地图工作区仍可临时切换底图。</p><button class="primary-action" type="button" :disabled="settingsSaving" @click="saveDefaultMapProvider">{{ settingsSaving ? '保存中…' : '保存默认地图' }}</button></div>
                <div class="settings-provider-grid"><article class="settings-card provider-card"><div class="settings-card-heading"><div><strong>百度地图</strong><small>JSAPI 4.0 / Web Service</small></div><span class="provider-status">{{ baiduKey ? '浏览器已配置' : '待配置' }}</span></div><p class="settings-status-line">浏览器端 Key：<strong>{{ baiduKey ? '已配置' : '未配置' }}</strong><br />服务端 Key：<strong>{{ settingsData?.map?.baidu?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>浏览器端 Key<input v-model="baiduBrowserKeyInput" type="password" :placeholder="settingsData?.map?.baidu?.browser_key_configured ? '已配置，输入新 Key 可替换' : '用于浏览器端地图'" autocomplete="off" /></label><label>服务端 Key<input v-model="baiduServerKeyInput" type="password" placeholder="留空保持当前值" autocomplete="off" /></label><a href="https://lbsyun.baidu.com/apiconsole/key" target="_blank" rel="noopener noreferrer">申请/管理百度 Key ↗</a></article><article class="settings-card provider-card"><div class="settings-card-heading"><div><strong>高德地图</strong><small>JS API 2.0 / Web Service</small></div><span class="provider-status">{{ settingsData?.map?.amap?.js_key_configured ? 'JS 已配置' : '待配置' }}</span></div><p class="settings-status-line">JS Key：<strong>{{ settingsData?.map?.amap?.js_key_configured ? '已配置' : '未配置' }}</strong><br />服务端 Key：<strong>{{ settingsData?.map?.amap?.server_key_configured ? '已配置' : '未配置' }}</strong><br />安全密钥：<strong>{{ settingsData?.map?.amap?.security_js_code_configured ? '已配置' : '未配置' }}</strong></p><label>JS Key<input v-model="amapJSKeyInput" type="password" placeholder="用于浏览器端地图" autocomplete="off" /></label><label>服务端 Key<input v-model="amapServerKeyInput" type="password" placeholder="留空保持当前值" autocomplete="off" /></label><label>JS 安全密钥<input v-model="amapSecurityJSCodeInput" type="password" placeholder="用于安全代理" autocomplete="off" /></label><a href="https://console.amap.com/dev/key/app" target="_blank" rel="noopener noreferrer">申请/管理高德 Key ↗</a></article></div><p class="settings-help">保存地图 Key 到数据库后，浏览器端 Key 会立即生效；服务端 Key 和安全密钥只返回配置状态，不会回显原文。</p><div class="settings-actions"><button class="primary-action" type="button" :disabled="settingsSaving" @click="saveMapKeys">{{ settingsSaving ? '保存中…' : '保存地图 Key 到数据库' }}</button></div><p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>

              <section v-else-if="settingsSection === 'search'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">PLACE SEARCH</span><h3>地点检索</h3><p>调整搜索 Provider 优先级，管理本地地点目录缓存。</p></div>
                <div class="settings-card"><div class="settings-card-heading"><div><strong>搜索优先级</strong><small>未命中本地目录后使用所选 Provider</small></div><span class="provider-status">{{ poiProviderPriority === 'amap' ? '高德优先' : '百度优先' }}</span></div><label class="select-field">优先 Provider<UiSelect v-model="poiProviderPriority" aria-label="地点检索优先 Provider" :options="poiPriorityOptions" /></label><p class="settings-help">Provider 不可用时会自动尝试另一家；新搜索结果只保留 7 天。</p><div class="settings-cache-row"><span>本地地点记录</span><strong>{{ localDirectoryCount }} 条</strong></div><div class="settings-actions"><button class="primary-action" type="button" @click="savePOIPreferences">保存检索优先级</button><button class="secondary-action" type="button" @click="clearLocalDirectory">清除本地记录</button></div></div><p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>

              <section v-else-if="settingsSection === 'sharing'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">ONLINE SHARING</span><h3>在线分享</h3><p>集中管理当前行程的只读分享链接；分享状态不会遮挡地图。</p></div>
                <div class="settings-card settings-share-card"><div class="settings-card-heading"><div><strong>{{ selected?.title || '当前行程' }}</strong><small>持有链接即可查看当前行程快照</small></div><span class="provider-status">{{ shareURL ? '已分享' : '未分享' }}</span></div><template v-if="selected"><a v-if="shareURL" class="settings-share-url" :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a><p v-if="shareExpiresAt" class="settings-share-expiry">有效期至 {{ formatDateTime(shareExpiresAt) }}</p><p v-if="!shareURL" class="settings-help">当前行程还没有在线分享；点击下方按钮创建一个只读链接。</p><div class="settings-actions"><button v-if="shareURL" class="primary-action" type="button" @click="copyShareURL">复制链接</button><button v-if="shareURL && shareID" class="secondary-action" type="button" :disabled="actionLoading" @click="revokeShare">撤销分享</button><button v-else class="primary-action" type="button" :disabled="actionLoading" @click="createShare">{{ actionLoading ? '创建中…' : '在线分享' }}</button></div><p v-if="shareCopyMessage" class="settings-feedback">{{ shareCopyMessage }}</p></template><p v-else class="settings-help">请先选择一条行程，再管理它的在线分享链接。</p></div>
                <div class="settings-note"><span class="settings-note-mark">i</span><span>分享链接是只读快照，默认有效期为 7 天。撤销后，持有链接的人将无法继续查看。</span></div>
              </section>

              <section v-else-if="settingsSection === 'about'" class="settings-page-section settings-about-section">
                <div class="settings-section-heading"><span class="eyebrow">ABOUT JOURNEYIN</span><h3>关于 JourneyIn</h3><p>了解当前版本、项目作者和 JourneyIn 的开源项目信息。</p></div>
                <div class="settings-card settings-about-hero"><span class="settings-about-mark">✦</span><div><strong>JourneyIn</strong><p>{{ APP_SLOGAN }}</p><a :href="GITHUB_URL" target="_blank" rel="noopener noreferrer">访问项目主页 ↗</a></div></div>
                <div class="settings-about-meta"><div class="settings-about-meta-item"><span>版本</span><strong>v{{ displayVersion }}</strong></div><div class="settings-about-meta-item"><span>作者</span><strong>NevermindZZT</strong></div><div class="settings-about-meta-item"><span>开源协议</span><strong>MIT License</strong></div></div>
                <div class="settings-card settings-about-description"><span class="eyebrow">PROJECT INTRODUCTION</span><p>JourneyIn 是一款地图优先的旅行规划工具，将地点、顺序、路线、天气和 Markdown 说明组织在同一份可保存的行程中。</p><p>项目提供百度地图与高德地图 Provider、Trip JSON、只读分享、同步、MCP 和 Docker 部署能力，帮助你把下一段旅程清晰地放到地图上。</p></div>
              </section>

              <section v-else class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">MODEL CONTEXT PROTOCOL</span><h3>MCP 连接</h3><p>为 AI Agent 提供 Trip 校验、预览和确认保存能力。</p></div>
                <div class="settings-card settings-mcp-card"><div class="settings-mcp-icon">◇</div><div><strong>MCP Endpoint</strong><code>{{ capabilities?.mcp?.http_endpoint || '/mcp' }}</code><p>Docker 远程部署时通过 JOURNEYIN_MCP_TOKEN 保护 HTTP MCP；本地 localhost 调试可以不设置。</p></div></div>
              </section>
            </div>
          </div>
        </section>
      </div>
      <div v-if="false && settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false"><section class="modal-panel settings-panel" role="dialog" aria-modal="true" aria-labelledby="settings-title"><button class="modal-close" aria-label="关闭" @click="settingsOpen = false">×</button><p class="eyebrow">JOURNEYIN SETTINGS</p><h2 id="settings-title">设置</h2><p class="settings-intro">当前主题：{{ themeLabel }}。Key 配置保存到 SQLite，服务端 Key 不会回显。</p><section class="settings-section"><h3>外观</h3><p class="settings-label">主题：{{ themeLabel }}</p><div class="theme-options"><button type="button" :class="{ selected: theme === 'system' }" @click="setTheme('system')">跟随系统</button><button type="button" :class="{ selected: theme === 'light' }" @click="setTheme('light')">浅色</button><button type="button" :class="{ selected: theme === 'dark' }" @click="setTheme('dark')">深色</button></div></section><section class="settings-section"><h3>服务端连接</h3><label>当前服务地址<input v-model="serverURL" readonly /></label><label>兼容 REST API Token<input v-model="authTokenInput" type="password" placeholder="仅用于兼容旧客户端，可留空" autocomplete="off" /></label><div class="modal-actions"><button type="button" @click="logout">清除令牌</button><button type="button" class="primary" @click="saveAuth">保存令牌</button></div><p v-if="settingsMessage" class="settings-message">{{ settingsMessage }}</p></section><section class="settings-section"><h3>默认地图</h3><label>默认地图 Provider<select v-model="defaultMapProvider"><option value="baidu">百度地图</option><option value="amap">高德地图</option></select></label><p class="key-help">用于没有单独地图偏好的新行程和查看页面；单个行程已保存的地图 Provider 不会被覆盖。地图工具仍可临时切换 Provider。</p><div class="modal-actions"><button type="button" class="primary" :disabled="settingsSaving" @click="saveDefaultMapProvider">{{ settingsSaving ? '保存中…' : '保存默认地图' }}</button></div></section><section class="settings-section"><h3>百度地图</h3><p class="key-status">浏览器端 Key：<strong>{{ baiduKey ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.baidu?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>百度浏览器端 Key<input v-model="baiduBrowserKeyInput" type="password" :placeholder="settingsData?.map?.baidu?.browser_key_configured ? '已配置，输入新 Key 可替换' : '用于 JSAPI 4.0/BMap 网页地图'" autocomplete="off" /></label><label>百度服务端 Key<input v-model="baiduServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><p class="key-help">浏览器端 Key 用于地图底图；服务端 Key 用于 POI 搜索、地理编码、路线和天气。请确认当前访问 host 在百度控制台白名单内。</p><a href="https://lbsyun.baidu.com/apiconsole/key" target="_blank" rel="noopener noreferrer">申请/管理百度地图 Key ↗</a></section><section class="settings-section"><h3>高德地图</h3><p class="key-status">JS Key：<strong>{{ settingsData?.map?.amap?.js_key_configured ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.amap?.server_key_configured ? '已配置' : '未配置' }}</strong> · 安全密钥：<strong>{{ settingsData?.map?.amap?.security_js_code_configured ? '已配置' : '未配置' }}</strong></p><label>高德 JS Key<input v-model="amapJSKeyInput" type="password" placeholder="用于高德 Web 地图" autocomplete="off" /></label><label>高德服务端 Key<input v-model="amapServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><label>高德 JS 安全密钥<input v-model="amapSecurityJSCodeInput" type="password" placeholder="用于 JSAPI 安全代理；已配置时输入新密钥可替换" autocomplete="off" /></label><a href="https://console.amap.com/dev/key/app" target="_blank" rel="noopener noreferrer">申请/管理高德 Key ↗</a><p class="key-help">保存后，规划点会优先使用已经保存的坐标，不会因为重新绘制地图重复查询。</p><div class="modal-actions"><button type="button" class="primary" :disabled="settingsSaving" @click="saveMapKeys">{{ settingsSaving ? '保存中…' : '保存地图 Key 到数据库' }}</button></div></section><section class="settings-section"><h3>地点检索</h3><label>优先 Provider<select v-model="poiProviderPriority"><option value="amap">高德优先</option><option value="baidu">百度优先</option></select></label><p class="key-help">当前策略会先查询本地地点目录；未命中后使用所选 Provider，Provider 不可用时自动尝试另一家。新搜索结果只保留 7 天。</p><p class="key-status">本地地点记录：<strong>{{ localDirectoryCount }}</strong> 条</p><div class="modal-actions"><button type="button" @click="savePOIPreferences">保存检索优先级</button><button type="button" @click="clearLocalDirectory">清除本地记录</button></div></section><section class="settings-section"><h3>MCP</h3><p>MCP 地址：{{ capabilities?.mcp?.http_endpoint || '/mcp' }}</p><p class="key-help">Docker 远程部署时设置 JOURNEYIN_MCP_TOKEN；本地 localhost 调试可不设置。</p></section></section></div>
      <div v-if="authOpen" class="modal-backdrop" @click.self="authOpen = false"><section class="modal-panel auth-panel" role="dialog" aria-modal="true" aria-labelledby="auth-title"><IonIcon class="auth-icon" :icon="logInOutline" /><h2 id="auth-title">登录 JourneyIn</h2><p>请输入 Docker 服务配置的账号和密码。登录成功后会在当前浏览器保存一个 HttpOnly 会话。</p><form class="auth-form" @submit.prevent="login"><label>账号<input v-model="loginUsername" type="text" autofocus autocomplete="username" /></label><label>密码<input v-model="loginPassword" type="password" autocomplete="current-password" /></label><p v-if="loginMessage" class="auth-error">{{ loginMessage }}</p><div class="modal-actions"><button type="button" @click="authOpen = false">稍后</button><button type="submit" class="primary" :disabled="loginLoading">{{ loginLoading ? '登录中…' : '登录' }}</button></div></form></section></div>
    </div>
  </IonApp>
</template>
