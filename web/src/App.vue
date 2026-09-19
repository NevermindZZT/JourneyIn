<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  IonApp, IonChip, IonIcon,
} from '@ionic/vue'
import BMapLoader from '@baidumap/jsapi-loader'
import AMapLoader from '@amap/amap-jsapi-loader'
import { addOutline, chevronBackOutline, chevronDownOutline, chevronForwardOutline, chevronUpOutline, closeOutline, cloudOfflineOutline, createOutline, footstepsOutline, imageOutline, linkOutline, logInOutline, mapOutline, menuOutline, navigateOutline, refreshOutline, searchOutline, settingsOutline, sunnyOutline } from 'ionicons/icons'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import PrototypePreview from './PrototypePreview.vue'
import UiSelect from './UiSelect.vue'
import MarkdownEditor from './MarkdownEditor.vue'
import TripPosterModal from './TripPosterModal.vue'
import BrandLogo from './BrandLogo.vue'
import MapLoadingState from './MapLoadingState.vue'

type Theme = 'system' | 'light' | 'dark'
type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord & { crs?: string }>; source?: string; provider_refs?: Record<string, unknown>; citycode?: string; adcode?: string; geocoded_at?: string; precision?: string; confidence?: number }
type LinkData = { id?: string; title: string; url: string; kind?: string }
type Stop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown>; children?: SubStop[] }
type SubStop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown> }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; mode?: string; snapshots?: Array<{ provider?: string; coordinate_system?: string; mode?: string; strategy?: string; source?: string; geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; fetched_at?: string }> }
type Day = { id: string; date: string; title?: string; notes_markdown?: string; stops: Stop[]; legs?: Leg[] }
type TripDocument = { title: string; show_in_atlas?: boolean; date_range?: { start: string; end: string }; timezone: string; description_markdown?: string; links?: LinkData[]; map?: { preferred_provider?: 'baidu' | 'amap'; enabled_providers?: Array<'baidu' | 'amap'>; default_mode?: TravelMode }; days: Day[] }
type SharedBootstrap = { trip: TripDocument & { id?: string; status?: string }; browser_key?: string; amap_browser_key?: string; amap_security_proxy_path?: string; amap_security_js_code_configured?: boolean; default_map_provider?: 'baidu' | 'amap'; revision?: number }
type TripSummary = { id: string; title: string; status: string; start_date: string; end_date: string; timezone: string; revision: number; days?: number; stops?: number; show_in_atlas?: boolean; updated_at?: string }
type TripHistoryEntry = { id: string; history_id?: string; trip_id: string; source_revision: number; title: string; start_date: string; end_date: string; label?: string; content_hash: string; created_at: string; read_only?: boolean }
type TripSortMode = 'updated' | 'date'
type Capabilities = { version?: string; default_map_provider?: 'baidu' | 'amap'; map_providers?: { baidu?: { browser_key_configured?: boolean; browser_key?: string }; amap?: { browser_key_configured?: boolean; browser_key?: string; security_proxy_path?: string; security_js_code_configured?: boolean } }; features?: { planning_point_edit?: boolean; coordinate_repair?: boolean }; mcp?: { http_endpoint?: string } }
type KeySettings = { map?: { default_provider?: 'baidu' | 'amap'; baidu?: { browser_key_configured?: boolean; server_key_configured?: boolean }; amap?: { js_key_configured?: boolean; server_key_configured?: boolean; security_js_code_configured?: boolean } }; poi?: { provider_priority?: 'amap' | 'baidu'; local_directory_count?: number }; photos?: { root_dir?: string; configured?: boolean } }
type PlaceCandidate = { id?: string; name: string; address?: string; location: Coord & { crs?: string }; provider?: string; citycode?: string; adcode?: string; typecode?: string }
type TravelMode = 'driving' | 'walking' | 'cycling' | 'transit'

const mapProviderOptions = [
  { value: 'amap', label: '高德地图', description: 'AMap' },
  { value: 'baidu', label: '百度地图', description: 'Baidu Maps' },
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
      // 最近的行程排在最前面（降序）
      const dateOrder = b.start_date.localeCompare(a.start_date) || b.end_date.localeCompare(a.end_date)
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
const tripDetailsShowInAtlasDraft = ref(true)

type AtlasStopSummary = {
  id: string
  sequence: number
  title: string
  day_index: number
  point?: Coord & { crs?: string }
}
type AtlasLegSummary = {
  id: string
  from_stop_id: string
  to_stop_id: string
  mode?: string
  distance_m?: number
  duration_s?: number
  geometry?: Array<[number, number]>
  crs?: string
}
type AtlasTripItem = {
  id: string
  title: string
  start_date: string
  end_date: string
  days_count: number
  stops_count: number
  distance_m: number
  duration_s: number
  key_stops: AtlasStopSummary[]
  legs: AtlasLegSummary[]
}
type AtlasSummaryResponse = {
  total_trips: number
  total_days: number
  total_stops: number
  total_distance_m: number
  total_duration_s: number
  trips: AtlasTripItem[]
}

const atlasLoading = ref(false)
const atlasData = ref<AtlasSummaryResponse | null>(null)
const selectedAtlasTripID = ref<string>('')

interface PhotoStatus {
  enabled: boolean
  root_dir?: string
  total_photos?: number
  gps_photos?: number
  scanning?: boolean
  last_scan_at?: string
}

interface PhotoAtlasItem {
  id: string
  file_name: string
  taken_at: string
  lat: number
  lng: number
  thumb_url: string
}

interface PhotoCluster {
  id: string
  lat: number
  lng: number
  count: number
  cover: PhotoAtlasItem
  photos: PhotoAtlasItem[]
}

const photoStatus = ref<PhotoStatus | null>(null)
const atlasPhotos = ref<PhotoAtlasItem[]>([])
const atlasShowTrips = ref(true)
const atlasShowPhotos = ref<boolean>(localStorage.getItem('journeyin.atlasShowPhotos') !== 'false')
let atlasPhotoMarkers: any[] = []
const selectedPhotoCluster = ref<PhotoCluster | null>(null)
const previewPhotoList = ref<PhotoAtlasItem[]>([])
const previewPhotoIndex = ref<number>(-1)
const previewPhotoLoading = ref<boolean>(false)
const previewPhotoError = ref<boolean>(false)
const loadedPhotoMap = ref<Record<string, boolean>>({})
const previewPhoto = computed(() => {
  if (previewPhotoIndex.value >= 0 && previewPhotoIndex.value < previewPhotoList.value.length) {
    const p = previewPhotoList.value[previewPhotoIndex.value]
    return {
      id: p.id,
      file_name: p.file_name,
      taken_at: p.taken_at,
      url: '/api/v1/photos/' + p.id + '/file',
      thumb_url: p.thumb_url
    }
  }
  return null
})
const photoSyncing = ref(false)
const ATLAS_PALETTE = [
  '#24695c',
  '#e56a4d',
  '#0284c7',
  '#8b5cf6',
  '#d97706',
  '#059669',
  '#db2777',
  '#4f46e5',
]

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
let errorDismissTimer: number | null = null
watch(error, (newVal) => {
  if (errorDismissTimer !== null) window.clearTimeout(errorDismissTimer)
  if (newVal) {
    errorDismissTimer = window.setTimeout(() => {
      error.value = ''
    }, 7000)
  }
})
function closeError() {
  if (errorDismissTimer !== null) window.clearTimeout(errorDismissTimer)
  error.value = ''
}

let noticeDismissTimer: number | null = null
watch(tripDetailsNotice, (newVal) => {
  if (noticeDismissTimer !== null) window.clearTimeout(noticeDismissTimer)
  if (newVal) {
    noticeDismissTimer = window.setTimeout(() => {
      tripDetailsNotice.value = ''
    }, 6000)
  }
})
function closeNotice() {
  if (noticeDismissTimer !== null) window.clearTimeout(noticeDismissTimer)
  tripDetailsNotice.value = ''
}
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
const posterModalOpen = ref(false)
const actionLoading = ref(false)
const settingsOpen = ref(false)
type SettingsSection = 'appearance' | 'connection' | 'maps' | 'photos' | 'search' | 'sharing' | 'mcp' | 'about'
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
const defaultMapProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.mapProvider') === 'baidu' ? 'baidu' : 'amap')
const baiduBrowserKeyInput = ref('')
const baiduServerKeyInput = ref('')
const amapJSKeyInput = ref('')
const amapServerKeyInput = ref('')
const amapSecurityJSCodeInput = ref('')
const poiProviderPriority = ref<'amap' | 'baidu'>('amap')
const photosRootDirInput = ref('')
const localDirectoryCount = ref(0)
const settingsSaving = ref(false)
const panelOpen = ref(localStorage.getItem('journeyin.panelOpen') !== 'false')
const panelCollapsed = ref(localStorage.getItem('journeyin.panelCollapsed') === 'true')
const mobileMapToolsOpen = ref(false)
const detailCollapsed = ref(localStorage.getItem('journeyin.detailCollapsed') === 'true')
const tripView = ref<'list' | 'detail' | 'atlas'>('list')
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
type MapLabelMode = 'auto' | 'always' | 'none'
const mapLabelMode = ref<MapLabelMode>((localStorage.getItem('journeyin.mapLabelMode') as MapLabelMode) || (localStorage.getItem('journeyin.mapLabels') === 'false' ? 'none' : 'auto'))
const showMapLabels = computed(() => mapLabelMode.value !== 'none')
const mapPickMode = ref(false)
const mapPickOpen = ref(false)
const mapPickTitle = ref('')
const mapPickAddress = ref('')
const mapPickDayID = ref('')
const mapPickLocation = ref<Coord & { crs: string } | null>(null)
const mapPickTargetID = ref('')
type MapPickKind = 'stop' | 'substop'
const mapPickKind = ref<MapPickKind>('stop')
const mapPickParentStopId = ref<string>('')
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
const planningProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.planningProvider') === 'baidu' ? 'baidu' : 'amap')
const selectedMapProvider = ref<'baidu' | 'amap'>(localStorage.getItem('journeyin.mapProvider') === 'baidu' ? 'baidu' : 'amap')
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
let currentStopMarkers: any[] = []
let atlasStopMarkers: any[] = []
let atlasTripLabels: any[] = []
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
const mapPickAvailableParentStops = computed(() => {
  if (!tripDocument.value || !mapPickDayID.value) return []
  const targetDay = tripDocument.value.days.find(d => d.id === mapPickDayID.value)
  return targetDay?.stops || []
})
const mapPickParentStopOptions = computed(() => {
  return mapPickAvailableParentStops.value.map((stop, idx) => ({
    value: stop.id,
    label: String(idx + 1).padStart(2, '0') + '. ' + stop.title,
    description: stop.address || (stop.children?.length ? stop.children.length + ' 个子地点' : '无子地点')
  }))
})
const mapPickParentStopTitle = computed(() => {
  if (!mapPickParentStopId.value) return ''
  return findPlanningPoint(mapPickParentStopId.value)?.title || ''
})
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
const ATLAS_PEEK_HEIGHT = 74
const SHEET_TOP_OFFSET = 60
const SHEET_BOTTOM_OFFSET = 8
function sheetViewportHeight() { return window.visualViewport?.height || window.innerHeight }
function currentSheetPeekHeight() {
  return tripView.value === 'atlas' ? ATLAS_PEEK_HEIGHT : SHEET_PEEK_HEIGHT
}
function sheetHeightBounds() {
  const viewportHeight = sheetViewportHeight()
  const peekHeight = currentSheetPeekHeight()
  const max = Math.max(peekHeight, viewportHeight - SHEET_TOP_OFFSET - SHEET_BOTTOM_OFFSET)
  const half = Math.min(max, Math.max(peekHeight, Math.round(viewportHeight * 0.53)))
  return { min: peekHeight, half, max }
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
    if (tripView.value !== 'atlas') {
      if (panelMode.value === 'search' && selectedSearchResultIndex.value >= 0 && searchResults.value[selectedSearchResultIndex.value]) {
        focusMapOnSearchResult(searchResults.value[selectedSearchResultIndex.value])
      } else if (selectedTarget.value) {
        focusSelectedMapTarget()
      }
    }
  })
  if (sheetRecenterTimer !== null) { window.clearTimeout(sheetRecenterTimer); sheetRecenterTimer = null }
  sheetRecenterTimer = window.setTimeout(() => {
    sheetRecenterTimer = null
    mapInstance?.resize?.()
    if (tripView.value === 'atlas') {
      void renderAtlasMap(false)
    } else if (panelMode.value === 'search' && selectedSearchResultIndex.value >= 0 && searchResults.value[selectedSearchResultIndex.value]) {
      focusMapOnSearchResult(searchResults.value[selectedSearchResultIndex.value])
    } else if (selectedTarget.value) {
      focusSelectedMapTarget()
    } else {
      fitVisibleMapContent()
    }
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
  const panel = (event.currentTarget as HTMLElement | null)?.closest<HTMLElement>('.workspace-panel, .stop-detail-panel, .atlas-floating-panel')
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
  if (previewPhoto.value) {
    if (event.key === 'Escape') {
      closePhotoPreview()
      event.preventDefault()
      return
    }
    if (event.key === 'ArrowLeft') {
      prevPreviewPhoto()
      event.preventDefault()
      return
    }
    if (event.key === 'ArrowRight') {
      nextPreviewPhoto()
      event.preventDefault()
      return
    }
  }
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

  let left = 0
  let top = 0
  let right = width
  let bottom = height

  const isMobile = isMobileViewport()

  if (isMobile) {
    // 手机端避让：
    // 1. 顶部避让：.workspace-topbar 和 .workspace-status
    const topbar = document.querySelector<HTMLElement>('.workspace-topbar')
    if (topbar && topbar.offsetHeight > 0) {
      const topbarRect = topbar.getBoundingClientRect()
      top = Math.max(top, topbarRect.bottom - containerRect.top + 8)
    }
    const statusPill = document.querySelector<HTMLElement>('.workspace-status')
    if (statusPill && statusPill.offsetHeight > 0) {
      const statusRect = statusPill.getBoundingClientRect()
      top = Math.max(top, statusRect.bottom - containerRect.top + 8)
    }
    if (top < 70) top = 88

    // 2. 底部避让：底部抽屉 (.stop-detail-panel 或 .workspace-panel)
    const activeSheet = document.querySelector<HTMLElement>('.stop-detail-panel, .workspace-panel')
    if (activeSheet && activeSheet.offsetHeight > 0) {
      const sheetRect = activeSheet.getBoundingClientRect()
      if (sheetRect.top < containerRect.bottom) {
        bottom = Math.min(bottom, sheetRect.top - containerRect.top - 10)
      }
    } else {
      bottom = height - 16
    }
    left = 12
    right = width - 12
  } else {
    // PC / 桌面端避让：
    // 1. 左侧避让：行程规划卡片 .workspace-panel
    const leftPanel = document.querySelector<HTMLElement>('.workspace-panel.itinerary-panel, .workspace-panel.atlas-floating-panel')
    if (leftPanel && leftPanel.offsetWidth > 0 && panelOpen.value) {
      const panelRect = leftPanel.getBoundingClientRect()
      if (panelRect.right > containerRect.left) {
        // 卡片右侧边缘 + 24px 呼吸间距，避免贴边
        left = Math.max(left, panelRect.right - containerRect.left + 24)
      }
    } else {
      left = 24
    }

    // 2. 右侧避让：若右侧地点详情抽屉打开 (.stop-detail-panel)
    const rightPanel = document.querySelector<HTMLElement>('.stop-detail-panel')
    if (rightPanel && rightPanel.offsetWidth > 0) {
      const rightRect = rightPanel.getBoundingClientRect()
      if (rightRect.left < containerRect.right) {
        right = Math.min(right, rightRect.left - containerRect.left - 24)
      }
    } else {
      right = width - 24
    }

    // 3. 上下安全边距
    top = 24
    bottom = height - 24
  }

  // 兜底安全校验，确保有效可视尺寸至少有 100px
  if (right <= left + 100 || bottom <= top + 100) {
    return { left: 0, top: 0, right: width, bottom: height }
  }

  return { left, top, right, bottom }
}

function visibleMapPoints(): any[] {
  if (!mapAPI) return []
  const provider = selectedMapProvider.value
  const points: any[] = []
  const pushPoint = (point: Coord | null) => {
    if (!point || !Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return
    if (provider === 'amap') points.push([point.lng, point.lat])
    else if (typeof mapAPI.Point === 'function') points.push(new mapAPI.Point(point.lng, point.lat))
  }
  for (const stop of mapStops.value) {
    pushPoint(pointForProvider(stop, provider))
  }
  for (const child of selectedStop.value?.children || []) {
    pushPoint(pointForProvider(child, provider))
  }
  // 采样加入可见路段的关键坐标，确保路线弧度也能被纳入视野
  for (const day of visibleDays.value) {
    for (const leg of day.legs || []) {
      const snapshot = chooseSnapshot(leg, provider, planningMode.value)
      if (!snapshot?.geometry?.length) continue
      const geo = snapshot.geometry
      const step = Math.max(1, Math.floor(geo.length / 8))
      for (let i = 0; i < geo.length; i += step) {
        const pt = mapRoutePointFor(geo[i], snapshot.coordinate_system || (provider === 'amap' ? 'gcj02' : 'bd09ll'), provider)
        if (pt) pushPoint(pt)
      }
      const lastPt = mapRoutePointFor(geo[geo.length - 1], snapshot.coordinate_system || (provider === 'amap' ? 'gcj02' : 'bd09ll'), provider)
      if (lastPt) pushPoint(lastPt)
    }
  }
  return points
}

function fitVisibleMapContent() {
  if (!mapInstance || !mapAPI) return
  if (tripView.value === 'atlas') {
    void renderAtlasMap(false)
    return
  }
  const rect = mapVisibleRect()
  if (!rect) return
  const container = mapContainer.value
  const fullW = container?.clientWidth || 0
  const fullH = container?.clientHeight || 0
  const points = visibleMapPoints()
  if (!points.length) return
  const provider = selectedMapProvider.value

  const isMobile = isMobileViewport()
  let padTop = 24
  let padBottom = 24
  let padLeft = 24
  let padRight = 24

  if (isMobile) {
    padTop = Math.max(20, Math.round(rect.top + 8))
    // 手机端左右对称居中
    padLeft = 24
    padRight = 24
    if (sheetBreakpoint.value === 'expanded') {
      padBottom = 40
    } else {
      padBottom = Math.max(24, Math.round((fullH - rect.bottom) + 12))
    }
  } else {
    // 桌面端：左侧避让行程卡片，右侧预留舒适边距
    padTop = 32
    padBottom = 32
    padLeft = Math.max(36, Math.round(rect.left))
    padRight = Math.max(48, Math.round(fullW - rect.right + 24))
  }

  if (provider === 'amap') {
    const overlays = mapOverlays.filter(Boolean)
    if (overlays.length && typeof mapInstance.setFitView === 'function') {
      try {
        // 高德 JSAPI 2.0 避让参数 avoid 为：[上, 下, 左, 右] (上下左右)
        mapInstance.setFitView(overlays, false, [padTop, padBottom, padLeft, padRight])
        return
      } catch {
        /* fallback to manual calculation */
      }
    }
  } else if (provider === 'baidu') {
    if (typeof mapInstance.getViewport === 'function' && points.length) {
      try {
        // 百度 JSAPI 4.0 margins 参数为：[上, 右, 下, 左] (上右下左)
        const view = mapInstance.getViewport(points, { margins: [padTop, padRight, padBottom, padLeft] })
        if (view) {
          mapInstance.centerAndZoom(view.center, view.zoom)
          return
        }
      } catch {
        /* fallback to manual calculation */
      }
    }
  }

  const visibleW = rect.right - rect.left
  const visibleH = rect.bottom - rect.top
  const readPixel = (value: any): { x: number; y: number } | null => {
    try {
      if (provider === 'amap') {
        const lnglat = Array.isArray(value) ? new mapAPI.LngLat(Number(value[0]), Number(value[1])) : value
        const pixel = mapInstance.lngLatToContainer?.(lnglat)
        if (Array.isArray(pixel)) return { x: Number(pixel[0]), y: Number(pixel[1]) }
        if (pixel && typeof pixel.getX === 'function') return { x: Number(pixel.getX()), y: Number(pixel.getY()) }
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
  const first = measure()
  if (!first) return
  const pad = 36
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
    const pixel = new mapAPI.Pixel(newCenterX, newCenterY)
    const center = mapInstance.containerToLngLat?.(pixel)
    if (center) mapInstance.setCenter?.(center, true)
    return
  }
  const center = mapInstance.pixelToPoint?.(new mapAPI.Pixel(newCenterX, newCenterY))
  if (center) mapInstance.setCenter?.(center, { noAnimation: true })
}

function recenterMapToVisibleViewport() {
  if (!mapInstance || !mapAPI) return
  const rect = mapVisibleRect()
  if (!rect) return
  const container = mapContainer.value
  const fullW = container?.clientWidth || 0
  const fullH = container?.clientHeight || 0
  const visibleCenterX = (rect.left + rect.right) / 2
  const visibleCenterY = (rect.top + rect.bottom) / 2
  const offsetX = visibleCenterX - fullW / 2
  const offsetY = visibleCenterY - fullH / 2
  if (Math.abs(offsetX) < 2 && Math.abs(offsetY) < 2) return
  if (selectedMapProvider.value === 'amap') {
    if (typeof mapInstance.containerToLngLat !== 'function' || typeof mapInstance.setCenter !== 'function') return
    const targetPixel = new mapAPI.Pixel(fullW / 2 - offsetX, fullH / 2 - offsetY)
    const shifted = mapInstance.containerToLngLat(targetPixel)
    if (shifted) mapInstance.setCenter(shifted, true)
    return
  }
  if (typeof mapAPI.Pixel !== 'function' || typeof mapInstance.pixelToPoint !== 'function' || typeof mapInstance.setCenter !== 'function') return
  const targetPixel = new mapAPI.Pixel(fullW / 2 - offsetX, fullH / 2 - offsetY)
  const center = mapInstance.pixelToPoint(targetPixel)
  if (center) mapInstance.setCenter(center, { noAnimation: true })
}

function handleViewportResize() {
  window.requestAnimationFrame(() => {
    mapInstance?.resize?.()
    if (tripView.value === 'atlas') {
      void renderAtlasMap(false)
      updateAtlasLabelsVisibility()
    } else if (panelMode.value === 'search' && selectedSearchResultIndex.value >= 0 && searchResults.value[selectedSearchResultIndex.value]) {
      focusMapOnSearchResult(searchResults.value[selectedSearchResultIndex.value])
    } else {
      if (selectedTarget.value) focusSelectedMapTarget()
      else fitVisibleMapContent()
      updateStopLabelsVisibility()
    }
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
  searchQuery.value = ''
  searchRegion.value = ''
  searchResults.value = []
  selectedSearchResultIndex.value = -1
  clearSearchResultMarkers()
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
  searchResults.value = []
  selectedSearchResultIndex.value = -1
  clearSearchResultMarkers()
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
    const response = await apiFetch('/api/v1/trips', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        schema_version: 1,
        title: newTitle.value.trim(),
        status: 'draft',
        locale: 'zh-CN',
        timezone: newTimezone.value,
        date_range: { start, end },
        description_markdown: newDescription.value,
        links: [],
        map: { preferred_provider: defaultMapProvider.value, enabled_providers: ['baidu', 'amap'], default_mode: 'walking' },
        days,
        metadata: { source: 'human' }
      })
    })
    const payload = await response.json() as { error?: { message?: string } } & TripSummary & { document?: TripDocument }
    if (!response.ok) throw new Error(payload.error?.message || '新建旅行规划失败')
    newTripOpen.value = false
    await loadTrips()
    if (payload.id) {
      const summaryItem: TripSummary = {
        id: payload.id,
        title: payload.title || newTitle.value.trim(),
        status: payload.status || 'draft',
        start_date: payload.start_date || start,
        end_date: payload.end_date || end,
        timezone: payload.timezone || newTimezone.value,
        revision: payload.revision || 1,
        days: payload.days ?? total,
        stops: payload.stops ?? 0,
        show_in_atlas: payload.show_in_atlas ?? true,
        updated_at: payload.updated_at
      }
      navigateToTrip(summaryItem, 'push')
    }
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
  const rect = mapVisibleRect()
  if (!rect) return null
  const width = rect.right - rect.left
  const height = rect.bottom - rect.top
  return {
    width,
    height,
    x: (rect.left + rect.right) / 2,
    y: (rect.top + rect.bottom) / 2
  }
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
  tripDetailsShowInAtlasDraft.value = tripDocument.value.show_in_atlas !== false
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
async function toggleTripAtlasFromList(trip: TripSummary) {
  tripMenuID.value = ''
  const nextVal = trip.show_in_atlas === false
  actionLoading.value = true
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(trip.id), {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        'If-Match': 'revision-' + trip.revision,
        'Idempotency-Key': makeID('toggle-atlas'),
      },
      body: JSON.stringify({ show_in_atlas: nextVal }),
    })
    if (!response.ok) {
      const payload = (await response.json()) as { error?: { message?: string } }
      throw new Error(payload.error?.message || '设置足迹状态失败')
    }
    const payload = (await response.json()) as { revision?: number; document?: TripDocument }
    trip.show_in_atlas = nextVal
    if (payload.revision !== undefined) trip.revision = payload.revision
    if (selected.value?.id === trip.id) {
      selected.value.show_in_atlas = nextVal
      if (tripDocument.value) tripDocument.value.show_in_atlas = nextVal
    }
    tripDetailsNotice.value = nextVal ? '已将“' + trip.title + '”加入足迹漫游' : '已从足迹漫游中移除“' + trip.title + '”'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '设置足迹状态失败'
  } finally {
    actionLoading.value = false
  }
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
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(tripID), { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision, 'Idempotency-Key': tripDetailsIdempotencyKey.value || makeID('trip-details') }, body: JSON.stringify({ title, date_range: { start: tripDetailsStartDateDraft.value, end: tripDetailsEndDateDraft.value }, show_in_atlas: tripDetailsShowInAtlasDraft.value }) })
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


async function renderBaiduMap(preserveView = false) {
  if (!baiduKey.value || !mapContainer.value || !tripDocument.value) return
  const renderVersion = ++mapRenderVersion
  ++mapFocusVersion
  try {
    await loadBaiduMap()
    if (renderVersion !== mapRenderVersion) return
    if (!mapAPI || typeof mapAPI.Map !== 'function' || !mapContainer.value) throw new Error('百度 JSAPI 未提供可用的 Map 构造器；请检查浏览器端 AK、服务权限、域名白名单和当前浏览器环境')
    if (!mapInstance) {
      mapReady.value = false
      mapInstance = new mapAPI.Map(mapContainer.value, { enableIconClick: true, enableMapClick: true, fixCenterWhenResize: true })
      mapInstance.enableScrollWheelZoom()
      mapInstance.addEventListener?.('click', handleMapClick)
      mapInstance.addEventListener?.('spotclick', handleBaiduSpotClick)
      mapInstance.addEventListener?.('zoomend', handleMapZoomChange)
      mapInstance.addEventListener?.('tilesloaded', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
      if (mapReadyTimer !== null) window.clearTimeout(mapReadyTimer)
      mapReadyTimer = window.setTimeout(() => {
        if (!mapReady.value) mapWarning.value = '百度地图底图加载较慢；请检查浏览器端 AK、' + window.location.hostname + ' 域名白名单和网络连接。地图仍可继续尝试加载。'
      }, 8000)
    }
    mapInstance.clearOverlays()
    currentStopMarkers = []
    const points: any[] = []
    const visibleLabelIDs = computeVisibleLabelStopIDs(mapStops.value)
    for (const stop of mapStops.value) {
      const point = mapPointFor(stop)
      if (!point || point.crs !== 'bd09ll') continue
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      points.push(mapPoint)
      const marker = new mapAPI.Marker(mapPoint)
      const carryOver = carryOverStop.value?.id === stop.id && selectedDay.value !== 'all'
      const isTarget = selectedTarget.value?.id === stop.id
      marker.__journeyinStopId = stop.id
      marker.__journeyinCarryOver = carryOver
      marker.__journeyinStop = stop
      marker.__journeyinTitle = carryOver ? '前日终点 · ' + stop.title : stop.title
      marker.__journeyinBadge = carryOver ? '' : stopDayBadge(stop)
      marker.__journeyinIsSubStop = false
      currentStopMarkers.push(marker)
      marker.addEventListener?.('click', () => {
        if (mapPickMode.value) { handleMapClick({ point: mapPoint }); return }
        if (carryOver) {
          const carryOverDayIndex = tripDocument.value?.days.findIndex(day => day.stops.some(item => item.id === stop.id)) ?? -1
          if (carryOverDayIndex >= 0) { selectedDay.value = carryOverDayIndex + 1; selectStop(stop) }
          return
        }
        selectStop(stop)
      })
      const badge = stopDayBadge(stop)
      const shouldShow = visibleLabelIDs.has(stop.id)
      attachMapLabel(marker, carryOver ? '前日终点 · ' + stop.title : stop.title, carryOver ? '' : badge, isTarget, shouldShow)
      mapInstance.addOverlay(marker)
    }
    if (selectedStop.value?.children?.length) {
      const childVisibleIDs = computeVisibleLabelStopIDs(selectedStop.value.children)
      for (const child of selectedStop.value.children) {
        const point = mapPointFor(child)
        if (!point || point.crs !== 'bd09ll') continue
        const mapPoint = new mapAPI.Point(point.lng, point.lat)
        const marker = new mapAPI.Marker(mapPoint)
        const isChildTarget = selectedTarget.value?.id === child.id
        marker.__journeyinSubStopId = child.id
        marker.__journeyinStop = child
        marker.__journeyinTitle = child.title
        marker.__journeyinBadge = ''
        marker.__journeyinIsSubStop = true
        currentStopMarkers.push(marker)
        marker.addEventListener?.('click', () => { if (mapPickMode.value) { handleMapClick({ point: mapPoint }); return }; selectSubStop(child, selectedStop.value!) })
        const shouldShow = childVisibleIDs.has(child.id)
        attachMapLabel(marker, child.title, '', isChildTarget, shouldShow)
        mapInstance.addOverlay(marker)
      }
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
        attachRouteLabel(snapshot, leg.id)
      }
    }
    if (!preserveView) {
      const focusTarget = selectedTarget.value
      if (focusTarget) {
        await nextTick()
        if (renderVersion !== mapRenderVersion) return
        focusMapOnPoint(focusTarget)
      } else if (points.length) fitVisibleMapContent()
      else mapInstance.centerAndZoom('中国', 5)
    }
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
  currentStopMarkers = []
  atlasStopMarkers = []
  atlasTripLabels = []
  atlasPhotoMarkers = []
  if (mapZoomDebounceTimer !== null) { window.clearTimeout(mapZoomDebounceTimer); mapZoomDebounceTimer = null }
  amapSatelliteLayer = null
  mapReady.value = false
  mapWarning.value = ''
  if (mapReadyTimer !== null) { window.clearTimeout(mapReadyTimer); mapReadyTimer = null }
  if (mapContainer.value) {
    try { mapContainer.value.innerHTML = '' } catch { /* best effort */ }
  }
  try { BMapLoader.reset() } catch { /* loader reset is best effort */ }
  try { (AMapLoader as any).reset?.() } catch { /* loader reset is best effort */ }
  try {
    delete (window as any).AMap
    delete (window as any).AMapUI
    delete (window as any).Loca
  } catch { /* cleanup is best effort */ }
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
    amapScriptPromise = AMapLoader.load({ key: currentKey, version: '2.0', plugins: ['AMap.Scale', 'AMap.TileLayer.Satellite'] }).then(namespace => {
      mapAPI = namespace
      loadedAMapKey = currentKey
    })
  }
  try { await amapScriptPromise } catch (cause) { resetMapSDK(); throw cause }
}
function safeMapError(cause: unknown, fallback: string) { const message = cause instanceof Error ? cause.message : String(cause || ''); return (message || fallback).replace(/([?&](?:ak|key|jscode)=)[^&\s'\"]+/gi, '$1<redacted>') }
function amapPointToArray(point: Coord) { return [point.lng, point.lat] }
function addAMapOverlay(overlay: any) { mapInstance?.add?.(overlay); mapOverlays.push(overlay); return overlay }
function clearAMapOverlays() { mapInstance?.clearMap?.(); mapOverlays = []; currentStopMarkers = []; atlasStopMarkers = []; atlasTripLabels = [] }
function attachAMapLabel(marker: any, title: string, dayBadge = '', isTarget = false, shouldShow = true) {
  if (typeof marker.setLabel !== 'function') return
  if (!shouldShow) {
    try { marker.setLabel({ content: '' }) } catch { /* best effort */ }
    return
  }
  const badgeHtml = dayBadge ? '<span style="display:inline-block;margin-left:5px;padding:1px 5px;background:#0284c7;color:#fff;border-radius:4px;font-size:10px;line-height:13px;font-weight:700;">' + escapeHTML(dayBadge) + '</span>' : ''
  const highlightStyle = isTarget ? 'border-color:#ff9a78;background:#13373fee;box-shadow:0 0 0 2px #ff9a7888, 0 4px 14px #0008;z-index:999;' : ''
  const content = '<span class="journey-map-marker-label' + (isTarget ? ' is-target' : '') + '" style="display:inline-flex;align-items:center;padding:4px 8px;border:1px solid #0b2f35;border-radius:8px;color:#ffffff;background:#173f47ee;box-shadow:0 3px 10px #0006;font-size:12px;font-weight:700;line-height:16px;white-space:nowrap;text-shadow:0 1px 2px #0008;' + highlightStyle + '">' + escapeHTML(title) + badgeHtml + '</span>'

  // 智能自适应展开方向：若图钉位于视口右侧边缘附近，自动改为向左展开
  let direction: 'right' | 'left' = 'right'
  let offset = new mapAPI.Pixel(12, -6)
  try {
    const pos = marker.getPosition?.()
    if (pos && mapInstance?.lngLatToContainer) {
      const pixel = mapInstance.lngLatToContainer(pos)
      const px = Array.isArray(pixel) ? pixel[0] : typeof pixel?.getX === 'function' ? pixel.getX() : pixel?.x
      const containerW = mapContainer.value?.clientWidth || window.innerWidth
      const rect = mapVisibleRect()
      const rightLimit = rect ? rect.right : containerW
      if (typeof px === 'number' && px > rightLimit - 180) {
        direction = 'left'
        offset = new mapAPI.Pixel(-12, -6)
      }
    }
  } catch {}

  marker.setLabel({ content, direction, offset })
  if (isTarget && typeof marker.setTop === 'function') {
    marker.setTop(true)
  }
}
function attachAMapRouteLabel(snapshot: { geometry?: Array<[number, number]> | Array<Coord>; coordinate_system?: string; distance_m?: number; duration_s?: number }, legId = '') {
  if (!snapshot.geometry?.length || typeof mapAPI?.Text !== 'function') return
  const isLegSelected = Boolean(legId && selectedLegId.value === legId)
  if (!shouldShowRouteLabel(snapshot.distance_m, isLegSelected)) return
  const text = [formatDistance(snapshot.distance_m), formatDuration(snapshot.duration_s)].filter(Boolean).join(' · ')
  if (!text) return
  const middle = mapRoutePointFor(snapshot.geometry[Math.floor(snapshot.geometry.length / 2)], snapshot.coordinate_system || 'gcj02', 'amap')
  if (!middle) return
  addAMapOverlay(new mapAPI.Text({
    text,
    position: amapPointToArray(middle),
    collision: true,
    allowCollision: false,
    zIndex: isLegSelected ? 120 : 60,
    style: {
      backgroundColor: isLegSelected ? '#e56a4ded' : '#24695cdd',
      color: '#ffffff',
      border: '0',
      borderRadius: '999px',
      padding: '4px 8px',
      fontSize: '11px',
      lineHeight: '15px',
      whiteSpace: 'nowrap',
      boxShadow: '0 3px 10px #0003',
    }
  }))
}
function focusAMapPoint(stop: Stop | SubStop) {
  if (!mapInstance || selectedMapProvider.value !== 'amap') return
  const point = pointForProvider(stop, 'amap')
  if (!point) return
  const focusVersion = ++mapFocusVersion
  mapInstance.resize?.()
  const pt = amapPointToArray(point)
  mapInstance.setCenter?.(pt, true)
  mapInstance.setZoom?.(SELECTED_STOP_ZOOM, true)
  const alignVisibleCenter = () => {
    if (focusVersion !== mapFocusVersion || !mapAPI?.Pixel || !mapInstance?.containerToLngLat) return
    const viewport = mapFocusViewport()
    if (!viewport) return
    const container = mapContainer.value
    const fullW = container?.clientWidth || 0
    const fullH = container?.clientHeight || 0
    const offsetX = viewport.x - fullW / 2
    const offsetY = viewport.y - fullH / 2
    if (Math.abs(offsetX) < 2 && Math.abs(offsetY) < 2) return
    const targetPixel = new mapAPI.Pixel(fullW / 2 - offsetX, fullH / 2 - offsetY)
    const adjustedCenter = mapInstance.containerToLngLat(targetPixel)
    if (adjustedCenter) mapInstance.setCenter(adjustedCenter, true)
  }
  window.requestAnimationFrame(alignVisibleCenter)
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
  const focusVersion = ++mapFocusVersion
  mapInstance.resize?.()
  if (selectedMapProvider.value === 'amap') {
    const pt = amapPointToArray(point)
    mapInstance.setCenter?.(pt, true)
    mapInstance.setZoom?.(SELECTED_STOP_ZOOM, true)
  } else {
    const mapPoint = new mapAPI.Point(point.lng, point.lat)
    if (typeof mapInstance.centerAndZoom === 'function') mapInstance.centerAndZoom(mapPoint, SELECTED_STOP_ZOOM, { noAnimation: true })
    else { mapInstance.setCenter?.(mapPoint); mapInstance.setZoom?.(SELECTED_STOP_ZOOM, { zoomCenter: mapPoint }) }
  }
  const alignVisibleCenter = () => {
    if (focusVersion !== mapFocusVersion) return
    const rect = mapVisibleRect()
    if (!rect) return
    const container = mapContainer.value
    const fullW = container?.clientWidth || 0
    const fullH = container?.clientHeight || 0
    const visibleCenterX = (rect.left + rect.right) / 2
    const visibleCenterY = (rect.top + rect.bottom) / 2
    const offsetX = visibleCenterX - fullW / 2
    const offsetY = visibleCenterY - fullH / 2
    if (Math.abs(offsetX) < 2 && Math.abs(offsetY) < 2) return
    if (selectedMapProvider.value === 'amap') {
      if (!mapAPI?.Pixel || !mapInstance?.containerToLngLat) return
      const targetPixel = new mapAPI.Pixel(fullW / 2 - offsetX, fullH / 2 - offsetY)
      const adjustedCenter = mapInstance.containerToLngLat(targetPixel)
      if (adjustedCenter) mapInstance.setCenter(adjustedCenter, true)
    } else {
      if (!mapAPI?.Pixel || !mapInstance?.pixelToPoint) return
      const targetPixel = new mapAPI.Pixel(fullW / 2 - offsetX, fullH / 2 - offsetY)
      const adjustedCenter = mapInstance.pixelToPoint(targetPixel)
      if (adjustedCenter) mapInstance.setCenter(adjustedCenter, { noAnimation: true })
    }
  }
  window.requestAnimationFrame(alignVisibleCenter)
}

function renderSearchResultMarkers() {
  if (!mapInstance || !mapAPI || panelMode.value !== 'search' || !searchResults.value.length) { clearSearchResultMarkers(); return }
  clearSearchResultMarkers()
  const containerW = mapContainer.value?.clientWidth || window.innerWidth
  const rect = mapVisibleRect()
  const rightLimit = rect ? rect.right : containerW

  searchResults.value.forEach((result, index) => {
    const point = searchResultMapPoint(result, selectedMapProvider.value)
    if (!point) return
    const selected = index === selectedSearchResultIndex.value
    const label = String(index + 1)
    let marker: any = null
    if (selectedMapProvider.value === 'amap') {
      marker = new mapAPI.Marker({
        position: amapPointToArray(point),
        title: result.name,
        content: '<div class="search-result-pin' + (selected ? ' selected' : '') + '"><span>' + label + '</span></div>',
        offset: new mapAPI.Pixel(-13, -13),
        zIndex: selected ? 150 : 120
      })
      marker.on?.('click', () => selectSearchResult(index))
      if (selected && typeof marker.setLabel === 'function') {
        let dir: 'right' | 'left' = 'right'
        let off = new mapAPI.Pixel(14, -6)
        try {
          const px = mapInstance.lngLatToContainer?.(new mapAPI.LngLat(point.lng, point.lat))
          const screenX = Array.isArray(px) ? px[0] : typeof px?.getX === 'function' ? px.getX() : px?.x
          if (typeof screenX === 'number' && screenX > rightLimit - 180) {
            dir = 'left'
            off = new mapAPI.Pixel(-14, -6)
          }
        } catch {}
        marker.setLabel({
          content: '<span style="display:inline-block;padding:3px 8px;border-radius:6px;background:#173f47ee;color:#fff;font-size:11px;font-weight:700;box-shadow:0 3px 10px rgba(0,0,0,0.3);border:1.5px solid #e56a4d;white-space:nowrap;cursor:pointer;">' + escapeHTML(result.name) + '</span>',
          direction: dir,
          offset: off
        })
      }
      mapInstance.add?.(marker)
    } else {
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      const icon = searchResultBaiduIcon(selected, label)
      marker = new mapAPI.Marker(mapPoint, icon ? { icon } : undefined)
      marker.addEventListener?.('click', () => selectSearchResult(index))
      if (selected && mapAPI?.Label) {
        let off = new mapAPI.Size(16, -18)
        try {
          const px = mapInstance.pointToPixel?.(mapPoint)
          const screenX = typeof px?.x === 'number' ? px.x : null
          if (typeof screenX === 'number' && screenX > rightLimit - 180) {
            const estWidth = Math.max(60, result.name.length * 12 + 20)
            off = new mapAPI.Size(-estWidth - 10, -18)
          }
        } catch {}
        const labelObj = new mapAPI.Label(result.name, { offset: off })
        labelObj.setStyle?.({
          color: '#fff',
          backgroundColor: '#173f47ee',
          border: '1.5px solid #e56a4d',
          borderRadius: '6px',
          padding: '3px 7px',
          fontSize: '11px',
          fontWeight: '700',
          cursor: 'pointer'
        })
        marker.setLabel(labelObj)
      }
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

async function renderAMapMap(preserveView = false) {
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
      mapInstance = new mapAPI.Map(mapContainer.value, {
        viewMode: '2D',
        zoom: first ? 12 : 5,
        center: first ? amapPointToArray(first) : [116.397428, 39.90923],
        resizeEnable: true,
        isHotspot: true,
      })
      mapInstance.on?.('click', (event: any) => {
        const lnglat = event?.lnglat
        const lng = Number(lnglat?.lng ?? lnglat?.getLng?.())
        const lat = Number(lnglat?.lat ?? lnglat?.getLat?.())
        if (Number.isFinite(lng) && Number.isFinite(lat)) handleMapClick({ point: { lng, lat, crs: 'gcj02' } })
      })
      mapInstance.on?.('hotspotclick', handleAMapHotspotClick)
      mapInstance.on?.('zoomend', handleMapZoomChange)
      mapInstance.on?.('complete', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
      mapInstance.on?.('error', (err: any) => {
        console.error('AMap runtime error:', err)
        mapError.value = safeMapError(err, '高德地图运行异常')
      })
      if (mapReadyTimer !== null) window.clearTimeout(mapReadyTimer)
      mapReadyTimer = window.setTimeout(() => { if (!mapReady.value) mapWarning.value = '高德地图底图加载较慢；请检查 JS Key、安全密钥、域名白名单和网络连接。地图仍可继续尝试加载。' }, 8000)
      loadedMapKey = ''
    }
    mapInstance.resize?.()
    clearAMapOverlays()
    currentStopMarkers = []
    const points: any[] = []
    const visibleLabelIDs = computeVisibleLabelStopIDs(mapStops.value)
    for (const stop of mapStops.value) {
      const point = pointForProvider(stop, 'amap')
      if (!point) continue
      const mapPoint = amapPointToArray(point)
      points.push(mapPoint)
      const marker = addAMapOverlay(new mapAPI.Marker({ position: mapPoint, title: stop.title, anchor: 'bottom-center' }))
      const carryOver = carryOverStop.value?.id === stop.id && selectedDay.value !== 'all'
      const isTarget = selectedTarget.value?.id === stop.id
      marker.__journeyinStopId = stop.id
      marker.__journeyinCarryOver = carryOver
      marker.__journeyinStop = stop
      marker.__journeyinTitle = carryOver ? '前日终点 · ' + stop.title : stop.title
      marker.__journeyinBadge = carryOver ? '' : stopDayBadge(stop)
      marker.__journeyinIsSubStop = false
      currentStopMarkers.push(marker)
      marker.on?.('click', () => {
        if (mapPickMode.value) { handleMapClick({ point: { lng: point.lng, lat: point.lat, crs: 'gcj02' } }); return }
        if (carryOver) {
          const carryOverDayIndex = tripDocument.value?.days.findIndex(day => day.stops.some(item => item.id === stop.id)) ?? -1
          if (carryOverDayIndex >= 0) { selectedDay.value = carryOverDayIndex + 1; selectStop(stop) }
          return
        }
        selectStop(stop)
      })
      const badge = stopDayBadge(stop)
      const shouldShow = visibleLabelIDs.has(stop.id)
      attachAMapLabel(marker, carryOver ? '前日终点 · ' + stop.title : stop.title, carryOver ? '' : badge, isTarget, shouldShow)
    }
    if (selectedStop.value?.children?.length) {
      const childVisibleIDs = computeVisibleLabelStopIDs(selectedStop.value.children)
      for (const child of selectedStop.value.children) {
        const point = pointForProvider(child, 'amap')
        if (!point) continue
        const mapPoint = amapPointToArray(point)
        const marker = addAMapOverlay(new mapAPI.Marker({ position: mapPoint, title: child.title, anchor: 'bottom-center' }))
        const isChildTarget = selectedTarget.value?.id === child.id
        marker.__journeyinSubStopId = child.id
        marker.__journeyinStop = child
        marker.__journeyinTitle = child.title
        marker.__journeyinBadge = ''
        marker.__journeyinIsSubStop = true
        currentStopMarkers.push(marker)
        marker.on?.('click', () => { if (mapPickMode.value) { handleMapClick({ point: { lng: point.lng, lat: point.lat, crs: 'gcj02' } }); return }; selectSubStop(child, selectedStop.value!) })
        const shouldShow = childVisibleIDs.has(child.id)
        attachAMapLabel(marker, child.title, '', isChildTarget, shouldShow)
      }
    }
    for (const day of visibleDays.value) for (const leg of day.legs || []) {
      const snapshot = chooseSnapshot(leg, 'amap', planningMode.value)
      if (!snapshot?.geometry?.length) continue
      const line = snapshot.geometry.map(value => mapRoutePointFor(value, snapshot.coordinate_system || 'gcj02', 'amap')).filter((point): point is Coord & { crs: string } => Boolean(point)).map(point => amapPointToArray(point))
      if (line.length < 2) continue
      const polyline = addAMapOverlay(new mapAPI.Polyline({ path: line, strokeColor: '#24695c', strokeWeight: 5, strokeOpacity: .82, lineJoin: 'round', showDir: true, zIndex: 50 }))
      polyline.__journeyinLegId = leg.id
      polyline.on?.('click', () => { selectedLegId.value = leg.id })
      attachAMapRouteLabel(snapshot, leg.id)
    }
    if (!preserveView) {
      const focusTarget = selectedTarget.value
      if (focusTarget) { await nextTick(); if (renderVersion !== mapRenderVersion) return; focusAMapPoint(focusTarget) }
      else if (points.length) fitVisibleMapContent()
      else mapInstance.setCenter?.([116.397428, 39.90923])
    }
    applyMapType()
    mapError.value = ''
    renderSearchResultMarkers()
  } catch (cause) {
    mapReady.value = false
    mapWarning.value = ''
    mapError.value = safeMapError(cause, '高德地图初始化失败')
  }
}
async function renderMap(options?: { preserveView?: boolean }) {
  if (!mapContainer.value) return
  const preserveView = options?.preserveView ?? false
  if (tripView.value === 'atlas') return renderAtlasMap(preserveView)
  if (!tripDocument.value) return
  if (!key.value) { resetMapSDK(); return }
  if (selectedMapProvider.value === 'amap') return renderAMapMap(preserveView)
  return renderBaiduMap(preserveView)
}

function toggleAtlasSheetBreakpoint() {
  const next: SheetBreakpoint = sheetBreakpoint.value === 'peek' ? 'half' : 'peek'
  setSheetBreakpoint(next)
}
const atlasPanelCollapsed = computed(() => sheetBreakpoint.value === 'peek')

async function loadAtlasData() {
  atlasLoading.value = true
  try {
    const resp = await apiFetch('/api/v1/atlas')
    if (!resp.ok) throw new Error('无法读取足迹漫游数据')
    atlasData.value = (await resp.json()) as AtlasSummaryResponse
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取足迹数据失败'
  } finally {
    atlasLoading.value = false
  }
}

async function loadPhotoStatus() {
  try {
    const resp = await apiFetch('/api/v1/photos/status')
    if (resp.ok) {
      photoStatus.value = (await resp.json()) as PhotoStatus
    }
  } catch {
    /* ignore */
  }
}

async function loadAtlasPhotos() {
  if (!photoStatus.value?.enabled) return
  try {
    const crs = selectedMapProvider.value === 'baidu' ? 'bd09ll' : 'gcj02'
    const resp = await apiFetch('/api/v1/photos/atlas?crs=' + crs)
    if (resp.ok) {
      atlasPhotos.value = (await resp.json()) as PhotoAtlasItem[]
    }
  } catch (cause) {
    console.warn('load atlas photos failed:', cause)
  }
}

async function triggerPhotoSync() {
  if (photoSyncing.value) return
  photoSyncing.value = true
  try {
    await apiFetch('/api/v1/photos/sync', { method: 'POST' })
    window.setTimeout(async () => {
      await loadPhotoStatus()
      await loadAtlasPhotos()
      photoSyncing.value = false
      if (tripView.value === 'atlas') void renderAtlasMap(true)
    }, 1500)
  } catch {
    photoSyncing.value = false
  }
}

function clusterPhotos(photos: PhotoAtlasItem[], zoom: number): PhotoCluster[] {
  if (!photos || !photos.length) return []
  if (zoom >= 17) {
    return photos.map(p => ({
      id: 'single_' + p.id,
      lat: p.lat,
      lng: p.lng,
      count: 1,
      cover: p,
      photos: [p]
    }))
  }
  // 按照屏幕像素聚合跨度 64px 动态计算经纬度网格步长
  const gridSize = (360 / (Math.pow(2, zoom) * 256)) * 64
  const grid = new Map<string, PhotoAtlasItem[]>()
  for (const p of photos) {
    const gx = Math.floor(p.lng / gridSize)
    const gy = Math.floor(p.lat / gridSize)
    const key = gx + '_' + gy
    const list = grid.get(key)
    if (list) list.push(p)
    else grid.set(key, [p])
  }

  const clusters: PhotoCluster[] = []
  grid.forEach((items, key) => {
    let sumLat = 0
    let sumLng = 0
    for (const item of items) {
      sumLat += item.lat
      sumLng += item.lng
    }
    const sorted = [...items].sort((a, b) => b.taken_at.localeCompare(a.taken_at))
    clusters.push({
      id: 'cluster_' + key,
      lat: sumLat / items.length,
      lng: sumLng / items.length,
      count: items.length,
      cover: sorted[0],
      photos: sorted
    })
  })
  return clusters
}

function clearAtlasPhotoMarkers() {
  if (selectedMapProvider.value === 'amap') {
    atlasPhotoMarkers.forEach(m => {
      try { mapInstance?.remove?.(m) } catch {}
    })
  } else {
    atlasPhotoMarkers.forEach(m => {
      try { mapInstance?.removeOverlay?.(m) } catch {}
    })
  }
  atlasPhotoMarkers = []
}

function renderAtlasPhotoMarkers() {
  clearAtlasPhotoMarkers()
  if (!atlasShowPhotos.value || !photoStatus.value?.enabled || !atlasPhotos.value.length || !mapInstance) return
  const zoom = mapInstance.getZoom?.() || 5
  const clusters = clusterPhotos(atlasPhotos.value, zoom)

  clusters.forEach(cluster => {
    const badgeHTML = cluster.count > 1 ? '<span class="photo-cluster-badge">' + cluster.count + '</span>' : ''
    const markerHTML = '<div class="photo-atlas-pin" data-cluster-id="' + cluster.id + '">' +
      '<div class="photo-atlas-pin-box">' +
        '<img src="' + cluster.cover.thumb_url + '?size=120" class="photo-atlas-pin-img" loading="lazy" />' +
      '</div>' +
      badgeHTML +
      '<div class="photo-atlas-pin-triangle"></div>' +
    '</div>'

    if (selectedMapProvider.value === 'amap') {
      const marker = addAMapOverlay(new mapAPI.Marker({
        position: [cluster.lng, cluster.lat],
        content: markerHTML,
        offset: new mapAPI.Pixel(-22, -50),
        zIndex: 140 + (cluster.count > 1 ? 20 : 0)
      }))
      marker.on?.('click', () => handlePhotoClusterClick(cluster))
      atlasPhotoMarkers.push(marker)
    } else {
      const pt = new mapAPI.Point(cluster.lng, cluster.lat)
      const label = new mapAPI.Label(markerHTML, {
        position: pt,
        offset: new mapAPI.Size(-22, -50)
      })
      label.setStyle({
        backgroundColor: 'transparent',
        border: 'none',
        padding: '0',
        cursor: 'pointer'
      })
      label.addEventListener?.('click', () => handlePhotoClusterClick(cluster))
      mapInstance.addOverlay(label)
      atlasPhotoMarkers.push(label)
    }
  })
}

function handlePhotoClusterClick(cluster: PhotoCluster) {
  selectedPhotoCluster.value = cluster
  if (cluster.count > 1) {
    const currentZoom = mapInstance?.getZoom?.() || 8
    if (currentZoom < 17) {
      if (selectedMapProvider.value === 'amap') {
        mapInstance?.setZoomAndCenter?.(Math.min(currentZoom + 2, 17), [cluster.lng, cluster.lat])
      } else {
        mapInstance?.centerAndZoom?.(new mapAPI.Point(cluster.lng, cluster.lat), Math.min(currentZoom + 2, 17))
      }
    }
  } else {
    openPhotoPreview(cluster.cover)
  }
}

function openPhotoPreview(photo: PhotoAtlasItem, list?: PhotoAtlasItem[]) {
  let activeList = list
  if (!activeList || !activeList.length) {
    if (selectedPhotoCluster.value && selectedPhotoCluster.value.photos.some(p => p.id === photo.id)) {
      activeList = selectedPhotoCluster.value.photos
    } else {
      activeList = atlasPhotos.value
    }
  }
  previewPhotoList.value = activeList && activeList.length ? activeList : [photo]
  const idx = previewPhotoList.value.findIndex(p => p.id === photo.id)
  setPreviewIndex(idx >= 0 ? idx : 0)
}

function setPreviewIndex(idx: number) {
  if (idx < 0 || idx >= previewPhotoList.value.length) return
  previewPhotoIndex.value = idx
  const current = previewPhotoList.value[idx]
  if (!current) return

  if (loadedPhotoMap.value[current.id]) {
    previewPhotoLoading.value = false
    previewPhotoError.value = false
  } else {
    previewPhotoLoading.value = true
    previewPhotoError.value = false
  }
  preloadNeighborPhotos(idx)
}

function onPreviewPhotoLoaded(id: string) {
  if (previewPhoto.value?.id === id) {
    previewPhotoLoading.value = false
    previewPhotoError.value = false
  }
  loadedPhotoMap.value[id] = true
}

function onPreviewPhotoError(id: string) {
  if (previewPhoto.value?.id === id) {
    previewPhotoLoading.value = false
    previewPhotoError.value = true
  }
}

function preloadNeighborPhotos(idx: number) {
  const list = previewPhotoList.value
  if (!list.length) return
  const nextIdx = (idx + 1) % list.length
  const prevIdx = (idx - 1 + list.length) % list.length
  const idsToPreload = [list[nextIdx]?.id, list[prevIdx]?.id].filter(Boolean)
  idsToPreload.forEach(id => {
    if (id && !loadedPhotoMap.value[id]) {
      const img = new Image()
      img.onload = () => { loadedPhotoMap.value[id] = true }
      img.src = '/api/v1/photos/' + id + '/file'
    }
  })
}

function closePhotoPreview() {
  previewPhotoIndex.value = -1
  previewPhotoList.value = []
  previewPhotoLoading.value = false
  previewPhotoError.value = false
}

function prevPreviewPhoto() {
  if (!previewPhotoList.value.length) return
  const nextIdx = previewPhotoIndex.value > 0 ? previewPhotoIndex.value - 1 : previewPhotoList.value.length - 1
  setPreviewIndex(nextIdx)
}

function nextPreviewPhoto() {
  if (!previewPhotoList.value.length) return
  const nextIdx = previewPhotoIndex.value < previewPhotoList.value.length - 1 ? previewPhotoIndex.value + 1 : 0
  setPreviewIndex(nextIdx)
}

let lightboxTouchStartX = 0
let lightboxTouchStartY = 0
let lightboxTouchDeltaX = 0
let lightboxTouchDeltaY = 0

function handleLightboxTouchStart(e: TouchEvent) {
  if (!e.touches || e.touches.length !== 1) return
  lightboxTouchStartX = e.touches[0].clientX
  lightboxTouchStartY = e.touches[0].clientY
  lightboxTouchDeltaX = 0
  lightboxTouchDeltaY = 0
}

function handleLightboxTouchMove(e: TouchEvent) {
  if (!e.touches || e.touches.length !== 1) return
  lightboxTouchDeltaX = e.touches[0].clientX - lightboxTouchStartX
  lightboxTouchDeltaY = e.touches[0].clientY - lightboxTouchStartY
}

function handleLightboxTouchEnd() {
  if (Math.abs(lightboxTouchDeltaX) > 40 && Math.abs(lightboxTouchDeltaX) > Math.abs(lightboxTouchDeltaY) * 1.1) {
    if (lightboxTouchDeltaX < 0) {
      nextPreviewPhoto()
    } else {
      prevPreviewPhoto()
    }
  }
  lightboxTouchDeltaX = 0
  lightboxTouchDeltaY = 0
}

function toggleAtlasPhotos(val?: boolean) {
  const next = typeof val === 'boolean' ? val : !atlasShowPhotos.value
  atlasShowPhotos.value = next
  localStorage.setItem('journeyin.atlasShowPhotos', next ? 'true' : 'false')
  renderAtlasPhotoMarkers()
  if (next && (!atlasPhotos.value || !atlasPhotos.value.length)) {
    void loadAtlasPhotos().then(() => renderAtlasPhotoMarkers())
  }
}

function formatPhotoTime(val?: string) {
  if (!val) return ''
  const d = new Date(val)
  if (isNaN(d.getTime())) return val
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0') + ' ' + String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0')
}

function navigateToAtlas() {
  historyOpen.value = false
  historyView.value = null
  selected.value = null
  tripDocument.value = null
  tripView.value = 'atlas'
  selectedStopId.value = ''
  selectedSubStopId.value = ''
  panelOpen.value = false
  mobileMapToolsOpen.value = false
  tripMenuID.value = ''
  detailMoreOpen.value = false
  resetMapSDK()
  void Promise.all([loadAtlasData(), loadPhotoStatus().then(() => loadAtlasPhotos())]).then(() => {
    void nextTick().then(() => renderAtlasMap())
  })
}

function selectAtlasTrip(item: AtlasTripItem) {
  if (selectedAtlasTripID.value === item.id) {
    clearSelectedAtlasTrip()
    return
  }
  selectedAtlasTripID.value = item.id
  void renderAtlasMap()
}

function clearSelectedAtlasTrip() {
  selectedAtlasTripID.value = ''
  void renderAtlasMap()
}

async function gotoTripFromAtlas(tripID: string) {
  const trip = trips.value.find(t => t.id === tripID)
  if (trip) {
    navigateToTrip(trip)
  } else {
    try {
      const resp = await apiFetch('/api/v1/trips/' + encodeURIComponent(tripID))
      if (!resp.ok) throw new Error('无法读取行程')
      const payload = (await resp.json()) as { document: TripDocument; revision: number; id: string; title: string; start_date: string; end_date: string; timezone: string }
      const summaryItem: TripSummary = {
        id: payload.id,
        title: payload.title,
        status: 'draft',
        start_date: payload.start_date,
        end_date: payload.end_date,
        timezone: payload.timezone,
        revision: payload.revision,
      }
      navigateToTrip(summaryItem)
    } catch (e) {
      error.value = e instanceof Error ? e.message : '跳转行程失败'
    }
  }
}

function getTripRouteMidpoint(tripItem: AtlasTripItem, provider: 'baidu' | 'amap'): Coord | null {
  let longestLeg: AtlasLegSummary | null = null
  let maxLen = 0
  for (const leg of tripItem.legs || []) {
    if (leg.geometry && leg.geometry.length >= 2) {
      const len = leg.distance_m || leg.geometry.length
      if (len > maxLen) {
        maxLen = len
        longestLeg = leg
      }
    }
  }
  if (longestLeg && longestLeg.geometry?.length) {
    const midIdx = Math.floor(longestLeg.geometry.length / 2)
    const val = longestLeg.geometry[midIdx]
    return mapRoutePointFor(val, longestLeg.crs || (provider === 'amap' ? 'gcj02' : 'bd09ll'), provider)
  }
  const firstStop = tripItem.key_stops?.[0]
  if (firstStop?.point) {
    const pt = pointForProvider({ location: { preferred: firstStop.point.crs || 'gcj02', coordinates: { [firstStop.point.crs || 'gcj02']: firstStop.point } } } as any, provider)
    return pt || firstStop.point
  }
  return null
}

function updateAtlasLabelsVisibility() {
  if (tripView.value !== 'atlas' || !mapInstance || !mapAPI) return
  if (!atlasStopMarkers.length && !atlasTripLabels.length) return

  const mode = mapLabelMode.value
  const hasSelection = Boolean(selectedAtlasTripID.value)
  const isAMap = selectedMapProvider.value === 'amap'
  const containerW = mapContainer.value?.clientWidth || window.innerWidth
  const rect = mapVisibleRect()
  const rightLimit = rect ? rect.right : containerW

  // 1. 轨迹路线名称标签显隐与高亮
  for (const item of atlasTripLabels) {
    const tripItem = item.__atlasTrip as AtlasTripItem
    const isSelected = selectedAtlasTripID.value === tripItem?.id
    const shouldShow = mode !== 'none' && (!hasSelection || isSelected)
    const baseColor = item.__atlasColor || '#24695c'

    if (isAMap) {
      if (!shouldShow) {
        item.hide?.()
      } else {
        item.show?.()
        item.setZIndex?.(isSelected ? 100 : 40)
        item.setStyle?.({
          backgroundColor: baseColor,
          color: '#ffffff',
          border: isSelected ? '1.5px solid #ffffff' : '1px solid rgba(255,255,255,0.4)',
          borderRadius: '999px',
          padding: '4px 10px',
          fontSize: '11px',
          fontWeight: '700',
          lineHeight: '15px',
          maxWidth: '220px',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          boxShadow: isSelected ? '0 0 0 2px #ffffff, 0 4px 16px rgba(0,0,0,0.45)' : '0 3px 10px rgba(0,0,0,0.3)',
          cursor: 'pointer',
          opacity: hasSelection && !isSelected ? '0.35' : '0.95',
        })
      }
    } else {
      // Baidu
      if (!shouldShow) {
        item.hide?.()
      } else {
        item.show?.()
        item.setStyle?.({
          backgroundColor: baseColor,
          color: '#ffffff',
          border: isSelected ? '1.5px solid #ffffff' : '1px solid rgba(255,255,255,0.4)',
          borderRadius: '999px',
          padding: '4px 10px',
          fontSize: '11px',
          fontWeight: '700',
          lineHeight: '15px',
          maxWidth: '220px',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          boxShadow: isSelected ? '0 0 0 2px #ffffff, 0 4px 16px rgba(0,0,0,0.45)' : '0 3px 10px rgba(0,0,0,0.3)',
          cursor: 'pointer',
          opacity: hasSelection && !isSelected ? 0.35 : 0.95,
          zIndex: isSelected ? 100 : 40,
        })
      }
    }
  }

  // 2. 关键规划点标签显示计算 (避让冲突)
  const visibleStopIDs = new Set<string>()
  if (mode !== 'none') {
    if (mode === 'always') {
      for (const m of atlasStopMarkers) {
        const stop = m.__atlasStop as AtlasStopSummary
        const tripItem = m.__atlasTrip as AtlasTripItem
        const isSelected = selectedAtlasTripID.value === tripItem?.id
        if (!hasSelection || isSelected) {
          visibleStopIDs.add(stop.id)
        }
      }
    } else {
      // auto 模式：根据屏幕像素距离避让
      const displayedPixels: { x: number; y: number }[] = []

      if (hasSelection) {
        for (const m of atlasStopMarkers) {
          const stop = m.__atlasStop as AtlasStopSummary
          const tripItem = m.__atlasTrip as AtlasTripItem
          if (tripItem?.id === selectedAtlasTripID.value) {
            visibleStopIDs.add(stop.id)
            if (stop.point) {
              const pt = pointForProvider({ location: { preferred: stop.point.crs || 'gcj02', coordinates: { [stop.point.crs || 'gcj02']: stop.point } } } as any, selectedMapProvider.value) || stop.point
              const pix = getMapPointScreenPixel(pt)
              if (pix) displayedPixels.push(pix)
            }
          }
        }
      } else {
        for (const m of atlasStopMarkers) {
          const stop = m.__atlasStop as AtlasStopSummary
          if (!stop.point) continue
          const pt = pointForProvider({ location: { preferred: stop.point.crs || 'gcj02', coordinates: { [stop.point.crs || 'gcj02']: stop.point } } } as any, selectedMapProvider.value) || stop.point
          if (!pt) continue
          const pix = getMapPointScreenPixel(pt)
          if (!pix) {
            visibleStopIDs.add(stop.id)
            continue
          }

          let conflict = false
          for (const occupied of displayedPixels) {
            const dist = Math.hypot(pix.x - occupied.x, pix.y - occupied.y)
            if (dist < 55) {
              conflict = true
              break
            }
          }

          if (!conflict) {
            visibleStopIDs.add(stop.id)
            displayedPixels.push(pix)
          }
        }
      }
    }
  }

  // 3. 应用关键规划点标签与边缘自适应
  for (const m of atlasStopMarkers) {
    const stop = m.__atlasStop as AtlasStopSummary
    const tripItem = m.__atlasTrip as AtlasTripItem
    const isSelected = selectedAtlasTripID.value === tripItem?.id
    const baseColor = m.__atlasColor || '#24695c'
    const shouldShow = visibleStopIDs.has(stop.id)

    if (isAMap) {
      if (!shouldShow) {
        try { m.setLabel({ content: '' }) } catch {}
      } else {
        const highlightBorder = isSelected ? 'border: 1.5px solid #ffffff;' : 'border: 1px solid ' + baseColor + ';'
        const labelContent = '<span style="display:inline-block;padding:3px 8px;border-radius:6px;background:#173f47ee;color:#fff;font-size:11px;font-weight:700;box-shadow:0 3px 10px rgba(0,0,0,0.3);' + highlightBorder + 'white-space:nowrap;cursor:pointer;">' + escapeHTML(stop.title) + '</span>'
        let dir: 'right' | 'left' = 'right'
        let off = new mapAPI.Pixel(10, -6)
        try {
          const pos = m.getPosition?.()
          if (pos && mapInstance?.lngLatToContainer) {
            const px = mapInstance.lngLatToContainer(pos)
            const screenX = Array.isArray(px) ? px[0] : typeof px?.getX === 'function' ? px.getX() : px?.x
            if (typeof screenX === 'number' && screenX > rightLimit - 170) {
              dir = 'left'
              off = new mapAPI.Pixel(-10, -6)
            }
          }
        } catch {}
        m.setLabel({ content: labelContent, direction: dir, offset: off })
      }
    } else {
      // Baidu
      if (!shouldShow) {
        try { m.setLabel(null) } catch {}
      } else {
        let off = new mapAPI.Size(14, -18)
        try {
          const pos = m.getPosition?.()
          if (pos && mapInstance?.pointToPixel) {
            const px = mapInstance.pointToPixel(pos)
            const screenX = typeof px?.x === 'number' ? px.x : null
            if (typeof screenX === 'number' && screenX > rightLimit - 170) {
              const estWidth = Math.max(60, stop.title.length * 12 + 20)
              off = new mapAPI.Size(-estWidth - 10, -18)
            }
          }
        } catch {}
        const label = new mapAPI.Label(stop.title, { offset: off })
        label.setStyle?.({
          color: '#fff',
          backgroundColor: '#173f47ee',
          border: isSelected ? '1.5px solid #ffffff' : '1px solid ' + baseColor,
          borderRadius: '6px',
          padding: '3px 7px',
          fontSize: '11px',
          fontWeight: '700',
          cursor: 'pointer'
        })
        m.setLabel(label)
      }
    }
  }
}

async function renderAtlasMap(preserveView = false) {
  if (tripView.value !== 'atlas' || !mapContainer.value) return
  const currentKey = selectedMapProvider.value === 'amap' ? amapKey.value.trim() : baiduKey.value.trim()
  if (!currentKey) {
    resetMapSDK()
    return
  }
  if (selectedMapProvider.value === 'amap') {
    await renderAtlasAMap(preserveView)
  } else {
    await renderAtlasBaidu(preserveView)
  }
}

async function renderAtlasAMap(preserveView = false) {
  if (!mapContainer.value) return
  await loadAMap()
  if (!mapAPI || typeof mapAPI.Map !== 'function') return
  if (!mapInstance || loadedMapKey) {
    if (mapInstance) { try { mapInstance.destroy?.() } catch {} }
    mapReady.value = false
    mapInstance = new mapAPI.Map(mapContainer.value, { viewMode: '2D', zoom: 5, center: [105, 35], resizeEnable: true })
    mapInstance.on?.('complete', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
    mapInstance.on?.('zoomend', handleMapZoomChange)
    loadedMapKey = ''
  }
  mapInstance.resize?.()
  clearAMapOverlays()
  atlasStopMarkers = []
  atlasTripLabels = []
  atlasPhotoMarkers = []

  const allPoints: any[] = []
  const tripsToRender = atlasShowTrips.value ? (atlasData.value?.trips || []) : []

  tripsToRender.forEach((tripItem, tIdx) => {
    const isSelected = selectedAtlasTripID.value === tripItem.id
    const hasSelection = Boolean(selectedAtlasTripID.value)
    const baseColor = ATLAS_PALETTE[tIdx % ATLAS_PALETTE.length]
    const strokeColor = hasSelection ? (isSelected ? baseColor : '#94a3b8') : baseColor
    const strokeOpacity = hasSelection ? (isSelected ? 0.95 : 0.25) : 0.82
    const strokeWeight = hasSelection ? (isSelected ? 6 : 3) : 5
    const zIndex = isSelected ? 80 : 30

    // 1. 绘制路线
    tripItem.legs.forEach(leg => {
      if (!leg.geometry || leg.geometry.length < 2) return
      const line = leg.geometry.map(val => {
        const pt = mapRoutePointFor(val, leg.crs || 'gcj02', 'amap')
        return pt ? amapPointToArray(pt) : null
      }).filter(Boolean)

      if (line.length >= 2) {
        if (!hasSelection || isSelected) {
          line.forEach(p => allPoints.push(p))
        }
        const polyline = addAMapOverlay(new mapAPI.Polyline({
          path: line,
          strokeColor,
          strokeWeight,
          strokeOpacity,
          lineJoin: 'round',
          showDir: false,
          zIndex,
        }))
        polyline.on?.('click', () => {
          selectAtlasTrip(tripItem)
        })
      }
    })

    // 2. 绘制代表性地标节点
    for (const stop of tripItem.key_stops || []) {
      if (!stop.point) continue
      const pt = pointForProvider({ location: { coordinates: { [stop.point.crs || 'gcj02']: stop.point }, preferred: stop.point.crs || 'gcj02' } } as any, 'amap')
      if (!pt) continue
      const mapPt = amapPointToArray(pt)
      allPoints.push(mapPt)
      
      const markerContent = '<div style="display:flex;align-items:center;justify-content:center;width:22px;height:22px;border-radius:50%;background:' + baseColor + ';color:#fff;font-size:11px;font-weight:800;border:2px solid #fff;box-shadow:0 3px 8px rgba(0,0,0,0.3);cursor:pointer;">' + stop.day_index + '</div>'
      const marker = addAMapOverlay(new mapAPI.Marker({
        position: mapPt,
        title: tripItem.title + ' · ' + stop.title,
        content: markerContent,
        offset: new mapAPI.Pixel(-11, -11),
        zIndex: zIndex + 5,
      }))
      marker.__atlasStop = stop
      marker.__atlasTrip = tripItem
      marker.__atlasColor = baseColor
      marker.on?.('click', () => {
        selectAtlasTrip(tripItem)
      })
      atlasStopMarkers.push(marker)
    }

    // 3. 绘制旅程轨迹名称标签 (Trip Title Badge)
    const midPoint = getTripRouteMidpoint(tripItem, 'amap')
    if (midPoint) {
      const mapMid = amapPointToArray(midPoint)
      allPoints.push(mapMid)
      const textOverlay = addAMapOverlay(new mapAPI.Text({
        text: '✦ ' + tripItem.title,
        position: mapMid,
        anchor: 'center',
        zIndex: zIndex + 10,
        style: {
          backgroundColor: baseColor,
          color: '#ffffff',
          border: isSelected ? '1.5px solid #ffffff' : '1px solid rgba(255,255,255,0.4)',
          borderRadius: '999px',
          padding: '4px 10px',
          fontSize: '11px',
          fontWeight: '700',
          lineHeight: '15px',
          maxWidth: '220px',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          boxShadow: isSelected ? '0 0 0 2px #ffffff, 0 4px 16px rgba(0,0,0,0.45)' : '0 3px 10px rgba(0,0,0,0.3)',
          cursor: 'pointer',
          opacity: hasSelection && !isSelected ? '0.35' : '0.95',
        }
      }))
      textOverlay.__atlasTrip = tripItem
      textOverlay.__atlasColor = baseColor
      textOverlay.on?.('click', () => {
        selectAtlasTrip(tripItem)
      })
      atlasTripLabels.push(textOverlay)
    }
  })

  updateAtlasLabelsVisibility()
  renderAtlasPhotoMarkers()

  if (allPoints.length === 0 && atlasShowPhotos.value && atlasPhotos.value.length > 0) {
    atlasPhotos.value.slice(0, 50).forEach(p => allPoints.push([p.lng, p.lat]))
  }

  if (!preserveView) {
    if (allPoints.length > 0) {
      try {
        const rect = mapVisibleRect()
        const container = mapContainer.value
        const fullW = container?.clientWidth || 0
        const fullH = container?.clientHeight || 0
        const isMobile = isMobileViewport()
        let padTop = 32, padBottom = 32, padLeft = 32, padRight = 32
        if (isMobile) {
          padTop = rect ? Math.max(20, Math.round(rect.top + 8)) : 60
          padBottom = rect ? (sheetBreakpoint.value === 'expanded' ? 40 : Math.max(24, Math.round((fullH - rect.bottom) + 12))) : 60
          padLeft = 24
          padRight = 24
        } else {
          padLeft = rect ? Math.max(36, Math.round(rect.left)) : 60
          padRight = rect ? Math.max(48, Math.round(fullW - rect.right + 24)) : 60
          padTop = 32
          padBottom = 32
        }
        // 高德 JSAPI 2.0 避让参数 avoid 为：[上, 下, 左, 右]
        mapInstance.setFitView?.(mapOverlays.filter(Boolean), false, [padTop, padBottom, padLeft, padRight])
      } catch {
        mapInstance.setCenter?.(allPoints[0])
      }
    } else {
      mapInstance.setCenter?.([105, 35])
      mapInstance.setZoom?.(4)
    }
  }
  applyMapType()
}

async function renderAtlasBaidu(preserveView = false) {
  if (!mapContainer.value) return
  await loadBaiduMap()
  if (!mapAPI || typeof mapAPI.Map !== 'function') return
  if (!mapInstance) {
    mapReady.value = false
    mapInstance = new mapAPI.Map(mapContainer.value, { enableIconClick: false, fixCenterWhenResize: true })
    mapInstance.enableScrollWheelZoom()
    mapInstance.addEventListener?.('zoomend', handleMapZoomChange)
    mapInstance.addEventListener?.('tilesloaded', () => { mapReady.value = true; mapError.value = ''; mapWarning.value = '' })
  }
  mapInstance.clearOverlays()
  atlasStopMarkers = []
  atlasTripLabels = []
  atlasPhotoMarkers = []

  const allPoints: any[] = []
  const tripsToRender = atlasShowTrips.value ? (atlasData.value?.trips || []) : []

  tripsToRender.forEach((tripItem, tIdx) => {
    const isSelected = selectedAtlasTripID.value === tripItem.id
    const hasSelection = Boolean(selectedAtlasTripID.value)
    const baseColor = ATLAS_PALETTE[tIdx % ATLAS_PALETTE.length]
    const strokeColor = hasSelection ? (isSelected ? baseColor : '#94a3b8') : baseColor
    const strokeOpacity = hasSelection ? (isSelected ? 0.95 : 0.25) : 0.82
    const strokeWeight = hasSelection ? (isSelected ? 6 : 3) : 5
    const zIndex = isSelected ? 80 : 30

    tripItem.legs.forEach(leg => {
      if (!leg.geometry || leg.geometry.length < 2) return
      const line = leg.geometry.map(val => {
        const pt = mapRoutePoint(val, leg.crs || 'bd09ll')
        return pt ? new mapAPI.Point(pt.lng, pt.lat) : null
      }).filter(Boolean)

      if (line.length >= 2) {
        if (!hasSelection || isSelected) {
          line.forEach(p => allPoints.push(p))
        }
        const polyline = new mapAPI.Polyline(line, {
          strokeColor,
          strokeWeight,
          strokeOpacity,
        })
        polyline.addEventListener?.('click', () => {
          selectAtlasTrip(tripItem)
        })
        mapInstance.addOverlay(polyline)
      }
    })

    for (const stop of tripItem.key_stops || []) {
      if (!stop.point) continue
      const pt = mapPointFor({ location: { coordinates: { [stop.point.crs || 'bd09ll']: stop.point }, preferred: stop.point.crs || 'bd09ll' } } as any)
      if (!pt) continue
      const mapPt = new mapAPI.Point(pt.lng, pt.lat)
      allPoints.push(mapPt)
      const marker = new mapAPI.Marker(mapPt)
      marker.__atlasStop = stop
      marker.__atlasTrip = tripItem
      marker.__atlasColor = baseColor
      marker.addEventListener?.('click', () => {
        selectAtlasTrip(tripItem)
      })
      mapInstance.addOverlay(marker)
      atlasStopMarkers.push(marker)
    }

    // 3. 绘制旅程轨迹名称标签 (Trip Title Badge)
    const midPoint = getTripRouteMidpoint(tripItem, 'baidu')
    if (midPoint) {
      const mapMid = new mapAPI.Point(midPoint.lng, midPoint.lat)
      allPoints.push(mapMid)
      const labelText = '✦ ' + tripItem.title
      const estW = Math.min(220, Math.max(80, labelText.length * 11 + 24))
      const label = new mapAPI.Label(labelText, {
        position: mapMid,
        offset: new mapAPI.Size(-Math.round(estW / 2), -12)
      })
      label.setStyle?.({
        color: '#ffffff',
        backgroundColor: baseColor,
        border: isSelected ? '1.5px solid #ffffff' : '1px solid rgba(255,255,255,0.4)',
        borderRadius: '999px',
        padding: '4px 10px',
        fontSize: '11px',
        fontWeight: '700',
        lineHeight: '15px',
        maxWidth: '220px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        boxShadow: isSelected ? '0 0 0 2px #ffffff, 0 4px 16px rgba(0,0,0,0.45)' : '0 3px 10px rgba(0,0,0,0.3)',
        cursor: 'pointer',
        opacity: hasSelection && !isSelected ? 0.35 : 0.95,
        zIndex: zIndex + 10,
      })
      label.addEventListener?.('click', () => {
        selectAtlasTrip(tripItem)
      })
      mapInstance.addOverlay(label)
      label.__atlasTrip = tripItem
      label.__atlasColor = baseColor
      atlasTripLabels.push(label)
    }
  })

  updateAtlasLabelsVisibility()
  renderAtlasPhotoMarkers()

  if (allPoints.length === 0 && atlasShowPhotos.value && atlasPhotos.value.length > 0) {
    atlasPhotos.value.slice(0, 50).forEach(p => allPoints.push(new mapAPI.Point(p.lng, p.lat)))
  }

  if (!preserveView) {
    if (allPoints.length > 0) {
      try {
        const rect = mapVisibleRect()
        const container = mapContainer.value
        const fullW = container?.clientWidth || 0
        const fullH = container?.clientHeight || 0
        const isMobile = isMobileViewport()
        let padTop = 32, padBottom = 32, padLeft = 32, padRight = 32
        if (isMobile) {
          padTop = rect ? Math.max(20, Math.round(rect.top + 8)) : 60
          padBottom = rect ? (sheetBreakpoint.value === 'expanded' ? 40 : Math.max(24, Math.round((fullH - rect.bottom) + 12))) : 60
          padLeft = 24
          padRight = 24
        } else {
          padLeft = rect ? Math.max(36, Math.round(rect.left)) : 60
          padRight = rect ? Math.max(48, Math.round(fullW - rect.right + 24)) : 60
          padTop = 32
          padBottom = 32
        }
        // 百度 JSAPI 4.0 margins 参数为：[上, 右, 下, 左]
        const view = mapInstance.getViewport?.(allPoints, { margins: [padTop, padRight, padBottom, padLeft] })
        if (view) mapInstance.centerAndZoom(view.center, view.zoom)
      } catch {}
    } else {
      mapInstance.centerAndZoom('中国', 4)
    }
  }
  applyMapType()
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
    if (mapType.value === 'satellite') {
      if (!amapSatelliteLayer) {
        amapSatelliteLayer = new mapAPI.TileLayer.Satellite({ zIndex: 10 })
      }
      mapInstance.add?.(amapSatelliteLayer)
    } else if (amapSatelliteLayer) {
      mapInstance.remove?.(amapSatelliteLayer)
      amapSatelliteLayer = null
    }
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
function isMobileViewport(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(max-width: 900px)').matches
}

function stopDayBadge(stop: Stop | SubStop): string {
  if (!tripDocument.value?.days?.length) return ''
  const day = dayForStop(stop)
  if (!day) return ''
  const index = tripDocument.value.days.findIndex(d => d.id === day.id)
  return index >= 0 ? 'D' + (index + 1) : ''
}

function getMapPointScreenPixel(point: Coord): { x: number; y: number } | null {
  if (!mapInstance || !mapAPI) return null
  try {
    if (selectedMapProvider.value === 'amap') {
      const lnglat = Array.isArray(point) ? new mapAPI.LngLat(Number(point[0]), Number(point[1])) : new mapAPI.LngLat(Number(point.lng), Number(point.lat))
      const pixel = mapInstance.lngLatToContainer?.(lnglat)
      if (Array.isArray(pixel)) return { x: Number(pixel[0]), y: Number(pixel[1]) }
      if (pixel && typeof pixel.getX === 'function') return { x: Number(pixel.getX()), y: Number(pixel.getY()) }
      return pixel && typeof pixel.x === 'number' ? { x: pixel.x, y: pixel.y } : null
    }
    const pt = new mapAPI.Point(point.lng, point.lat)
    const pixel = mapInstance.pointToPixel?.(pt)
    return pixel && typeof pixel.x === 'number' ? { x: pixel.x, y: pixel.y } : null
  } catch {
    return null
  }
}

function computeVisibleLabelStopIDs(stops: (Stop | SubStop)[]): Set<string> {
  const visibleIDs = new Set<string>()
  if (mapLabelMode.value === 'none') {
    return visibleIDs
  }
  if (mapLabelMode.value === 'always' || !isMobileViewport()) {
    for (const stop of stops) visibleIDs.add(stop.id)
    return visibleIDs
  }

  // 手机端 auto 模式：有空间就显示，空间不够就优化！
  const targetID = selectedTarget.value?.id
  if (targetID) {
    visibleIDs.add(targetID)
  }

  // 如果点数较少（<= 7 个点），视野开阔，全量显示
  if (stops.length <= 7) {
    for (const stop of stops) visibleIDs.add(stop.id)
    return visibleIDs
  }

  // 点数较多时，通过真实屏幕像素距离进行几何避让
  const displayedPixels: { x: number; y: number }[] = []
  if (targetID) {
    const targetStop = stops.find(s => s.id === targetID)
    if (targetStop) {
      const pt = pointForProvider(targetStop, selectedMapProvider.value)
      if (pt) {
        const pix = getMapPointScreenPixel(pt)
        if (pix) displayedPixels.push(pix)
      }
    }
  }

  for (const stop of stops) {
    if (stop.id === targetID) continue
    const pt = pointForProvider(stop, selectedMapProvider.value)
    if (!pt) continue
    const pix = getMapPointScreenPixel(pt)
    if (!pix) {
      visibleIDs.add(stop.id)
      continue
    }

    let conflict = false
    for (const occupied of displayedPixels) {
      const dist = Math.hypot(pix.x - occupied.x, pix.y - occupied.y)
      if (dist < 60) {
        conflict = true
        break
      }
    }

    if (!conflict) {
      visibleIDs.add(stop.id)
      displayedPixels.push(pix)
    }
  }

  return visibleIDs
}

function shouldShowRouteLabel(distanceM: number | undefined, isLegSelected: boolean): boolean {
  if (mapLabelMode.value === 'none') return false
  if (mapLabelMode.value === 'always') return true
  if (isLegSelected) return true
  if (!isMobileViewport()) return true
  // 短程行程（< 500m）在小屏下避让，长途行程正常显示
  const isShortDistance = typeof distanceM === 'number' && distanceM > 0 && distanceM < 500
  return !isShortDistance
}

function updateStopLabelsVisibility() {
  if (!mapInstance || !mapAPI || !currentStopMarkers.length) return
  const visibleLabelIDs = computeVisibleLabelStopIDs(mapStops.value)
  const childVisibleIDs = selectedStop.value?.children?.length
    ? computeVisibleLabelStopIDs(selectedStop.value.children)
    : new Set<string>()

  const isAMap = selectedMapProvider.value === 'amap'
  for (const marker of currentStopMarkers) {
    const isSubStop = Boolean(marker.__journeyinIsSubStop)
    const stopId = isSubStop ? marker.__journeyinSubStopId : marker.__journeyinStopId
    if (!stopId) continue
    const isTarget = selectedTarget.value?.id === stopId
    const shouldShow = isSubStop ? childVisibleIDs.has(stopId) : visibleLabelIDs.has(stopId)
    const title = marker.__journeyinTitle || ''
    const badge = marker.__journeyinBadge || ''
    if (isAMap) {
      attachAMapLabel(marker, title, badge, isTarget, shouldShow)
    } else {
      attachMapLabel(marker, title, badge, isTarget, shouldShow)
    }
  }
}

let mapZoomDebounceTimer: number | null = null
function handleMapZoomChange() {
  if (tripView.value === 'atlas') {
    if (mapZoomDebounceTimer !== null) window.clearTimeout(mapZoomDebounceTimer)
    mapZoomDebounceTimer = window.setTimeout(() => {
      if (mapLabelMode.value === 'auto') updateAtlasLabelsVisibility()
      renderAtlasPhotoMarkers()
    }, 120)
    return
  }
  if (mapLabelMode.value !== 'auto') return
  if (mapZoomDebounceTimer !== null) window.clearTimeout(mapZoomDebounceTimer)
  mapZoomDebounceTimer = window.setTimeout(() => {
    if (isMobileViewport()) updateStopLabelsVisibility()
  }, 120)
}

function setMapLabelMode(mode: MapLabelMode) {
  mapLabelMode.value = mode
  localStorage.setItem('journeyin.mapLabelMode', mode)
  localStorage.setItem('journeyin.mapLabels', mode === 'none' ? 'false' : 'true')
  if (tripView.value === 'atlas') {
    updateAtlasLabelsVisibility()
  } else {
    void renderMap({ preserveView: true })
  }
}

function toggleMapLabels() {
  const nextMode: MapLabelMode = mapLabelMode.value === 'none' ? 'auto' : 'none'
  setMapLabelMode(nextMode)
}
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

async function fetchAddressForPoint(point: Coord & { crs: string }, provider: 'baidu' | 'amap'): Promise<{ address: string; name: string }> {
  try {
    const resp = await apiFetch('/api/v1/maps/reverse-geocode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        provider,
        location: { lat: point.lat, lng: point.lng, crs: point.crs }
      })
    })
    if (!resp.ok) return { address: '', name: '' }
    const payload = await resp.json() as { address?: string; name?: string }
    return { address: payload.address || '', name: payload.name || '' }
  } catch {
    return { address: '', name: '' }
  }
}

function promptAddMapPoint(info: { point: Coord & { crs: string }; title: string; address?: string; provider: 'baidu' | 'amap' }) {
  if (tripView.value !== 'detail' || readOnlyView.value || !selected.value || !tripDocument.value) return
  const target = mapPickTargetID.value ? findPlanningPoint(mapPickTargetID.value) : null
  mapPickLocation.value = info.point
  const initialTitle = target?.title || (info.title && info.title !== '地图地点' ? info.title : '')
  mapPickTitle.value = initialTitle
  mapPickAddress.value = target?.address || info.address || ''

  if (selectedStop.value && !selectedSubStopId.value && !mapPickTargetID.value) {
    mapPickKind.value = 'substop'
    mapPickParentStopId.value = selectedStop.value.id
    const parentDay = dayForStop(selectedStop.value)
    mapPickDayID.value = parentDay?.id || tripDocument.value.days[0]?.id || ''
  } else {
    mapPickKind.value = 'stop'
    const day = target ? dayForStop(target) : selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]
    mapPickDayID.value = day?.id || tripDocument.value.days[0]?.id || ''
    mapPickParentStopId.value = (day?.stops && day.stops[0]?.id) || ''
  }

  mapPickMode.value = false
  mapPickOpen.value = true
  error.value = ''

  void fetchAddressForPoint(info.point, info.provider).then(res => {
    if (mapPickOpen.value) {
      if (!mapPickAddress.value && res.address) mapPickAddress.value = res.address
      if ((!mapPickTitle.value.trim() || mapPickTitle.value === '地图地点') && res.name) {
        mapPickTitle.value = res.name
      }
    }
  })
}

function handleAMapHotspotClick(event: any) {
  if (tripView.value !== 'detail' || readOnlyView.value || !selected.value || !tripDocument.value) return
  const lnglat = event?.lnglat
  const lng = Number(lnglat?.lng ?? lnglat?.getLng?.())
  const lat = Number(lnglat?.lat ?? lnglat?.getLat?.())
  if (!Number.isFinite(lng) || !Number.isFinite(lat)) return

  const point = { lat, lng, crs: 'gcj02' }
  const name = String(event?.name || '').trim() || '地图地点'

  promptAddMapPoint({
    point,
    title: name,
    provider: 'amap'
  })
}

const BAIDU_MCBAND = [12890594.86, 8362377.87, 5591021, 3481989.83, 1678043.12, 0]
const BAIDU_MC2LL = [
  [1.410526172116255e-8, 0.00000898305509648872, -1.9939833816331, 200.9824383106796, -187.2403703815547, 91.6087516669843, -23.38765649603339, 2.57121317296198, -0.03801003308653, 17337981.2],
  [-7.435856389565537e-9, 0.000008983055097726239, -0.78625201886289, 96.32687599759846, -1.85204757529826, -59.36935905485877, 47.40033549296737, -16.50741931063887, 2.28786674699375, 10260144.86],
  [-3.030883460898826e-8, 0.00000898305509983578, 0.30071316287616, 59.74293618442277, 7.357984074871, -25.38371002664745, 13.45380521110908, -3.29883767235584, 0.32710905363475, 6856817.37],
  [-1.981981304930552e-8, 0.000008983055099779535, 0.03278182852591, 40.31678527705744, 0.65659298677277, -4.44255534477492, 0.85341911805263, 0.12923347998204, -0.04625736007561, 4482777.06],
  [3.09191371068437e-9, 0.000008983055096812155, 0.00006995724062, 23.10934304144901, -0.00023663490511, -0.6321817810242, -0.00663494467273, 0.03430082397953, -0.00466043876332, 2555164.4],
  [2.890871144776878e-9, 0.000008983055095805407, -3.068298e-8, 7.47137025468032, -0.00000353937994, -0.02145144861037, -0.00001234426596, 0.00010322952773, -0.00000323890364, 826088.5]
]

function baiduConvertMC2LL(mcLng: number, mcLat: number): { lng: number; lat: number } {
  if (mapInstance && typeof mapInstance.mercatorToLnglat === 'function') {
    try {
      const res = mapInstance.mercatorToLnglat(mcLng, mcLat)
      if (Array.isArray(res) && res.length >= 2 && Number.isFinite(res[0]) && Number.isFinite(res[1])) {
        return { lng: Number(res[0]), lat: Number(res[1]) }
      }
    } catch {}
  }
  if (mapInstance && typeof mapInstance.mercatorToLngLat === 'function') {
    try {
      const res = mapInstance.mercatorToLngLat(mcLng, mcLat)
      if (Array.isArray(res) && res.length >= 2 && Number.isFinite(res[0]) && Number.isFinite(res[1])) {
        return { lng: Number(res[0]), lat: Number(res[1]) }
      }
    } catch {}
  }
  const absLng = Math.abs(mcLng)
  const absLat = Math.abs(mcLat)
  let nl: number[] | null = null
  for (let t = 0; t < BAIDU_MCBAND.length; t++) {
    if (absLat >= BAIDU_MCBAND[t]) {
      nl = BAIDU_MC2LL[t]
      break
    }
  }
  if (!nl) return { lng: 0, lat: 0 }
  const e = nl[0] + nl[1] * absLng
  const i = absLat / nl[9]
  const l = nl[2] + nl[3] * i + nl[4] * i * i + nl[5] * i * i * i + nl[6] * i * i * i * i + nl[7] * i * i * i * i * i + nl[8] * i * i * i * i * i * i
  return {
    lng: e * (mcLng < 0 ? -1 : 1),
    lat: l * (mcLat < 0 ? -1 : 1)
  }
}

function handleBaiduSpotClick(event: any) {
  if (tripView.value !== 'detail' || readOnlyView.value || !selected.value || !tripDocument.value) return
  const spot = event?.spots?.[0]
  if (!spot) return
  const pt = spot.point || spot.pt
  if (!pt) return
  let lng = Number(pt.lng)
  let lat = Number(pt.lat)
  if (Math.abs(lng) > 180 || Math.abs(lat) > 90) {
    const converted = baiduConvertMC2LL(lng, lat)
    lng = converted.lng
    lat = converted.lat
  }
  if (!Number.isFinite(lng) || !Number.isFinite(lat)) return

  let name = ''
  if (typeof mapInstance?.getIconByClickPosition === 'function' && event.pixel) {
    try {
      const icon = mapInstance.getIconByClickPosition(event.pixel)
      if (icon?.name) name = String(icon.name).trim()
    } catch {}
  }
  if (!name) {
    const raw = String(spot.n || spot.userdata?.name || spot.name || spot.title || event?.name || event?.title || '').replace(/<[^>]+>/g, '').trim()
    if (raw && raw !== '地图地点') name = raw
  }

  promptAddMapPoint({
    point: { lat, lng, crs: 'bd09ll' },
    title: name,
    provider: 'baidu'
  })

  const uid = spot.userdata?.uid || spot.uid
  if (uid && typeof mapInstance?.getPoiByUid === 'function') {
    try {
      mapInstance.getPoiByUid(uid, (poi: any) => {
        if (mapPickOpen.value && (!mapPickTitle.value.trim() || mapPickTitle.value === '地图地点')) {
          const poiName = String(poi?.name || poi?.title || '').trim()
          if (poiName) mapPickTitle.value = poiName
        }
      })
    } catch {}
  }
}

function handleMapClick(event: any) {
  if (readOnlyView.value || !mapPickMode.value || !event?.point || !tripDocument.value) return
  let lat = Number(event.point.lat)
  let lng = Number(event.point.lng)
  if (selectedMapProvider.value === 'baidu' && (Math.abs(lng) > 180 || Math.abs(lat) > 90)) {
    const converted = baiduConvertMC2LL(lng, lat)
    lng = converted.lng
    lat = converted.lat
  }
  const point = { lat, lng, crs: selectedMapProvider.value === 'amap' ? 'gcj02' : 'bd09ll' }
  if (!Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return
  const target = mapPickTargetID.value ? findPlanningPoint(mapPickTargetID.value) : null
  mapPickLocation.value = point
  mapPickTitle.value = target?.title || ''
  mapPickAddress.value = target?.address || ''

  if (selectedStop.value && !selectedSubStopId.value && !mapPickTargetID.value) {
    mapPickKind.value = 'substop'
    mapPickParentStopId.value = selectedStop.value.id
    const parentDay = dayForStop(selectedStop.value)
    mapPickDayID.value = parentDay?.id || tripDocument.value.days[0]?.id || ''
  } else {
    mapPickKind.value = 'stop'
    const day = target ? dayForStop(target) : selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]
    mapPickDayID.value = day?.id || tripDocument.value.days[0]?.id || ''
    mapPickParentStopId.value = (day?.stops && day.stops[0]?.id) || ''
  }

  mapPickMode.value = false
  mapPickOpen.value = true
  if (!mapPickAddress.value) {
    void fetchAddressForPoint(point, selectedMapProvider.value).then(res => {
      if (mapPickOpen.value) {
        if (!mapPickAddress.value && res.address) mapPickAddress.value = res.address
        if (!mapPickTitle.value.trim() && res.name) mapPickTitle.value = res.name
      }
    })
  }
}

function cancelMapPick() {
  mapPickOpen.value = false
  mapPickMode.value = false
  mapPickTargetID.value = ''
  mapPickDayID.value = ''
  mapPickKind.value = 'stop'
  mapPickParentStopId.value = ''
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
  const addedTitle = mapPickTitle.value.trim()
  try {
    if (mapPickTargetID.value) {
      const target = findPlanningPoint(mapPickTargetID.value)
      if (!target) throw new Error('找不到要更新的规划点')
      await persistPlanningPointUpdate(target, { title: addedTitle, address: mapPickAddress.value.trim(), location: locationForMapPoint(point, provider) })
      cancelMapPick()
      return
    }
    if (!mapPickDayID.value) { error.value = '请选择行程日期'; return }

    if (mapPickKind.value === 'substop') {
      const parentID = mapPickParentStopId.value
      if (!parentID) { error.value = '请选择所属主规划点'; return }
      const endpoint = '/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(mapPickDayID.value) + '/stops/' + encodeURIComponent(parentID) + '/children'
      const response = await apiFetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision },
        body: JSON.stringify({ stop: { title: addedTitle, address: mapPickAddress.value.trim(), location: locationForMapPoint(point, provider) } })
      })
      const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
      if (!response.ok) throw new Error(payload.error?.message || '保存子规划点失败')
      applyTripPayload(payload)
      selectedDay.value = tripDocument.value.days.findIndex(day => day.id === mapPickDayID.value) + 1
      selectedStopId.value = parentID
      selectedSubStopId.value = ''
      const parentTitle = findPlanningPoint(parentID)?.title || '主规划点'
      cancelMapPick()
      tripDetailsNotice.value = '已将“' + addedTitle + '”添加为“' + parentTitle + '”的子规划点'
      return
    }

    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(mapPickDayID.value) + '/stops', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision },
      body: JSON.stringify({ stop: { title: addedTitle, address: mapPickAddress.value.trim(), location: locationForMapPoint(point, provider) } })
    })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存规划点失败')
    applyTripPayload(payload)
    selectedDay.value = tripDocument.value.days.findIndex(day => day.id === mapPickDayID.value) + 1
    cancelMapPick()
    tripDetailsNotice.value = '已将“' + addedTitle + '”添加至行程规划'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存规划点失败'
  } finally {
    actionLoading.value = false
  }
}
function attachMapLabel(marker: any, title: string, dayBadge = '', isTarget = false, shouldShow = true) {
  if (!mapAPI?.Label || !mapAPI?.Size || typeof marker.setLabel !== 'function') return
  if (!shouldShow) {
    try { marker.setLabel(null) } catch { /* best effort */ }
    return
  }
  const text = dayBadge ? title + ' · ' + dayBadge : title

  // 智能自适应展开方向：若图钉位于视口右侧边缘附近，自动改为向左偏移
  let offset = new mapAPI.Size(16, -20)
  try {
    const pos = marker.getPosition?.()
    if (pos && mapInstance?.pointToPixel) {
      const pixel = mapInstance.pointToPixel(pos)
      const px = typeof pixel?.x === 'number' ? pixel.x : null
      const containerW = mapContainer.value?.clientWidth || window.innerWidth
      const rect = mapVisibleRect()
      const rightLimit = rect ? rect.right : containerW
      if (typeof px === 'number' && px > rightLimit - 180) {
        const estWidth = Math.max(70, text.length * 13 + 24)
        offset = new mapAPI.Size(-estWidth - 12, -20)
      }
    }
  } catch {}

  const label = new mapAPI.Label(text, { offset })
  const border = isTarget ? '2px solid #e56a4d' : '1px solid #6f797a'
  label.setStyle?.({
    color: '#172624',
    backgroundColor: '#fffffffa',
    border,
    borderRadius: '8px',
    padding: '4px 7px',
    fontSize: '12px',
    fontWeight: '700',
    lineHeight: '16px',
    whiteSpace: 'nowrap',
    boxShadow: isTarget ? '0 0 0 2px #ff9a7855, 0 4px 12px #0004' : '0 3px 10px #0003',
    zIndex: isTarget ? 999 : 10,
  })
  marker.setLabel(label)
  if (isTarget && typeof marker.setTop === 'function') {
    marker.setTop(true)
  }
}
function formatDistance(meters?: number) { if (!meters || meters < 0) return ''; return meters < 1000 ? Math.round(meters) + ' m' : (Math.round(meters / 100) / 10).toFixed(1).replace(/\.0$/, '') + ' km' }
function formatDuration(seconds?: number) { if (!seconds || seconds < 0) return ''; const minutes = Math.max(1, Math.round(seconds / 60)); return minutes < 60 ? minutes + ' 分钟' : Math.floor(minutes / 60) + ' 小时' + (minutes % 60 ? ' ' + minutes % 60 + ' 分钟' : '') }
function attachRouteLabel(snapshot: { geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; coordinate_system?: string }, legId = '') {
  if (!mapAPI?.Label || !mapAPI?.Size || !snapshot.geometry?.length) return
  const isLegSelected = Boolean(legId && selectedLegId.value === legId)
  if (!shouldShowRouteLabel(snapshot.distance_m, isLegSelected)) return
  const text = [formatDistance(snapshot.distance_m), formatDuration(snapshot.duration_s)].filter(Boolean).join(' · '); if (!text) return
  const middle = mapRoutePoint(snapshot.geometry[Math.floor(snapshot.geometry.length / 2)], snapshot.coordinate_system || 'bd09ll'); if (!middle) return
  const label = new mapAPI.Label(text, { offset: new mapAPI.Size(-24, -10) })
  label.setStyle?.({
    color: '#ffffff',
    backgroundColor: isLegSelected ? '#e56a4ded' : '#24695cdd',
    border: '0',
    borderRadius: '999px',
    padding: '4px 8px',
    fontSize: '11px',
    lineHeight: '15px',
    whiteSpace: 'nowrap',
    boxShadow: '0 3px 10px #0003',
    zIndex: isLegSelected ? 120 : 60,
  })
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
    if (!searchResults.value.length) {
      searchMessage.value = '没有找到结果，请补充城市或更换关键词'
      renderSearchResultMarkers()
    } else {
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur()
      }
      if (isMobileViewport() && sheetBreakpoint.value === 'expanded') {
        setSheetBreakpoint('half', 'replace')
      }
      selectSearchResult(0)
    }
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
    const payload = await response.json() as { error?: { message?: string; details?: { issues?: Array<{ path?: string; message?: string }> } } } & TripSummary
    if (!response.ok) throw new Error(importErrorMessage(payload, '导入失败'))
    await loadTrips()
    if (payload.id) {
      const summaryItem: TripSummary = {
        id: payload.id,
        title: payload.title,
        status: payload.status || 'draft',
        start_date: payload.start_date,
        end_date: payload.end_date,
        timezone: payload.timezone,
        revision: payload.revision || 1,
        days: payload.days,
        stops: payload.stops,
        show_in_atlas: payload.show_in_atlas ?? true,
        updated_at: payload.updated_at
      }
      navigateToTrip(summaryItem, 'push')
    }
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
const effectiveTheme = computed<'light' | 'dark'>(() => {
  if (theme.value === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return theme.value === 'dark' ? 'dark' : 'light'
})
function openTripPoster() {
  posterModalOpen.value = true
}
async function openPosterForTrip(trip: TripSummary) {
  tripMenuID.value = ''
  if (selected.value?.id !== trip.id) {
    await selectTrip(trip)
  }
  posterModalOpen.value = true
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
function weatherDetails(stop: Stop | SubStop) {
  const weather = stop.weather || {}
  if (!weather || Object.keys(weather).length === 0) return null
  const condition = String(weather.condition || weather.text_day || weather.text || '')
  const tempMin = weather.temp_min_c ?? weather.low
  const tempMax = weather.temp_max_c ?? weather.high
  const tempAvg = weather.temperature_c ?? weather.temp
  const currentCondition = weather.current_condition ? String(weather.current_condition) : ''
  const currentTemp = weather.current_temp_c

  let rangeText = ''
  if (tempMin !== undefined && tempMax !== undefined && tempMin !== null && tempMax !== null) {
    rangeText = Math.round(Number(tempMin)) + '°C ~ ' + Math.round(Number(tempMax)) + '°C'
  } else if (tempAvg !== undefined && tempAvg !== null) {
    rangeText = Math.round(Number(tempAvg)) + '°C'
  }

  let liveText = ''
  if (currentCondition || currentTemp !== undefined) {
    if (currentCondition && currentTemp !== undefined && currentTemp !== null) {
      liveText = currentCondition + ' ' + Math.round(Number(currentTemp)) + '°C'
    } else if (currentCondition) {
      liveText = currentCondition
    } else if (currentTemp !== undefined && currentTemp !== null) {
      liveText = Math.round(Number(currentTemp)) + '°C'
    }
  }

  // 更多气象指标：湿度、风向、风力、气压
  const metrics: string[] = []
  if (weather.humidity_percent !== undefined && weather.humidity_percent !== null) {
    metrics.push('湿度 ' + Math.round(Number(weather.humidity_percent)) + '%')
  }
  const windParts: string[] = []
  if (weather.wind_direction) windParts.push(String(weather.wind_direction))
  if (weather.wind_power) {
    const power = String(weather.wind_power)
    windParts.push(power.includes('级') ? power : power + '级')
  }
  if (windParts.length > 0) {
    metrics.push(windParts.join(' '))
  }
  if (weather.pressure_hpa !== undefined && weather.pressure_hpa !== null) {
    metrics.push(Math.round(Number(weather.pressure_hpa)) + ' hPa')
  }

  return {
    condition: condition || '天气预报',
    rangeText,
    liveText,
    metricsText: metrics.join(' · '),
    provider: weather.provider === 'amap' ? '高德天气' : weather.provider === 'baidu' ? '百度天气' : '',
  }
}
function weatherText(stop: Stop | SubStop) {
  const details = weatherDetails(stop)
  if (!details) return '暂无天气快照'
  const parts: string[] = []
  if (details.liveText) parts.push('当前 ' + details.liveText)
  if (details.condition) parts.push(String(details.condition))
  if (details.rangeText) parts.push(details.rangeText)
  return parts.join(' · ')
}
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
  if (selectedSubStopId.value) {
    navigateBackFromSubStop()
  } else if (selectedStopId.value) {
    navigateBackFromStop()
  } else if (selected.value) {
    selectedStopId.value = ''
    selectedSubStopId.value = ''
    syncNavigationURL('replace')
    void renderMap()
  } else {
    navigateBackToList()
  }
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
  const day = dayForStop(stop)
  if (!day) { error.value = '无法确定规划点所属日期'; return }
  const isChild = isChildStop(stop)
  const label = isChild ? '子规划点' : '规划点'
  if (!window.confirm('确认删除“' + stop.title + '”这个' + label + '吗？')) return
  const parent = isChild ? parentForStop(stop) : null
  const wasSelectedStop = selectedStopId.value === stop.id
  const wasSelectedSubStop = selectedSubStopId.value === stop.id
  actionLoading.value = true
  error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(stop.id), {
      method: 'DELETE',
      headers: { 'If-Match': 'revision-' + selected.value.revision }
    })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '删除规划点失败')
    applyTripPayload(payload)
    if (isChild) {
      if (wasSelectedSubStop && parent) {
        selectedStopId.value = parent.id
        selectedSubStopId.value = ''
        syncNavigationURL('replace')
      }
    } else {
      if (wasSelectedStop) {
        selectedStopId.value = ''
        selectedSubStopId.value = ''
        detailMoreOpen.value = false
        descriptionEditing.value = false
        descriptionFullscreen.value = false
        descriptionDraft.value = ''
        syncNavigationURL('replace')
      }
    }
    if (isMobileViewport() && (wasSelectedStop || wasSelectedSubStop)) {
      setSheetBreakpoint('half', 'replace')
    }
    void renderMap()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '删除规划点失败'
  } finally {
    actionLoading.value = false
  }
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
    photosRootDirInput.value = settingsData.value.photos?.root_dir || photoStatus.value?.root_dir || ''
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '无法读取设置' }
}

async function savePhotosRootDir() {
  settingsSaving.value = true
  try {
    const resp = await apiFetch('/api/v1/settings/photos', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root_dir: photosRootDirInput.value.trim() })
    })
    const payload = await resp.json() as { error?: { message?: string } }
    if (!resp.ok) throw new Error(payload.error?.message || '保存相册目录失败')
    await loadPhotoStatus()
    if (photoStatus.value?.enabled) {
      await loadAtlasPhotos()
      if (tripView.value === 'atlas') void renderAtlasMap(true)
    }
    settingsMessage.value = '相册扫描目录已保存，后台正在同步照片数据'
  } catch (cause) {
    settingsMessage.value = cause instanceof Error ? cause.message : '保存相册目录失败'
  } finally {
    settingsSaving.value = false
  }
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
        <main class="journey-redesign" :class="{ 'is-list-view': tripView === 'list', 'is-detail-view': tripView === 'detail', 'is-atlas-view': tripView === 'atlas', 'has-stop-selection': Boolean(selectedStop), 'is-shared-view': shareMode, 'is-history-view': Boolean(historyView) }">
          <input ref="fileInput" class="visually-hidden" type="file" accept="application/json,.json" aria-hidden="true" tabindex="-1" @change="importTrip" />
          <aside class="journey-rail" aria-label="JourneyIn 主导航">
            <button class="rail-brand" type="button" aria-label="返回行程列表" @click="navigateToList()"><BrandLogo :size="38" variant="mark" shape="squircle" class="rail-brand-mark-logo" /><span class="rail-brand-name">JourneyIn</span></button>
            <nav class="rail-nav" aria-label="工作区">
              <button class="rail-nav-item" :class="{ selected: tripView === 'list' }" type="button" @click="navigateToList()"><IonIcon :icon="menuOutline" /><span>行程</span></button>
              <button class="rail-nav-item" :class="{ selected: tripView === 'detail' }" type="button" :disabled="!selected" @click="selected ? navigateToTrip(selected, 'replace') : undefined"><IonIcon :icon="mapOutline" /><span>地图</span></button>
              <button class="rail-nav-item" :class="{ selected: tripView === 'atlas' }" type="button" @click="navigateToAtlas()"><IonIcon :icon="footstepsOutline" /><span>足迹</span></button>
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
                <button class="secondary-action" type="button" @click="navigateToAtlas()"><IonIcon :icon="footstepsOutline" /> 足迹漫游</button>
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
                  <span class="trip-card-copy"><strong>{{ trip.title }}</strong><span>{{ formatDateRange(trip.start_date, trip.end_date) }}</span><small>
                    {{ trip.days ?? '—' }} 天 · {{ trip.stops ?? '—' }} 个规划点 · revision {{ trip.revision }}
                    <span v-if="trip.show_in_atlas !== false" class="trip-atlas-tag" title="已汇聚在足迹漫游地图"><IonIcon :icon="footstepsOutline" /><span>足迹</span></span>
                  </small><small v-if="trip.updated_at" class="trip-card-updated">最后修改 {{ formatDateTime(trip.updated_at) }}</small></span>
                  <span class="trip-card-arrow">›</span>
                </button>
                <button class="trip-card-menu-button" type="button" :aria-expanded="tripMenuID === trip.id" :aria-label="'打开 ' + trip.title + ' 更多操作'" @click.stop="toggleTripMenu(trip.id)">⋯</button>
                <div v-if="tripMenuID === trip.id" class="trip-card-menu" role="menu"><button type="button" role="menuitem" @click="openPosterForTrip(trip)"><IonIcon :icon="imageOutline" /> 分享行程海报</button><button type="button" role="menuitem" @click="toggleTripAtlasFromList(trip)"><IonIcon :icon="footstepsOutline" /> {{ trip.show_in_atlas === false ? '加入足迹漫游' : '从足迹中隐藏' }}</button><button type="button" role="menuitem" @click="editTripDetailsFromList(trip)"><IonIcon :icon="createOutline" /> 编辑行程信息</button><button type="button" role="menuitem" @click="openTripHistoryFromList(trip)"><span aria-hidden="true">↶</span> 版本历史</button><button type="button" role="menuitem" class="danger-menu-item" @click="tripMenuID = ''; deleteTrip(trip)"><IonIcon :icon="closeOutline" /> 删除行程</button></div>
              </article>
            </div>
            </div>
          </section>

          <!-- 足迹漫游 (Atlas) 汇总全景视图 -->
          <section v-else-if="tripView === 'atlas'" class="journey-workspace atlas-workspace" aria-label="足迹漫游工作区">
            <div class="map-canvas redesign-map-canvas">
              <div v-if="keyConfigured && !mapError" :key="selectedMapProvider + '_atlas'" ref="mapContainer" id="map"></div>

              <!-- 足迹漫游微动效加载层 -->
              <MapLoadingState
                v-if="atlasLoading || (keyConfigured && !mapError && !mapReady && !mapWarning)"
                title="正在绘制足迹漫游…"
                subtitle="正在汇总历史行程路线与地标网络"
                :hint="atlasLoading ? '汇总行程数据中' : (mapProviderLabel + '底图连接中')"
                mode="atlas"
              />

              <div v-else-if="!keyConfigured || mapError" class="map-fallback">
                <IonIcon :icon="footstepsOutline" />
                <strong>{{ mapError || (mapProviderLabel + '未配置') }}</strong>
                <span>配置 {{ mapProviderLabel }} 浏览器端 Key 后即可呈现所有历史行程路线与足迹网络。</span>
                <button v-if="mapError" type="button" class="secondary-action compact-action" @click="retryMap">重新尝试加载</button>
                <button v-else-if="!readOnlyView" type="button" class="primary-action compact-action" @click="openSettings()">前往设置配置 Key</button>
              </div>

              <div v-if="mapWarning" class="map-warning"><span>{{ mapWarning }}</span><button type="button" @click="retryMap">重新加载</button></div>
            </div>

            <!-- 足迹漫游顶栏：左侧返回按钮与右侧地图选项/设置操作 -->
            <header class="workspace-topbar atlas-topbar">
              <button class="workspace-back atlas-back-btn" type="button" aria-label="返回行程列表" @click="navigateToList()">
                <span>‹</span><small>行程</small>
              </button>
              <div class="workspace-top-actions">
                <button class="workspace-tool-trigger" type="button" :aria-expanded="mobileMapToolsOpen" aria-label="打开地图选项" @click="toggleMobileMapTools">
                  <IonIcon :icon="mapOutline" /><span>地图选项</span>
                </button>
                <button v-if="!readOnlyView" class="workspace-more" type="button" aria-label="打开更多操作" @click="openSettings">
                  <span>⋯</span>
                </button>
              </div>
            </header>

            <div v-if="mobileMapToolsOpen" class="map-tools-backdrop" @click="mobileMapToolsOpen = false"></div>
            <div v-if="mobileMapToolsOpen" class="map-tools-card" role="dialog" aria-label="地图选项">
              <div class="map-tools-heading"><div><span class="eyebrow">MAP OPTIONS</span><strong>地图选项</strong></div><button type="button" aria-label="关闭地图选项" @click="toggleMobileMapTools"><IonIcon :icon="closeOutline" /></button></div>
              <div class="map-tool-row"><span>底图 Provider</span><div class="provider-segment"><button type="button" :class="{ active: selectedMapProvider === 'amap' }" @click="setMapProvider('amap')">高德</button><button type="button" :class="{ active: selectedMapProvider === 'baidu' }" @click="setMapProvider('baidu')">百度</button></div></div>
              <div class="map-tool-row"><span>图层</span><div class="provider-segment layer-segment" role="group" aria-label="地图图层"><button type="button" :class="{ active: mapType === 'normal' }" :aria-pressed="mapType === 'normal'" @click="setMapType('normal')">标准图</button><button type="button" :class="{ active: mapType === 'satellite' }" :aria-pressed="mapType === 'satellite'" @click="setMapType('satellite')">卫星图</button></div></div>
              <div class="map-tool-row"><span>地图标签</span><div class="provider-segment label-segment" role="group" aria-label="地图标签显示模式"><button type="button" :class="{ active: mapLabelMode === 'auto' }" @click="setMapLabelMode('auto')">自动</button><button type="button" :class="{ active: mapLabelMode === 'always' }" @click="setMapLabelMode('always')">全部</button><button type="button" :class="{ active: mapLabelMode === 'none' }" @click="setMapLabelMode('none')">隐藏</button></div></div>
              <p class="map-tool-hint">{{ mapLabelMode === 'auto' ? '自动：有空间时显示，密集时自动防重叠' : mapLabelMode === 'always' ? '全部：始终展开全部地名与路线标签' : '隐藏：仅保留图钉，隐藏浮动文字' }}</p>
              <div v-if="photoStatus?.enabled" class="map-tool-row">
                <span>足迹相片</span>
                <div class="provider-segment">
                  <button type="button" :class="{ active: atlasShowPhotos }" @click="toggleAtlasPhotos(true)">显示 ({{ atlasPhotos.length }})</button>
                  <button type="button" :class="{ active: !atlasShowPhotos }" @click="toggleAtlasPhotos(false)">隐藏</button>
                </div>
              </div>
            </div>

            <!-- 足迹漫游浮层：成就看板与行程高亮列表 (完全复用 workspace-panel / stop-detail-panel 的设计与结构) -->
            <aside
              class="floating-panel workspace-panel atlas-floating-panel"
              :class="['sheet-' + sheetBreakpoint, { 'is-sheet-dragging': sheetDragActive, 'has-photo-drawer-open': Boolean(selectedPhotoCluster) }]"
              :style="sheetDragStyle"
              aria-label="足迹漫游看板"
            >
              <!-- 移动端拖拽手柄 (复用与行程卡片完全一致的 sheet-handle) -->
              <button
                class="sheet-handle"
                type="button"
                :aria-label="sheetBreakpoint === 'peek' ? '展开足迹面板' : '收起足迹面板'"
                @pointerdown="startSheetDrag"
                @click="cycleSheetBreakpoint"
              >
                <span></span>
              </button>

              <header class="workspace-panel-head atlas-panel-head">
                <div class="atlas-brand-badge">
                  <div class="atlas-badge-icon-wrap">
                    <IonIcon :icon="footstepsOutline" />
                  </div>
                  <div>
                    <div class="workspace-panel-kicker">
                      <p class="eyebrow">JOURNEYIN ATLAS</p>
                      <span class="history-status-tag">全景</span>
                    </div>
                    <h1>足迹漫游</h1>
                  </div>
                </div>
                <div class="panel-head-actions">
                  <button
                    type="button"
                    class="panel-action-button collapse-action"
                    :aria-label="sheetBreakpoint === 'peek' ? '展开面板' : '收起面板'"
                    @click="setSheetBreakpoint(sheetBreakpoint === 'peek' ? 'half' : 'peek')"
                  >
                    <IonIcon :icon="sheetBreakpoint === 'peek' ? chevronUpOutline : chevronDownOutline" />
                  </button>
                  <button type="button" class="panel-action-button" aria-label="返回行程列表" @click="navigateToList()">
                    <IonIcon :icon="closeOutline" />
                  </button>
                </div>
              </header>

              <!-- 统一的可滚动区域：信息小卡片、图层过滤与行程列表整体参与滚动，优化小屏视野 -->
              <div class="atlas-trip-scroll">
                <!-- 成就数据卡片 -->
                <div class="atlas-metrics-grid">
                  <div class="atlas-metric-card">
                    <span>点亮行程</span>
                    <strong>{{ atlasData?.total_trips || 0 }} <em>次</em></strong>
                  </div>
                  <div class="atlas-metric-card">
                    <span>累计天数</span>
                    <strong>{{ atlasData?.total_days || 0 }} <em>天</em></strong>
                  </div>
                  <div class="atlas-metric-card">
                    <span>规划地点</span>
                    <strong>{{ atlasData?.total_stops || 0 }} <em>处</em></strong>
                  </div>
                  <div class="atlas-metric-card highlight">
                    <span>足迹总里程</span>
                    <strong>{{ formatDistance(atlasData?.total_distance_m) || '0 km' }}</strong>
                  </div>
                  <div v-if="photoStatus?.enabled" class="atlas-metric-card highlight-photo">
                    <span>足迹照片</span>
                    <strong>{{ photoStatus.gps_photos || 0 }} <em>张</em></strong>
                  </div>
                </div>

                <!-- 图层切换与照片库同步：M3 胶囊 Chip 风格，完全融入整体质感 -->
                <div class="atlas-filter-toolbar">
                  <div class="atlas-filter-chips">
                    <button
                      type="button"
                      class="atlas-pill-btn"
                      :class="{ active: atlasShowTrips }"
                      :aria-pressed="atlasShowTrips"
                      @click="atlasShowTrips = !atlasShowTrips; renderAtlasMap(true)"
                    >
                      <IonIcon :icon="footstepsOutline" />
                      <span>路线轨迹</span>
                    </button>
                    <button
                      v-if="photoStatus?.enabled"
                      type="button"
                      class="atlas-pill-btn"
                      :class="{ active: atlasShowPhotos }"
                      :aria-pressed="atlasShowPhotos"
                      @click="toggleAtlasPhotos()"
                    >
                      <IonIcon :icon="imageOutline" />
                      <span>足迹照片</span>
                      <span v-if="atlasPhotos.length" class="atlas-pill-badge">{{ atlasPhotos.length }}</span>
                    </button>
                  </div>
                  <button
                    v-if="photoStatus?.enabled"
                    type="button"
                    class="atlas-refresh-action"
                    :disabled="photoSyncing || photoStatus.scanning"
                    @click="triggerPhotoSync"
                    title="重新扫描照片目录"
                  >
                    <IonIcon :icon="refreshOutline" :class="{ 'atlas-sync-spin': photoSyncing || photoStatus.scanning }" />
                    <span>{{ photoSyncing || photoStatus.scanning ? '扫描中…' : '刷新' }}</span>
                  </button>
                </div>

                <!-- 行程列表与筛选 -->
                <div class="atlas-list-title">
                  <span>全部足迹路线</span>
                  <small v-if="selectedAtlasTripID">已聚焦 1 条路线 · <a href="javascript:void(0)" @click="clearSelectedAtlasTrip">重置全景</a></small>
                </div>

                <div v-if="atlasLoading" class="atlas-loading">
                  <span class="loading-dot"></span>
                  <span>正在汇总各行程轨迹…</span>
                </div>
                <div v-else-if="!atlasData?.trips?.length" class="atlas-empty">
                  <span>暂无纳入足迹漫游的行程</span>
                  <small>在行程卡片菜单或行程编辑中勾选“在足迹漫游中汇总显示”即可汇聚到此</small>
                </div>
                <div v-else class="atlas-trip-cards">
                  <article
                    v-for="(item, idx) in atlasData.trips"
                    :key="item.id"
                    class="atlas-trip-card"
                    :class="{ active: selectedAtlasTripID === item.id }"
                    :style="{ '--trip-accent': ATLAS_PALETTE[idx % ATLAS_PALETTE.length] }"
                    @click="selectAtlasTrip(item)"
                  >
                    <span class="atlas-color-indicator"></span>
                    <div class="atlas-trip-info">
                      <strong>{{ item.title }}</strong>
                      <span>{{ formatDateRange(item.start_date, item.end_date) }} · {{ item.days_count }}天 · {{ item.stops_count }}点</span>
                      <small v-if="item.distance_m > 0">路线里程 ~{{ formatDistance(item.distance_m) }}</small>
                    </div>
                    <div class="atlas-card-actions">
                      <button type="button" class="atlas-goto-btn" title="进入该行程规划地图" @click.stop="gotoTripFromAtlas(item.id)">
                        <span>直达</span>
                      </button>
                    </div>
                  </article>
                </div>
              </div>
            </aside>

            <!-- 照片聚类详情抽屉 (点击聚簇 Marker 呼出，小屏幕作为高层级底部半屏抽屉滑出，桌面端作为居中悬浮卡片) -->
            <div v-if="selectedPhotoCluster" class="atlas-photo-drawer">
              <div class="atlas-photo-drawer-head">
                <strong>该区域照片 ({{ selectedPhotoCluster.photos.length }} 张)</strong>
                <button type="button" aria-label="关闭照片列表" @click="selectedPhotoCluster = null">×</button>
              </div>
              <div class="atlas-photo-grid">
                <div
                  v-for="p in selectedPhotoCluster.photos"
                  :key="p.id"
                  class="atlas-photo-item-card"
                  @click="openPhotoPreview(p, selectedPhotoCluster.photos)"
                >
                  <img :src="p.thumb_url + '?size=120'" class="atlas-photo-item-img" loading="lazy" />
                  <span class="atlas-photo-item-date">{{ formatPhotoTime(p.taken_at) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section v-else class="journey-workspace" aria-label="地图工作区">
            <div class="map-canvas redesign-map-canvas" :class="{ 'map-pick-active': mapPickMode }">
              <div v-if="keyConfigured && tripDocument && !mapError" :key="selectedMapProvider" ref="mapContainer" id="map"></div>

              <!-- 全新旅行微动效加载层 -->
              <MapLoadingState
                v-if="!tripDocument || (keyConfigured && !mapError && !mapReady && !mapWarning)"
                :title="!tripDocument ? '正在绘制旅途地图…' : '正在连接' + mapProviderLabel + '…'"
                :subtitle="!tripDocument ? '正在解析地点坐标与路线拓扑' : '正在绘制图钉与路线网络'"
                :hint="!tripDocument ? '读取行程规划中' : (mapProviderLabel + '底图连接中')"
              />

              <!-- 真异常或未配置 Key 降级面板 -->
              <div v-else-if="!keyConfigured || mapError" class="map-fallback">
                <IonIcon :icon="mapOutline" />
                <strong>{{ mapError || (mapProviderLabel + '未配置') }}</strong>
                <span>{{ mapError ? '请确认浏览器端 Key、域名白名单和网络连接。当前页面：' + serverURL : '配置' + mapProviderLabel + '浏览器端 Key 后显示真实地图；已保存的行程数据仍然可查看。' }}</span>
                <button v-if="mapError" type="button" class="secondary-action compact-action" @click="retryMap">重新尝试加载</button>
                <button v-else-if="!readOnlyView" type="button" class="primary-action compact-action" @click="openSettings()">前往设置配置 Key</button>
              </div>

              <div v-if="mapWarning" class="map-warning"><span>{{ mapWarning }}</span><button type="button" @click="retryMap">重新加载</button></div>
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
                <div v-if="!readOnlyView" class="panel-data-actions"><button type="button" @click="openImportPicker">导入</button><button type="button" :disabled="actionLoading" @click="downloadTrip">导出 JSON</button><button type="button" :disabled="actionLoading" @click="createShare">在线分享</button><button type="button" @click="openTripPoster">生成海报</button></div><div v-else class="panel-data-actions"><button type="button" @click="openTripPoster">保存为图片</button></div>
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
                <div class="detail-weather">
                  <IonIcon :icon="sunnyOutline" />
                  <span class="weather-text-wrap">
                    <strong class="weather-title">
                      <span v-if="weatherDetails(selectedTarget || selectedStop)?.liveText" class="weather-live-chip">当前 {{ weatherDetails(selectedTarget || selectedStop)?.liveText }}</span>
                      <span class="weather-forecast-str">{{ weatherDetails(selectedTarget || selectedStop) ? (weatherDetails(selectedTarget || selectedStop)?.condition + (weatherDetails(selectedTarget || selectedStop)?.rangeText ? ' · ' + weatherDetails(selectedTarget || selectedStop)?.rangeText : '')) : '暂无天气快照' }}</span>
                    </strong>
                    <span v-if="weatherDetails(selectedTarget || selectedStop)?.metricsText" class="weather-metrics-line">
                      {{ weatherDetails(selectedTarget || selectedStop)?.metricsText }}
                    </span>
                    <small v-if="weatherUpdatedAt(selectedTarget || selectedStop)">
                      <span v-if="weatherDetails(selectedTarget || selectedStop)?.provider">{{ weatherDetails(selectedTarget || selectedStop)?.provider }} · </span>更新于 {{ weatherUpdatedAt(selectedTarget || selectedStop) }}
                    </small>
                  </span>
                  <button v-if="!readOnlyView" type="button" :disabled="weatherLoading || !pointFor(selectedTarget || selectedStop)" @click="refreshWeather">{{ weatherLoading ? '查询中…' : '刷新' }}</button>
                </div>
                <section v-if="!selectedSubStop" class="detail-section"><div class="section-title-row"><h2>子规划点 <span>{{ selectedStop.children?.length || 0 }}</span></h2><button v-if="!readOnlyView" class="text-action" type="button" @click="openChildSearch(selectedStop)">添加</button></div><p v-if="selectedStop.children?.length" class="detail-section-help">点击子点进入下一层，返回箭头会回到主规划点。</p><div v-if="selectedStop.children?.length" class="detail-child-list"><button v-for="child in selectedStop.children" :key="child.id" type="button" class="detail-child-row" @click="selectSubStop(child, selectedStop)"><span class="child-number">{{ child.sequence }}</span><span><strong>{{ child.title }}</strong><small>{{ stopDate(child) }} · {{ child.address || '地址待补充' }}</small><em class="stop-location-badge" :class="{ missing: !pointFor(child) }">{{ locationStatus(child) }}</em></span><span>›</span></button></div><button v-if="!readOnlyView" class="add-place-action" type="button" @click="openChildSearch(selectedStop)"><IonIcon :icon="searchOutline" /> 添加子规划点</button></section>
                <button v-else class="detail-parent-button" type="button" @click="navigateBackFromSubStop">‹ 返回主规划点：{{ selectedStop.title }}</button>
                <section class="detail-section"><div class="section-title-row"><h2>地点说明与时间</h2><div v-if="!readOnlyView" class="section-actions"><button class="text-action" type="button" @click="beginEditDescription">{{ descriptionEditing ? '编辑中' : '编辑规划点' }}</button><button v-if="descriptionEditing" class="text-action" type="button" @click="openDescriptionFullscreen">全屏</button></div></div><template v-if="descriptionEditing && !readOnlyView"><div class="detail-time-editor"><div class="detail-time-editor-heading"><strong>时间窗口</strong><small>到达和离开时间均为可选，留空表示未设置</small></div><div class="detail-time-fields"><label>到达<input v-model="arrivalTimeDraft" type="time" /></label><label>离开<input v-model="departureTimeDraft" type="time" /></label></div></div><MarkdownEditor v-model="descriptionDraft" v-model:mode="descriptionEditorMode" :preview-html="renderMarkdown(descriptionDraft)" :rows="7" editor-label="MARKDOWN" preview-label="地点说明预览" editor-aria-label="地点说明 Markdown 原始文本" placeholder="补充门票、开放时间、行程备注等信息" /><div class="editor-actions"><button class="secondary-action compact-action" type="button" @click="cancelEditDescription">取消</button><button class="primary-action compact-action" type="button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存规划点' }}</button></div></template><div v-else-if="selectedTarget?.description_markdown" class="markdown" v-html="renderMarkdown(selectedTarget.description_markdown)"></div><p v-else class="muted">{{ shareMode ? '暂无地点说明。' : '暂无地点说明，点击“编辑说明与时间”添加。' }}</p></section>
                <button v-if="!readOnlyView" class="detail-danger-button" type="button" @click="deletePlanningPoint(selectedTarget || selectedStop)">删除{{ selectedSubStop ? '子规划点' : '规划点' }}</button>
              </div>
            </aside>

            <header class="workspace-topbar">
              <button v-if="!readOnlyView" class="workspace-back" type="button" aria-label="返回行程列表" @click="navigateBackToList"><span>‹</span><small>行程</small></button>
              <div class="workspace-title"><strong>{{ historyTitle }}</strong><span>{{ historyDateRange || '地图工作区' }}</span></div>
              <div class="workspace-top-actions">
                <button class="workspace-tool-trigger" type="button" :aria-expanded="mobileMapToolsOpen" aria-label="打开地图选项" @click="toggleMobileMapTools"><IonIcon :icon="mapOutline" /><span>地图选项</span></button>
                <button v-if="!readOnlyView" class="workspace-more" type="button" aria-label="打开更多操作" @click="openSettings"><span>⋯</span></button>
              </div>
            </header>

            <div v-if="tripDocument" class="workspace-status"><span class="status-dot" :class="{ ready: keyConfigured && mapReady && !mapError }"></span><span>{{ !keyConfigured ? '离线数据可用' : mapError ? mapProviderLabel + '不可用' : mapReady ? mapProviderLabel + '已连接' : mapProviderLabel + '加载中' }} · {{ visibleStops.length }} 个规划点<span v-if="unlocatedMainStops.length"> · 待定位 {{ unlocatedMainStops.length }}</span></span></div>

            <div v-if="mobileMapToolsOpen" class="map-tools-backdrop" @click="mobileMapToolsOpen = false"></div>
            <div v-if="mobileMapToolsOpen" class="map-tools-card" role="dialog" aria-label="地图选项">
              <div class="map-tools-heading"><div><span class="eyebrow">MAP OPTIONS</span><strong>地图选项</strong></div><button type="button" aria-label="关闭地图选项" @click="toggleMobileMapTools"><IonIcon :icon="closeOutline" /></button></div>
              <div class="map-tool-row"><span>底图 Provider</span><div class="provider-segment"><button type="button" :class="{ active: selectedMapProvider === 'amap' }" @click="setMapProvider('amap')">高德</button><button type="button" :class="{ active: selectedMapProvider === 'baidu' }" @click="setMapProvider('baidu')">百度</button></div></div>
              <div class="map-tool-row"><span>图层</span><div class="provider-segment layer-segment" role="group" aria-label="地图图层"><button type="button" :class="{ active: mapType === 'normal' }" :aria-pressed="mapType === 'normal'" @click="setMapType('normal')">标准图</button><button type="button" :class="{ active: mapType === 'satellite' }" :aria-pressed="mapType === 'satellite'" @click="setMapType('satellite')">卫星图</button></div></div>
              <div class="map-tool-row"><span>地图标签</span><div class="provider-segment label-segment" role="group" aria-label="地图标签显示模式"><button type="button" :class="{ active: mapLabelMode === 'auto' }" @click="setMapLabelMode('auto')">自动</button><button type="button" :class="{ active: mapLabelMode === 'always' }" @click="setMapLabelMode('always')">全部</button><button type="button" :class="{ active: mapLabelMode === 'none' }" @click="setMapLabelMode('none')">隐藏</button></div></div>
              <p class="map-tool-hint">{{ mapLabelMode === 'auto' ? '自动：有空间时显示，密集时自动防重叠' : mapLabelMode === 'always' ? '全部：始终展开全部地名与路线标签' : '隐藏：仅保留图钉，隐藏浮动文字' }}</p>
              <button v-if="!readOnlyView" class="map-pick-action" type="button" :disabled="!mapReady || !tripDocument" @click="toggleMapPick"><IonIcon :icon="mapOutline" /> {{ mapPickMode ? mapPickTargetID ? '取消更新选点' : '取消地图选点' : '地图选点' }}</button>
            </div>

            <section v-if="error || tripDetailsNotice || historyView || (shareNoticeVisible && shareURL)" class="map-notices redesign-notices">
              <div v-if="historyView" class="history-readonly-banner"><span><strong>历史版本 · 只读</strong><small>{{ historyView.label || '保存于 ' + formatDateTime(historyView.created_at) }} · 工作版本 {{ historyView.source_revision }}</small></span><button type="button" @click="exitTripHistory">返回当前版本</button></div>
              <div v-if="error" class="global-error"><IonIcon :icon="cloudOfflineOutline" /><span>{{ error }}</span><button type="button" class="notice-close-btn" aria-label="关闭错误提示" @click="closeError">×</button></div>
              <div v-if="tripDetailsNotice" class="global-notice"><IonIcon :icon="createOutline" /><span>{{ tripDetailsNotice }}</span><button type="button" class="notice-close-btn" aria-label="关闭提示" @click="closeNotice">×</button></div>
              <div v-if="shareNoticeVisible && shareURL" class="share-banner"><span><strong>只读分享已创建</strong><a :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a><small v-if="shareExpiresAt">有效期至 {{ formatDateTime(shareExpiresAt) }}</small><small v-if="shareCopyMessage" class="share-copy-feedback">{{ shareCopyMessage }}</small></span><div class="share-actions"><button type="button" @click="copyShareURL">复制链接</button><button type="button" @click="openTripPoster">生成海报</button><button v-if="shareID" type="button" @click="revokeShare">撤销</button><button type="button" class="notice-close-btn" aria-label="关闭分享提示" @click="dismissShareNotice">×</button></div></div>
            </section>
          </section>
        </main>
      </div>

      <div v-if="pointEditorOpen && !readOnlyView" class="modal-backdrop point-editor-backdrop" @click.self="cancelEditPoint"><section class="modal-panel point-editor-panel" role="dialog" aria-modal="true" aria-labelledby="point-editor-title" aria-describedby="point-editor-description"><header class="point-editor-header"><div><p class="eyebrow">POINT EDITOR</p><h2 id="point-editor-title">编辑规划点</h2><p id="point-editor-description">名称、地址和坐标分开处理；不会因为改名而替换位置。</p></div><button class="modal-close" type="button" :disabled="pointEditorSaving" aria-label="关闭规划点编辑" @click="cancelEditPoint">×</button></header><form id="point-editor-form" class="point-editor-form" @submit.prevent="savePointDetails"><label>规划点名称<input ref="pointEditorTitleInput" v-model="pointEditorTitleDraft" maxlength="200" required placeholder="例如：西湖断桥" /></label><label>地址或补充定位线索<input v-model="pointEditorAddressDraft" maxlength="500" placeholder="用于确认候选，不会自动猜坐标" /></label><section class="point-editor-location" :class="{ missing: !pointEditorPoint() }"><div class="point-editor-location-head"><div><strong>{{ pointEditorLocationStatus() }}</strong><small v-if="pointEditorPoint()">{{ pointEditorPoint()?.crs }} · {{ pointEditorPoint()?.lat.toFixed(6) }}, {{ pointEditorPoint()?.lng.toFixed(6) }}</small><small v-else>没有可靠坐标，路线与导航暂不可用。</small></div><span class="location-state-label">{{ pointEditorPoint() ? '已保存' : '待处理' }}</span></div><small v-if="pointEditorPoint()">来源：{{ pointEditorLocationSource() }}</small><p v-else>请从候选中选择一个明确地点，或在地图上点击准确位置。不会根据数字外观补造 CRS。</p><div class="point-editor-location-actions"><button class="secondary-action compact-action" type="button" @click="openPointSearchFromEditor">重新搜索候选</button><button class="secondary-action compact-action" type="button" :disabled="!mapReady" @click="startMapPickFromEditor">地图选点更新</button></div></section><p class="point-editor-note">重新搜索或地图选点会清除受影响的路线和该点天气；保存名称和地址本身不会改变路线。</p><p v-if="error" class="point-editor-error" role="alert">{{ error }}</p><div class="point-editor-related"><span><strong>更多编辑</strong><small>说明、时间窗口、日期、顺序和删除仍在详情页中管理。</small></span><button class="text-action" type="button" @click="beginEditDescriptionFromPointEditor">编辑说明与时间</button></div></form><div class="modal-actions point-editor-actions"><button type="button" :disabled="pointEditorSaving" @click="cancelEditPoint">取消</button><button class="primary" type="submit" form="point-editor-form" :disabled="pointEditorSaving || !pointEditorTitleDraft.trim()">{{ pointEditorSaving ? '保存中…' : '保存名称和地址' }}</button></div></section></div>
      <div v-if="descriptionFullscreen && descriptionEditing && !readOnlyView" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-description-title"><header><h2 id="fullscreen-description-title">编辑规划点信息</h2><button class="modal-close" type="button" aria-label="退出全屏编辑" @click="closeDescriptionFullscreen">×</button></header><MarkdownEditor class="fullscreen-markdown-editor" v-model="descriptionDraft" v-model:mode="descriptionEditorMode" :preview-html="renderMarkdown(descriptionDraft)" :rows="12" editor-label="MARKDOWN" preview-label="地点说明预览" editor-aria-label="地点说明 Markdown 原始文本" placeholder="补充门票、开放时间、行程备注等信息" /><div class="description-actions"><button class="text-button" type="button" @click="cancelEditDescription">取消</button><button class="primary-text-button" type="button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存规划点' }}</button></div></section></div>
      <div v-if="tripDescriptionFullscreen && tripDescriptionEditing && !readOnlyView" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-trip-description-title"><header><h2 id="fullscreen-trip-description-title">编辑行程总体说明</h2><button class="modal-close" type="button" aria-label="退出全屏编辑" @click="closeTripDescriptionFullscreen">×</button></header><MarkdownEditor class="fullscreen-markdown-editor" v-model="tripDescriptionDraft" v-model:mode="tripDescriptionEditorMode" :preview-html="renderMarkdown(tripDescriptionDraft)" :rows="12" editor-label="MARKDOWN" preview-label="行程说明预览" editor-aria-label="行程说明 Markdown 原始文本" placeholder="补充整个行程的背景、节奏和注意事项" /><div class="description-actions"><button class="text-button" type="button" @click="cancelEditTripDescription">取消</button><button class="primary-text-button" type="button" :disabled="tripDescriptionSaving" @click="saveTripDescription">{{ tripDescriptionSaving ? '保存中…' : '保存说明' }}</button></div></section></div>
      <div v-if="mapPickOpen" class="modal-backdrop" @click.self="cancelMapPick">
        <section class="modal-panel map-pick-panel" role="dialog" aria-modal="true" aria-labelledby="map-pick-title">
          <button class="modal-close" aria-label="取消添加" @click="cancelMapPick">×</button>
          <p class="eyebrow">MAP POINT</p>
          <h2 id="map-pick-title">
            {{
              mapPickTargetID ? '更新规划点位置' :
              mapPickKind === 'substop' ? '添加子规划点' :
              (mapPickTitle && mapPickTitle !== '地图地点') ? '添加地点到行程' : '保存地图选点'
            }}
          </h2>
          <p class="map-pick-coordinate">{{ mapPickLocation?.crs }} · {{ mapPickLocation?.lat.toFixed(6) }}, {{ mapPickLocation?.lng.toFixed(6) }}</p>
          <p class="map-pick-context">
            {{
              mapPickTargetID ? '点击保存后会替换当前坐标，并清除受影响的路线和天气。' :
              (mapPickKind === 'substop' && mapPickParentStopTitle) ?
                ('确认将地图上的「' + (mapPickTitle || '此地点') + '」添加为「' + mapPickParentStopTitle + '」的子规划点吗？') :
              (mapPickTitle && mapPickTitle !== '地图地点') ?
                ('确认将地图上的「' + mapPickTitle + '」添加至当前规划吗？') :
              '确认将该地图地点添加至当前规划吗？'
            }}
          </p>
          <label>地点名称<input v-model="mapPickTitle" required autofocus placeholder="请输入地点名称" /></label>
          <label>地址或备注（可选）<input v-model="mapPickAddress" placeholder="补充位置说明" /></label>

          <!-- 仅在新增地点（非更新坐标）时显示级别分段器与关联设置 -->
          <template v-if="!mapPickTargetID">
            <div class="map-pick-segment-row">
              <span>规划点类型</span>
              <div class="provider-segment">
                <button
                  type="button"
                  :class="{ active: mapPickKind === 'stop' }"
                  @click="mapPickKind = 'stop'"
                >
                  主规划点
                </button>
                <button
                  type="button"
                  :class="{ active: mapPickKind === 'substop' }"
                  :disabled="!mapPickAvailableParentStops.length"
                  @click="mapPickKind = 'substop'"
                >
                  子规划点 {{ mapPickAvailableParentStops.length ? '' : '(当天无主点)' }}
                </button>
              </div>
            </div>

            <label class="select-field">
              加入日期
              <UiSelect v-model="mapPickDayID" aria-label="加入日期" :options="tripDayOptions" />
            </label>

            <label v-if="mapPickKind === 'substop'" class="select-field">
              所属主规划点
              <UiSelect
                v-model="mapPickParentStopId"
                aria-label="所属主规划点"
                :options="mapPickParentStopOptions"
              />
            </label>
          </template>

          <p v-if="error" class="modal-form-error" role="alert">{{ error }}</p>
          <div class="modal-actions">
            <button type="button" @click="cancelMapPick">取消</button>
            <button
              type="button"
              class="primary"
              :disabled="actionLoading || !mapPickTitle.trim() || (mapPickKind === 'substop' && !mapPickParentStopId)"
              @click="saveMapPick"
            >
              {{ actionLoading ? '保存中…' : mapPickTargetID ? '更新规划点' : mapPickKind === 'substop' ? '确认添加为子点' : '确认添加' }}
            </button>
          </div>
        </section>
      </div>
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
            <div class="trip-details-atlas-toggle">
              <label class="atlas-checkbox-label">
                <input type="checkbox" v-model="tripDetailsShowInAtlasDraft" />
                <span class="atlas-checkbox-text">
                  <strong>在足迹漫游中汇总显示此行程</strong>
                  <small>开启后，本行程的轨迹路线与代表性地标将汇聚在全景“足迹漫游”总图中</small>
                </span>
              </label>
            </div>
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
              <button type="button" :class="{ active: settingsSection === 'photos' }" @click="settingsSection = 'photos'"><span class="settings-nav-icon"><IonIcon :icon="imageOutline" /></span><span>足迹相册</span><small>相册扫描目录</small></button>
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
                <div class="settings-provider-grid"><article class="settings-card provider-card"><div class="settings-card-heading"><div><strong>高德地图</strong><small>JS API 2.0 / Web Service</small></div><span class="provider-status">{{ settingsData?.map?.amap?.js_key_configured ? 'JS 已配置' : '待配置' }}</span></div><p class="settings-status-line">JS Key：<strong>{{ settingsData?.map?.amap?.js_key_configured ? '已配置' : '未配置' }}</strong><br />服务端 Key：<strong>{{ settingsData?.map?.amap?.server_key_configured ? '已配置' : '未配置' }}</strong><br />安全密钥：<strong>{{ settingsData?.map?.amap?.security_js_code_configured ? '已配置' : '未配置' }}</strong></p><label>JS Key<input v-model="amapJSKeyInput" type="password" placeholder="用于浏览器端地图" autocomplete="off" /></label><label>服务端 Key<input v-model="amapServerKeyInput" type="password" placeholder="留空保持当前值" autocomplete="off" /></label><label>JS 安全密钥<input v-model="amapSecurityJSCodeInput" type="password" placeholder="用于安全代理" autocomplete="off" /></label><a href="https://console.amap.com/dev/key/app" target="_blank" rel="noopener noreferrer">申请/管理高德 Key ↗</a></article><article class="settings-card provider-card"><div class="settings-card-heading"><div><strong>百度地图</strong><small>JSAPI 4.0 / Web Service</small></div><span class="provider-status">{{ baiduKey ? '浏览器已配置' : '待配置' }}</span></div><p class="settings-status-line">浏览器端 Key：<strong>{{ baiduKey ? '已配置' : '未配置' }}</strong><br />服务端 Key：<strong>{{ settingsData?.map?.baidu?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>浏览器端 Key<input v-model="baiduBrowserKeyInput" type="password" :placeholder="settingsData?.map?.baidu?.browser_key_configured ? '已配置，输入新 Key 可替换' : '用于浏览器端地图'" autocomplete="off" /></label><label>服务端 Key<input v-model="baiduServerKeyInput" type="password" placeholder="留空保持当前值" autocomplete="off" /></label><a href="https://lbsyun.baidu.com/apiconsole/key" target="_blank" rel="noopener noreferrer">申请/管理百度 Key ↗</a></article></div><p class="settings-help">保存地图 Key 到数据库后，浏览器端 Key 会立即生效；服务端 Key 和安全密钥只返回配置状态，不会回显原文。</p><div class="settings-actions"><button class="primary-action" type="button" :disabled="settingsSaving" @click="saveMapKeys">{{ settingsSaving ? '保存中…' : '保存地图 Key 到数据库' }}</button></div><p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>


              <section v-else-if="settingsSection === 'photos'" class="settings-page-section">
                <div class="settings-section-heading">
                  <span class="eyebrow">ATLAS PHOTOS</span>
                  <h3>足迹相册扫描目录</h3>
                  <p>配置本地或容器内的照片目录，自动提取拍摄时间与 GPS 地理信息并在足迹漫游中打点呈现。</p>
                </div>
                <div class="settings-card">
                  <div class="settings-card-heading">
                    <div>
                      <strong>相册扫描根目录 (Photos Directory)</strong>
                      <small>递归扫描所有子文件夹，支持 JPG、PNG、HEIC、WEBP、TIFF 等格式</small>
                    </div>
                    <span class="provider-status">{{ photoStatus?.enabled ? '已启用 (' + (photoStatus?.gps_photos || 0) + ' 处)' : '未启用' }}</span>
                  </div>
                  <label>
                    照片目录路径
                    <input
                      v-model="photosRootDirInput"
                      type="text"
                      placeholder="例如：D:/Photos 或 /photos (留空保存可清除)"
                      autocomplete="off"
                    />
                  </label>
                  <p class="settings-help">
                    若留空保存将清除配置；若宿主机配置了环境变量 <code>JOURNEYIN_PHOTOS_DIR</code>，将在未保存设置时自动生效。保存新目录后，系统将自动开始增量扫描并清理已失效点位。
                  </p>
                  <div v-if="photoStatus?.enabled" class="settings-cache-row">
                    <span>已索引照片</span>
                    <strong>{{ photoStatus?.total_photos || 0 }} 张 (含 GPS：{{ photoStatus?.gps_photos || 0 }} 处)</strong>
                  </div>
                  <div v-if="photoStatus?.last_scan_at" class="settings-cache-row">
                    <span>最近扫描时间</span>
                    <small>{{ formatPhotoTime(photoStatus.last_scan_at) }}</small>
                  </div>
                  <div class="settings-actions">
                    <button class="primary-action" type="button" :disabled="settingsSaving" @click="savePhotosRootDir">
                      {{ settingsSaving ? '保存中…' : '保存相册目录' }}
                    </button>
                    <button
                      v-if="photoStatus?.enabled"
                      class="secondary-action"
                      type="button"
                      :disabled="photoSyncing || photoStatus?.scanning"
                      @click="triggerPhotoSync"
                    >
                      {{ photoSyncing || photoStatus?.scanning ? '扫描中…' : '立即增量重新扫描' }}
                    </button>
                  </div>
                </div>
                <p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>

              <section v-else-if="settingsSection === 'search'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">PLACE SEARCH</span><h3>地点检索</h3><p>调整搜索 Provider 优先级，管理本地地点目录缓存。</p></div>
                <div class="settings-card"><div class="settings-card-heading"><div><strong>搜索优先级</strong><small>未命中本地目录后使用所选 Provider</small></div><span class="provider-status">{{ poiProviderPriority === 'amap' ? '高德优先' : '百度优先' }}</span></div><label class="select-field">优先 Provider<UiSelect v-model="poiProviderPriority" aria-label="地点检索优先 Provider" :options="poiPriorityOptions" /></label><p class="settings-help">Provider 不可用时会自动尝试另一家；新搜索结果只保留 7 天。</p><div class="settings-cache-row"><span>本地地点记录</span><strong>{{ localDirectoryCount }} 条</strong></div><div class="settings-actions"><button class="primary-action" type="button" @click="savePOIPreferences">保存检索优先级</button><button class="secondary-action" type="button" @click="clearLocalDirectory">清除本地记录</button></div></div><p v-if="settingsMessage" class="settings-feedback">{{ settingsMessage }}</p>
              </section>

              <section v-else-if="settingsSection === 'sharing'" class="settings-page-section">
                <div class="settings-section-heading"><span class="eyebrow">ONLINE SHARING</span><h3>在线分享</h3><p>集中管理当前行程的只读分享链接；分享状态不会遮挡地图。</p></div>
                <div class="settings-card settings-share-card"><div class="settings-card-heading"><div><strong>{{ selected?.title || '当前行程' }}</strong><small>持有链接即可查看当前行程快照</small></div><span class="provider-status">{{ shareURL ? '已分享' : '未分享' }}</span></div><template v-if="selected"><a v-if="shareURL" class="settings-share-url" :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a><p v-if="shareExpiresAt" class="settings-share-expiry">有效期至 {{ formatDateTime(shareExpiresAt) }}</p><p v-if="!shareURL" class="settings-help">当前行程还没有在线分享；点击下方按钮创建一个只读链接。</p><div class="settings-actions"><button v-if="shareURL" class="primary-action" type="button" @click="copyShareURL">复制链接</button><button class="secondary-action" type="button" @click="openTripPoster">分享海报</button><button v-if="shareURL && shareID" class="secondary-action" type="button" :disabled="actionLoading" @click="revokeShare">撤销分享</button><button v-else class="primary-action" type="button" :disabled="actionLoading" @click="createShare">{{ actionLoading ? '创建中…' : '在线分享' }}</button></div><p v-if="shareCopyMessage" class="settings-feedback">{{ shareCopyMessage }}</p></template><p v-else class="settings-help">请先选择一条行程，再管理它的在线分享链接。</p></div>
                <div class="settings-note"><span class="settings-note-mark">i</span><span>分享链接是只读快照，默认有效期为 7 天。撤销后，持有链接的人将无法继续查看。</span></div>
              </section>

              <section v-else-if="settingsSection === 'about'" class="settings-page-section settings-about-section">
                <div class="settings-section-heading"><span class="eyebrow">ABOUT JOURNEYIN</span><h3>关于 JourneyIn</h3><p>了解当前版本、项目作者和 JourneyIn 的开源项目信息。</p></div>
                <div class="settings-card settings-about-hero"><BrandLogo :size="52" variant="mark" shape="squircle" class="settings-about-logo" /><div><strong>JourneyIn</strong><p>{{ APP_SLOGAN }}</p><a :href="GITHUB_URL" target="_blank" rel="noopener noreferrer">访问项目主页 ↗</a></div></div>
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
      <!-- 全屏照片大图预览 Lightbox (防坍塌视口 + 渐进式模糊占位 + Loading动效 + 左右切换) -->
      <div
        v-if="previewPhoto"
        class="photo-lightbox-modal"
        @click.self="closePhotoPreview()"
      >
        <div
          class="photo-lightbox-card"
          @touchstart.passive="handleLightboxTouchStart"
          @touchmove.passive="handleLightboxTouchMove"
          @touchend="handleLightboxTouchEnd"
        >
          <button type="button" class="photo-lightbox-close" aria-label="关闭预览" @click="closePhotoPreview()">×</button>
          
          <!-- 左侧上一张切换按钮 -->
          <button
            v-if="previewPhotoList.length > 1"
            type="button"
            class="photo-lightbox-nav prev"
            aria-label="上一张照片"
            @click.stop="prevPreviewPhoto"
          >
            <IonIcon :icon="chevronBackOutline" />
          </button>

          <!-- 稳定的固定比例图片视口：彻底解决弱网大图未加载时的布局坍塌 -->
          <div class="photo-lightbox-viewport">
            <!-- 1. 底层即时模糊缩略图占位 (几 KB，几乎 0 延迟，避免白屏或黑屏) -->
            <img
              :src="previewPhoto.thumb_url + '?size=120'"
              class="photo-lightbox-blur-bg"
              aria-hidden="true"
            />

            <!-- 2. 高清大图 (异步加载，加载完成后平滑淡入覆盖) -->
            <img
              :key="previewPhoto.id"
              :src="previewPhoto.url"
              class="photo-lightbox-img"
              :class="{ 'photo-loaded': !previewPhotoLoading && !previewPhotoError }"
              alt="大图预览"
              @load="onPreviewPhotoLoaded(previewPhoto.id)"
              @error="onPreviewPhotoError(previewPhoto.id)"
            />

            <!-- 3. 加载中微动效覆盖层 (明确展示加载反馈，防止操作迟滞感) -->
            <div v-if="previewPhotoLoading" class="photo-lightbox-loading">
              <div class="photo-loading-spinner"></div>
              <span>正在加载大图…</span>
            </div>

            <!-- 4. 加载异常提示与重试 -->
            <div v-if="previewPhotoError" class="photo-lightbox-error">
              <span>大图加载超时或网络异常</span>
              <button type="button" @click="setPreviewIndex(previewPhotoIndex)">重新加载</button>
            </div>
          </div>

          <!-- 右侧下一张切换按钮 -->
          <button
            v-if="previewPhotoList.length > 1"
            type="button"
            class="photo-lightbox-nav next"
            aria-label="下一张照片"
            @click.stop="nextPreviewPhoto"
          >
            <IonIcon :icon="chevronForwardOutline" />
          </button>

          <div class="photo-lightbox-footer">
            <div class="photo-lightbox-title-wrap">
              <strong>{{ previewPhoto.file_name }}</strong>
              <small v-if="previewPhotoList.length > 1" class="photo-lightbox-counter">{{ previewPhotoIndex + 1 }} / {{ previewPhotoList.length }}</small>
            </div>
            <span>拍摄于 {{ formatPhotoTime(previewPhoto.taken_at) }}</span>
          </div>
        </div>
      </div>

      <TripPosterModal
        :is-open="posterModalOpen"
        :trip="tripDocument"
        :share-url="shareURL"
        :current-theme="effectiveTheme"
        @close="posterModalOpen = false"
        @create-share="createShare"
      />
    </div>
  </IonApp>
</template>
