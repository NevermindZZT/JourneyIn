<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  IonApp, IonBadge, IonButton, IonChip, IonContent, IonHeader, IonIcon,
  IonPage, IonTitle, IonToolbar,
} from '@ionic/vue'
import BMapLoader from '@baidumap/jsapi-loader'
import { addOutline, closeOutline, cloudOfflineOutline, linkOutline, logInOutline, mapOutline, menuOutline, navigateOutline, searchOutline, settingsOutline, sunnyOutline } from 'ionicons/icons'

type Theme = 'system' | 'light' | 'dark'
type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord>; source?: string; provider_refs?: Record<string, unknown>; geocoded_at?: string; precision?: string; confidence?: number }
type LinkData = { id?: string; title: string; url: string; kind?: string }
type Stop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown>; children?: SubStop[] }
type SubStop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown> }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; mode?: string; snapshots?: Array<{ provider?: string; coordinate_system?: string; geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; fetched_at?: string }> }
type Day = { id: string; date: string; title?: string; notes_markdown?: string; stops: Stop[]; legs?: Leg[] }
type TripDocument = { title: string; timezone: string; description_markdown?: string; links?: LinkData[]; days: Day[] }
type SharedBootstrap = { trip: TripDocument & { id?: string; status?: string }; browser_key?: string; revision?: number }
type TripSummary = { id: string; title: string; status: string; start_date: string; end_date: string; timezone: string; revision: number; days?: number; stops?: number }
type Capabilities = { version?: string; map_providers?: { baidu?: { browser_key_configured?: boolean; browser_key?: string }; amap?: { browser_key_configured?: boolean } }; mcp?: { http_endpoint?: string } }
type KeySettings = { map?: { baidu?: { browser_key_configured?: boolean; server_key_configured?: boolean }; amap?: { js_key_configured?: boolean; server_key_configured?: boolean } }; poi?: { provider_priority?: 'amap' | 'baidu'; local_directory_count?: number } }
type PlaceCandidate = { id?: string; name: string; address?: string; location: Coord & { crs?: string }; provider?: string }
type TravelMode = 'driving' | 'walking' | 'cycling' | 'transit'

const APP_VERSION = '0.2.3'
const APP_SLOGAN = '在地图上规划每一段旅程'
const GITHUB_URL = 'https://github.com/NevermindZZT/JourneyIn'
const shareMode = window.location.pathname.startsWith('/s/') && !window.location.pathname.endsWith('.json')

declare global { interface Window { BMap?: any; BMapGL?: any; BMAP_NORMAL_MAP?: any; BMAP_SATELLITE_MAP?: any } }

const trips = ref<TripSummary[]>([])
const selected = ref<TripSummary | null>(null)
const tripDocument = ref<TripDocument | null>(null)
const capabilities = ref<Capabilities | null>(null)
const selectedStopId = ref('')
const selectedSubStopId = ref('')
const searchParentStopId = ref('')
const weatherLoading = ref(false)
const descriptionEditing = ref(false)
const descriptionDraft = ref('')
const descriptionSaving = ref(false)
const descriptionFullscreen = ref(false)
const tripDescriptionEditing = ref(false)
const tripDescriptionFullscreen = ref(false)
const tripDescriptionDraft = ref('')
const tripDescriptionSaving = ref(false)
const selectedDay = ref<number | 'all'>('all')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const mapError = ref('')
const mapWarning = ref('')
const mapReady = ref(false)
const mapContainer = ref<HTMLElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const shareURL = ref('')
const shareID = ref('')
const shareExpiresAt = ref('')
const actionLoading = ref(false)
const settingsOpen = ref(false)
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
const baiduBrowserKeyInput = ref('')
const baiduServerKeyInput = ref('')
const amapJSKeyInput = ref('')
const amapServerKeyInput = ref('')
const poiProviderPriority = ref<'amap' | 'baidu'>('amap')
const localDirectoryCount = ref(0)
const settingsSaving = ref(false)
const panelOpen = ref(localStorage.getItem('journeyin.panelOpen') !== 'false')
const tripView = ref<'list' | 'detail'>('list')
const mapType = ref<'normal' | 'satellite'>((localStorage.getItem('journeyin.mapType') as 'normal' | 'satellite') || 'normal')
const showMapLabels = ref(localStorage.getItem('journeyin.mapLabels') !== 'false')
const mapPickMode = ref(false)
const mapPickOpen = ref(false)
const mapPickTitle = ref('')
const mapPickAddress = ref('')
const mapPickDayID = ref('')
const mapPickLocation = ref<Coord & { crs: string } | null>(null)
const panelMode = ref<'journey' | 'search'>('journey')
const searchQuery = ref('')
const searchRegion = ref('')
const searchCategory = ref<'all' | '旅游景点' | '酒店' | '餐饮'>('all')
const searchResults = ref<PlaceCandidate[]>([])
const searchLoading = ref(false)
const searchMessage = ref('')
const planningMode = ref<TravelMode>('walking')
const planningLoading = ref(false)
const reorderMessage = ref('')
const reorderMode = ref(false)
const draggedStopID = ref('')
const dragOverStopID = ref('')
let mapInstance: any = null
let mapAPI: any = null
let mapScriptPromise: Promise<void> | null = null
let mediaQuery: MediaQueryList | null = null
let mapReadyTimer: number | null = null
let loadedMapKey = ''
let mapRenderVersion = 0

const key = computed(() => capabilities.value?.map_providers?.baidu?.browser_key || '')
const keyConfigured = computed(() => Boolean(key.value))
const visibleDays = computed(() => {
  if (!tripDocument.value) return []
  return selectedDay.value === 'all' ? tripDocument.value.days : tripDocument.value.days.filter((_, index) => index + 1 === selectedDay.value)
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
  let distanceM = 0; let durationS = 0; let segments = 0
  for (const day of visibleDays.value) for (const leg of day.legs || []) {
    const snapshot = (leg.snapshots || []).find(item => item.distance_m !== undefined || item.duration_s !== undefined)
    if (!snapshot) continue
    distanceM += snapshot.distance_m || 0; durationS += snapshot.duration_s || 0; segments++
  }
  return { distanceM, durationS, segments }
})
const hasCarryOverRoute = computed(() => {
  const carryOver = carryOverStop.value
  return Boolean(carryOver && visibleDays.value.some(day => (day.legs || []).some(leg => leg.from_stop_id === carryOver.id)))
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
function stopDate(stop: Stop | SubStop) { return tripDocument.value?.days.find(day => day.stops.some(item => item.id === stop.id || item.children?.some(child => child.id === stop.id)))?.date || '日期待定' }
function dayForStop(stop: Stop | SubStop) { return tripDocument.value?.days.find(day => day.stops.some(item => item.id === stop.id || item.children?.some(child => child.id === stop.id))) || null }
const themeLabel = computed(() => theme.value === 'system' ? '跟随系统' : theme.value === 'dark' ? '深色' : '浅色')

function apiFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  const headers = new Headers(init.headers)
  const token = authTokenInput.value.trim()
  if (token) headers.set('Authorization', 'Bearer ' + token)
  return fetch(input, { ...init, headers, credentials: 'same-origin' }).then(response => {
    if (response.status === 401) { authOpen.value = true; settingsMessage.value = '当前服务需要登录令牌' }
    return response
  })
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
    if (settingsResponse.ok) { const settings = await settingsResponse.json() as KeySettings; settingsData.value = settings; poiProviderPriority.value = settings.poi?.provider_priority === 'baidu' ? 'baidu' : 'amap'; localDirectoryCount.value = settings.poi?.local_directory_count || 0 }
    const currentID = selected.value?.id
    const nextTrip = trips.value.find(trip => trip.id === currentID) || trips.value[0]
    if (nextTrip) { await loadDetail(nextTrip); if (!currentID) tripView.value = 'list' }
    else { selected.value = null; tripDocument.value = null; tripView.value = 'list'; renderMap() }
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
  capabilities.value = { version: APP_VERSION, map_providers: { baidu: { browser_key_configured: Boolean(bootstrap.browser_key), browser_key: bootstrap.browser_key || '' } } }
  selectedDay.value = 'all'; tripView.value = 'detail'; panelMode.value = 'journey'; panelOpen.value = true; selectedStopId.value = ''; selectedSubStopId.value = ''; reorderMode.value = false; descriptionEditing.value = false; tripDescriptionEditing.value = false
  await nextTick(); await renderMap()
}
function selectTrip(trip: TripSummary) { tripView.value = 'detail'; void loadDetail(trip) }
async function deleteTrip(trip: TripSummary) {
  if (!window.confirm('确认删除“' + trip.title + '”吗？该行程及其规划点、路线和天气快照都会删除。')) return
  actionLoading.value = true; error.value = ''
  try { const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(trip.id), { method: 'DELETE', headers: { 'If-Match': 'revision-' + trip.revision } }); if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '删除行程失败') }; if (selected.value?.id === trip.id) { selected.value = null; tripDocument.value = null; tripView.value = 'list' }; await loadTrips() } catch (cause) { error.value = cause instanceof Error ? cause.message : '删除行程失败' } finally { actionLoading.value = false }
}
function deleteSelectedTrip() { if (selected.value) void deleteTrip(selected.value) }

async function loadDetail(trip: TripSummary) {
  selected.value = trip
  restoreShareState(trip.id)
  selectedStopId.value = ''
  detailLoading.value = true
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(trip.id))
    if (response.status === 401) return
    if (!response.ok) throw new Error('无法读取行程详情')
    const payload = await response.json() as { document?: TripDocument }
    tripDocument.value = payload.document || null
    selectedDay.value = 'all'
    await nextTick()
    await renderMap()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '行程详情读取失败'
    tripDocument.value = null
  } finally {
    detailLoading.value = false
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
    const response = await apiFetch('/api/v1/trips', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ schema_version: 1, title: newTitle.value.trim(), status: 'draft', locale: 'zh-CN', timezone: newTimezone.value, date_range: { start, end }, description_markdown: newDescription.value, links: [], map: { preferred_provider: 'baidu', enabled_providers: ['baidu', 'amap'], default_mode: 'walking' }, days, metadata: { source: 'human' } }) })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '新建旅行规划失败') }
    newTripOpen.value = false
    await loadTrips()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '新建旅行规划失败' } finally { actionLoading.value = false }
}

function gcj02ToBd09(point: Coord) {
  const x = point.lng; const y = point.lat; const z = Math.sqrt(x * x + y * y) + 0.00002 * Math.sin(y * Math.PI * 3000 / 180); const theta = Math.atan2(y, x) + 0.000003 * Math.cos(x * Math.PI * 3000 / 180); return { lng: z * Math.cos(theta) + 0.0065, lat: z * Math.sin(theta) + 0.006, crs: 'bd09ll' }
}
function savedLocationFor(candidate: PlaceCandidate): LocationData {
  const sourceCRS = candidate.location.crs || (candidate.provider === 'amap' ? 'gcj02' : 'bd09ll'); const coordinates: Record<string, Coord & { crs?: string }> = { [sourceCRS]: { lat: candidate.location.lat, lng: candidate.location.lng, crs: sourceCRS } }; if (sourceCRS === 'gcj02') coordinates.bd09ll = gcj02ToBd09(candidate.location); const provider = candidate.provider === 'amap' ? 'amap' : 'baidu'; return { preferred: coordinates.bd09ll ? 'bd09ll' : sourceCRS, coordinates, source: provider + '-place-search', provider_refs: candidate.id ? { [provider + '_uid']: candidate.id } : {}, geocoded_at: new Date().toISOString(), precision: 'poi' }
}
function pointFor(stop: Stop | SubStop): (Coord & { crs: string }) | null {
  const coordinates = stop.location?.coordinates
  if (!coordinates) return null
  const preferred = coordinates.bd09ll ? 'bd09ll' : stop.location?.preferred && coordinates[stop.location.preferred] ? stop.location.preferred : Object.keys(coordinates)[0]
  const point = preferred ? coordinates[preferred] : null
  if (!point || !Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return null
  return { ...point, crs: preferred }
}
function navigationPointFor(stop: Stop | SubStop, provider: 'baidu' | 'amap'): (Coord & { crs: string }) | null {
  const coordinates = stop.location?.coordinates
  if (!coordinates) return null
  const order = provider === 'amap' ? ['gcj02', 'bd09ll', 'wgs84'] : ['bd09ll', 'gcj02', 'wgs84']
  for (const crs of order) {
    const point = coordinates[crs]
    if (point && Number.isFinite(point.lat) && Number.isFinite(point.lng)) return { ...point, crs }
  }
  return null
}
function navigationPlatform(): 'android' | 'ios' | 'web' {
  const userAgent = navigator.userAgent
  if (/Android/i.test(userAgent)) return 'android'
  if (/iPhone|iPad|iPod/i.test(userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)) return 'ios'
  return 'web'
}
function reserveNavigationWindow(platform: 'android' | 'ios' | 'web') {
  if (platform !== 'web') return null
  const opened = window.open('about:blank', '_blank')
  if (opened) { try { opened.opener = null } catch { /* best effort */ } }
  return opened
}
function openNavigationURL(url: string, platform: 'android' | 'ios' | 'web', reservedWindow: Window | null, fallbackURL = '') {
  if (platform === 'web' && reservedWindow && !reservedWindow.closed) {
    reservedWindow.location.replace(url)
    return
  }
  if (platform === 'web') {
    window.location.assign(url)
    return
  }
  let appOpened = false
  const onPageHide = () => { appOpened = true }
  const onVisibilityChange = () => { if (document.visibilityState === 'hidden') appOpened = true }
  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('pagehide', onPageHide, { once: true })
  window.location.assign(url)
  if (fallbackURL) window.setTimeout(() => {
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener('pagehide', onPageHide)
    if (!appOpened && document.visibilityState === 'visible') window.location.assign(fallbackURL)
  }, 1800)
}
function mapPointFor(stop: Stop | SubStop): (Coord & { crs: string }) | null {
  const point = pointFor(stop)
  if (!point) return null
  return point.crs === 'gcj02' ? gcj02ToBd09(point) : point
}
function routePoint(value: [number, number] | Coord, crs: string) {
  if (Array.isArray(value)) return { lng: value[0], lat: value[1], crs }
  return { lng: value.lng, lat: value.lat, crs: (value as Coord & { crs?: string }).crs || crs }
}
function mapRoutePoint(value: [number, number] | Coord, crs: string): (Coord & { crs: string }) | null {
  const point = routePoint(value, crs)
  if (point.crs === 'gcj02') return gcj02ToBd09(point)
  return point.crs === 'bd09ll' ? point : null
}
function chooseSnapshot(leg: Leg) { return (leg.snapshots || []).find(snapshot => (snapshot.coordinate_system === 'bd09ll' || snapshot.coordinate_system === 'gcj02') && snapshot.geometry && snapshot.geometry.length > 1) || null }
function selectStop(stop: Stop) { selectedStopId.value = stop.id; selectedSubStopId.value = ''; void renderMap(); if (window.matchMedia('(max-width: 900px)').matches) { panelOpen.value = false; localStorage.setItem('journeyin.panelOpen', 'false') }; if (mapInstance && mapAPI) { const point = pointFor(stop); if (point) mapInstance.panTo(new mapAPI.Point(point.lng, point.lat)) } }
function selectSubStop(child: SubStop, parent: Stop) { selectedStopId.value = parent.id; selectedSubStopId.value = child.id; void renderMap(); if (window.matchMedia('(max-width: 900px)').matches) { panelOpen.value = false; localStorage.setItem('journeyin.panelOpen', 'false') }; if (mapInstance && mapAPI) { const point = pointFor(child); if (point) mapInstance.panTo(new mapAPI.Point(point.lng, point.lat)) } }
function openChildSearch(parent: Stop) { selectedStopId.value = parent.id; selectedSubStopId.value = ''; searchParentStopId.value = parent.id; panelOpen.value = true; panelMode.value = 'search'; searchMessage.value = '为“' + parent.title + '”添加子规划点' }
function beginEditDescription() { descriptionDraft.value = selectedTarget.value?.description_markdown || ''; descriptionEditing.value = true }
function cancelEditDescription() { descriptionEditing.value = false; descriptionDraft.value = ''; descriptionFullscreen.value = false }
function openDescriptionFullscreen() { descriptionFullscreen.value = true }
function closeDescriptionFullscreen() { descriptionFullscreen.value = false }
function beginEditTripDescription() { tripDescriptionDraft.value = tripDocument.value?.description_markdown || ''; tripDescriptionEditing.value = true }
function openTripDescriptionFullscreen() { tripDescriptionFullscreen.value = true }
function closeTripDescriptionFullscreen() { tripDescriptionFullscreen.value = false }
function cancelEditTripDescription() { tripDescriptionEditing.value = false; tripDescriptionDraft.value = ''; tripDescriptionFullscreen.value = false }
async function saveTripDescription() {
  if (!selected.value || !tripDocument.value) return
  const previous = tripDocument.value.description_markdown || ''; tripDocument.value.description_markdown = tripDescriptionDraft.value.trim(); tripDescriptionSaving.value = true; error.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(tripDocument.value) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存行程说明失败')
    applyTripPayload(payload); tripDescriptionEditing.value = false; tripDescriptionDraft.value = ''
  } catch (cause) { if (tripDocument.value) tripDocument.value.description_markdown = previous; error.value = cause instanceof Error ? cause.message : '保存行程说明失败' } finally { tripDescriptionSaving.value = false }
}
async function saveDescription() {
  if (!selected.value || !tripDocument.value || !selectedTarget.value) return
  const target = selectedTarget.value; const previous = target.description_markdown || ''; target.description_markdown = descriptionDraft.value.trim(); descriptionSaving.value = true; error.value = ''
  const parentID = selectedStop.value?.id || ''; const childID = selectedSubStop.value?.id || ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(tripDocument.value) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地点说明失败')
    applyTripPayload(payload); selectedStopId.value = parentID; selectedSubStopId.value = childID; descriptionEditing.value = false; descriptionFullscreen.value = false; descriptionDraft.value = ''
  } catch (cause) { target.description_markdown = previous; error.value = cause instanceof Error ? cause.message : '保存地点说明失败' } finally { descriptionSaving.value = false }
}

function resetBaiduMapSDK() {
  try { mapInstance?.destroy?.() } catch { /* SDK cleanup is best effort */ }
  mapInstance = null
  mapAPI = null
  mapReady.value = false
  mapWarning.value = ''
  try { BMapLoader.reset() } catch { /* loader reset is best effort */ }
  mapScriptPromise = null
  loadedMapKey = ''
}
async function loadBaiduMap() {
  const currentKey = key.value.trim()
  if (!currentKey) return
  if (loadedMapKey && loadedMapKey !== currentKey) resetBaiduMapSDK()
  if (mapAPI && typeof mapAPI.Map === 'function') return
  if (!mapScriptPromise) {
    mapScriptPromise = BMapLoader.load({ ak: currentKey, version: '4.0', timeout: 8000 }).then(namespace => {
      mapAPI = namespace
      loadedMapKey = currentKey
    })
  }
  try { await mapScriptPromise } catch (cause) { resetBaiduMapSDK(); throw cause }
}

async function renderMap() {
  if (!key.value || !mapContainer.value || !tripDocument.value) return
  const renderVersion = ++mapRenderVersion
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
      const snapshot = chooseSnapshot(leg)
      if (!snapshot || !snapshot.geometry) continue
      const line = snapshot.geometry.map(value => mapRoutePoint(value, snapshot.coordinate_system || 'bd09ll')).filter((point): point is Coord & { crs: string } => Boolean(point)).map(point => new mapAPI.Point(point.lng, point.lat))
      if (line.length > 1) {
        mapInstance.addOverlay(new mapAPI.Polyline(line, { strokeColor: '#006874', strokeWeight: 5, strokeOpacity: .82 }))
        attachRouteLabel(snapshot)
      }
    }
    if (points.length) mapInstance.setViewport(points)
    else mapInstance.centerAndZoom('中国', 5)
    applyMapType()
    mapError.value = ''
  } catch (cause) { mapReady.value = false; mapWarning.value = ''; mapError.value = cause instanceof Error ? cause.message : '地图初始化失败' }
}
function retryMap() { mapError.value = ''; mapWarning.value = ''; resetBaiduMapSDK(); void renderMap() }

function togglePanel() { panelOpen.value = !panelOpen.value; localStorage.setItem('journeyin.panelOpen', String(panelOpen.value)) }
function applyMapType() {
  if (!mapInstance || !mapAPI || typeof mapInstance.setMapType !== 'function') return
  const type = mapType.value === 'satellite' ? mapAPI.BMAP_SATELLITE_MAP || (window as any).BMAP_SATELLITE_MAP : mapAPI.BMAP_NORMAL_MAP || (window as any).BMAP_NORMAL_MAP
  if (type) mapInstance.setMapType(type)
}
function toggleMapType() { mapType.value = mapType.value === 'normal' ? 'satellite' : 'normal'; localStorage.setItem('journeyin.mapType', mapType.value); applyMapType() }
function toggleMapLabels() { showMapLabels.value = !showMapLabels.value; localStorage.setItem('journeyin.mapLabels', String(showMapLabels.value)); void renderMap() }
function toggleMapPick() { if (!mapReady.value || !tripDocument.value) { error.value = '地图加载完成后才能使用地图选点'; return }; mapPickMode.value = !mapPickMode.value; error.value = '' }
function handleMapClick(event: any) { if (!mapPickMode.value || !event?.point || !tripDocument.value) return; mapPickLocation.value = { lat: Number(event.point.lat), lng: Number(event.point.lng), crs: 'bd09ll' }; mapPickTitle.value = ''; mapPickAddress.value = ''; const day = selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]; mapPickDayID.value = day?.id || tripDocument.value.days[0]?.id || ''; mapPickMode.value = false; mapPickOpen.value = true }
function cancelMapPick() { mapPickOpen.value = false; mapPickLocation.value = null; mapPickTitle.value = ''; mapPickAddress.value = '' }
async function saveMapPick() {
  if (!selected.value || !tripDocument.value || !mapPickLocation.value || !mapPickTitle.value.trim() || !mapPickDayID.value) { error.value = '请填写地点名称并选择行程日期'; return }
  actionLoading.value = true; error.value = ''
  try {
    const point = mapPickLocation.value; const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(mapPickDayID.value) + '/stops', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ stop: { title: mapPickTitle.value.trim(), address: mapPickAddress.value.trim(), location: { preferred: 'bd09ll', coordinates: { bd09ll: point }, source: 'baidu-map-click', geocoded_at: new Date().toISOString(), precision: 'map-click' } } }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }; if (!response.ok) throw new Error(payload.error?.message || '保存地图选点失败'); applyTripPayload(payload); selectedDay.value = tripDocument.value.days.findIndex(day => day.id === mapPickDayID.value) + 1; cancelMapPick()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '保存地图选点失败' } finally { actionLoading.value = false }
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
  label.setStyle?.({ color: '#ffffff', backgroundColor: '#006874dd', border: '0', borderRadius: '999px', padding: '4px 8px', fontSize: '12px', lineHeight: '16px', whiteSpace: 'nowrap', boxShadow: '0 3px 10px #0003' })
  label.setPosition?.(new mapAPI.Point(middle.lng, middle.lat)); mapInstance.addOverlay(label)
}
function applyTripPayload(payload: { document?: TripDocument; revision?: number; stops?: number; days?: number }) {
  if (payload.document) tripDocument.value = payload.document
  if (selected.value) {
    selected.value = { ...selected.value, revision: payload.revision ?? selected.value.revision, stops: payload.stops ?? selected.value.stops, days: payload.days ?? selected.value.days }
    const index = trips.value.findIndex(trip => trip.id === selected.value?.id)
    if (index >= 0) trips.value[index] = { ...trips.value[index], revision: selected.value.revision, stops: selected.value.stops, days: selected.value.days }
  }
  selectedStopId.value = ''
}
async function searchPlaces() {
  if (!searchQuery.value.trim()) { searchMessage.value = '请输入景点、酒店、餐厅或地址'; return }
  searchLoading.value = true; searchMessage.value = ''; searchResults.value = []
  try {
    const response = await apiFetch('/api/v1/maps/pois/search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider: poiProviderPriority.value, query: searchQuery.value.trim(), region: searchRegion.value.trim(), category: searchCategory.value === 'all' ? undefined : searchCategory.value, page: 1, page_size: 10 }) })
    const payload = await response.json() as { items?: PlaceCandidate[]; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '地点搜索失败')
    searchResults.value = payload.items || []
    if (!searchResults.value.length) searchMessage.value = '没有找到结果，请补充城市或更换关键词'
  } catch (cause) { searchMessage.value = cause instanceof Error ? cause.message : '地点搜索失败' } finally { searchLoading.value = false }
}
async function addPlaceToTrip(candidate: PlaceCandidate) {
  if (!selected.value || !tripDocument.value) { searchMessage.value = '请先创建或选择一条旅行规划'; return }
  const day = selectedDay.value === 'all' ? tripDocument.value.days[0] : tripDocument.value.days[selectedDay.value - 1]
  if (!day) { searchMessage.value = '当前规划没有可用日期'; return }
  const location = candidate.location
  if (!location || !Number.isFinite(location.lat) || !Number.isFinite(location.lng)) { searchMessage.value = '搜索结果没有可靠坐标，未添加'; return }
  actionLoading.value = true
  const parentID = searchParentStopId.value
  const endpoint = parentID ? '/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(parentID) + '/children' : '/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops'
  try {
    const response = await apiFetch(endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ stop: { title: candidate.name, address: candidate.address, location: savedLocationFor(candidate) } }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '添加规划点失败')
    applyTripPayload(payload)
    if (parentID) { selectedStopId.value = parentID; selectedSubStopId.value = ''; searchMessage.value = '已添加“' + candidate.name + '”为子规划点'; } else { searchMessage.value = '已添加“' + candidate.name + '”，路线尚未生成' }
    searchParentStopId.value = ''; panelMode.value = 'journey'; searchResults.value = []
  } catch (cause) { searchMessage.value = cause instanceof Error ? cause.message : '添加规划点失败' } finally { actionLoading.value = false }
}

async function planRoutes() {
  if (!selected.value || !tripDocument.value) { error.value = '请先选择一条旅行规划'; return }
  if (!plannableDays.value.length) { error.value = '至少有两个相邻的带坐标规划点后才能生成路线'; return }
  planningLoading.value = true; error.value = ''
  try {
    const day = selectedDay.value === 'all' ? undefined : tripDocument.value.days[selectedDay.value - 1]?.id
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/plan', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ provider: 'baidu', mode: planningMode.value, day_id: day }) })
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
  shareURL.value = ''; shareID.value = ''; shareExpiresAt.value = ''
  try {
    const saved = JSON.parse(localStorage.getItem(shareStorageKey(tripID)) || 'null') as { id?: string; url?: string; expires_at?: string } | null
    if (!saved?.url || (saved.expires_at && Date.parse(saved.expires_at) <= Date.now())) { localStorage.removeItem(shareStorageKey(tripID)); return }
    shareID.value = saved.id || ''; shareURL.value = saved.url; shareExpiresAt.value = saved.expires_at || ''
  } catch { localStorage.removeItem(shareStorageKey(tripID)) }
}
async function createShare() {
  if (!selected.value) return
  const tripID = selected.value.id; const existingToken = shareTokenFromURL(shareURL.value) || (() => { try { const saved = JSON.parse(localStorage.getItem(shareStorageKey(tripID)) || 'null') as { url?: string } | null; return saved?.url ? shareTokenFromURL(saved.url) : '' } catch { return '' } })()
  actionLoading.value = true; error.value = ''
  try {
    const response = await apiFetch('/api/v1/shares', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ trip_id: tripID, existing_token: existingToken || undefined }) })
    const payload = await response.json() as { id?: string; url?: string; expires_at?: string; error?: { message?: string } }
    if (!response.ok || !payload.url) throw new Error(payload.error?.message || '分享链接创建失败')
    shareID.value = payload.id || ''; shareURL.value = payload.url; shareExpiresAt.value = payload.expires_at || ''; saveShareState(tripID)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '分享链接创建失败' } finally { actionLoading.value = false }
}
async function copyShareURL() {
  if (!shareURL.value) return
  try { await navigator.clipboard.writeText(shareURL.value); settingsMessage.value = '分享链接已复制' } catch { error.value = '复制失败，请手动复制分享链接' }
}
async function revokeShare() {
  if (!shareID.value || !window.confirm('确认撤销当前在线分享吗？撤销后链接将无法访问。')) return
  actionLoading.value = true
  try {
    const response = await apiFetch('/api/v1/shares/' + encodeURIComponent(shareID.value) + '/revoke', { method: 'POST' })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '撤销分享失败') }
    shareURL.value = ''; shareID.value = ''; shareExpiresAt.value = ''; if (selected.value) localStorage.removeItem(shareStorageKey(selected.value.id)); settingsMessage.value = '在线分享已撤销'
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '撤销分享失败' } finally { actionLoading.value = false }
}
function safeURL(raw: string) { try { const parsed = new URL(raw); return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : '#' } catch { return '#' } }
function escapeHTML(value: string) { return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('\\\"', '&quot;') }
function renderMarkdown(source: string) {
  const escaped = escapeHTML(source)
  const formatted = escaped.replace(/^### (.+)$/gm, '<h4>$1</h4>').replace(/^## (.+)$/gm, '<h3>$1</h3>').replace(/^# (.+)$/gm, '<h2>$1</h2>').replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>').replace(/\*([^*]+?)\*/g, '<em>$1</em>').replace(/\[([^\]]+)\]\((https?:\/\/[^) ]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
  return formatted.split(/\n{2,}/).map(block => block.startsWith('<h') ? block : '<p>' + block.replaceAll('\n', '<br>') + '</p>').join('')
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
function weatherUpdatedAt(stop: Stop | SubStop) { const value = stop.weather?.fetched_at; return value ? new Date(String(value)).toLocaleString() : '' }
async function refreshWeather() {
  if (!selected.value || !tripDocument.value || !selectedTarget.value) { error.value = '请先选择一个有坐标的规划点'; return }
  const day = dayForStop(selectedTarget.value); const parent = selectedStop.value; if (!day || !parent) { error.value = '无法确定天气对应日期'; return }
  weatherLoading.value = true; error.value = ''
  const childID = selectedSubStop.value?.id || ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(selectedTarget.value.id) + '/weather', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ provider: 'baidu', local_date: day.date }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '天气查询失败')
    applyTripPayload(payload); selectedStopId.value = parent.id; selectedSubStopId.value = childID
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '天气查询失败' } finally { weatherLoading.value = false }
}
function closeDetail() { selectedStopId.value = ''; selectedSubStopId.value = ''; void renderMap() }
function parentForStop(stop: Stop | SubStop, day: Day | null = dayForStop(stop)) {
  if (!day) return null
  return day.stops.find(parent => parent.id === stop.id) || day.stops.find(parent => parent.children?.some(child => child.id === stop.id)) || null
}
function isChildStop(stop: Stop | SubStop) {
  const parent = parentForStop(stop)
  return Boolean(parent && parent.id !== stop.id)
}
function toggleReorderMode() {
  reorderMode.value = !reorderMode.value
  draggedStopID.value = ''
  dragOverStopID.value = ''
  reorderMessage.value = reorderMode.value ? '请拖动规划点到目标位置；松开鼠标后立即保存。' : ''
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
function startPlanningPointDrag(event: DragEvent, stop: Stop | SubStop) {
  if (!reorderMode.value) { event.preventDefault(); return }
  draggedStopID.value = stop.id
  dragOverStopID.value = ''
  if (event.dataTransfer) { event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', stop.id) }
}
function dragOverPlanningPoint(event: DragEvent, stop: Stop | SubStop) {
  if (!reorderMode.value || !draggedStopID.value || draggedStopID.value === stop.id) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dragOverStopID.value = stop.id
}
function dragLeavePlanningPoint(event: DragEvent, stop: Stop | SubStop) {
  const current = event.currentTarget as HTMLElement | null
  const related = event.relatedTarget as Node | null
  if (current && related && current.contains(related)) return
  if (dragOverStopID.value === stop.id) dragOverStopID.value = ''
}
function endPlanningPointDrag() {
  draggedStopID.value = ''
  dragOverStopID.value = ''
}
function startPlanningPointPointer(event: PointerEvent, stop: Stop | SubStop) {
  if (!reorderMode.value || event.pointerType === 'mouse') return
  event.preventDefault()
  draggedStopID.value = stop.id
  dragOverStopID.value = ''
}
function enterPlanningPointPointer(event: PointerEvent, stop: Stop | SubStop) {
  if (!reorderMode.value || event.pointerType === 'mouse' || !draggedStopID.value || draggedStopID.value === stop.id) return
  event.preventDefault()
  dragOverStopID.value = stop.id
}
async function dropPlanningPointPointer(event: PointerEvent, target: Stop | SubStop) {
  if (!reorderMode.value || event.pointerType === 'mouse' || !draggedStopID.value) return
  event.preventDefault()
  const source = findPlanningPoint(draggedStopID.value)
  draggedStopID.value = ''
  dragOverStopID.value = ''
  if (source) await commitPlanningPointDrop(source, target)
}
async function dropPlanningPoint(event: DragEvent, target: Stop | SubStop) {
  if (!reorderMode.value) return
  event.preventDefault()
  const sourceID = draggedStopID.value || event.dataTransfer?.getData('text/plain') || ''
  const source = findPlanningPoint(sourceID)
  draggedStopID.value = ''
  dragOverStopID.value = ''
  if (source) await commitPlanningPointDrop(source, target)
}
async function commitPlanningPointDrop(source: Stop | SubStop, target: Stop | SubStop) {
  if (source.id === target.id) return
  const sourceDay = dayForStop(source); const targetDay = dayForStop(target)
  const sourceParent = parentForStop(source, sourceDay); const targetParent = parentForStop(target, targetDay)
  if (!sourceDay || !targetDay || sourceDay.id !== targetDay.id || isChildStop(source) !== isChildStop(target) || (isChildStop(source) && sourceParent?.id !== targetParent?.id)) {
    reorderMessage.value = '只能在同一天的同一层级内拖动排序'
    return
  }
  await reorderPlanningPointTo(source, target.sequence)
}
async function reorderPlanningPointTo(stop: Stop | SubStop, targetSequence: number) {
  if (!selected.value) return
  const day = dayForStop(stop); const parent = parentForStop(stop, day); if (!day || !parent) return
  const child = isChildStop(stop); const parentID = parent.id; const stopID = stop.id; const revision = selected.value.revision
  actionLoading.value = true; error.value = ''; reorderMessage.value = ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/days/' + encodeURIComponent(day.id) + '/stops/' + encodeURIComponent(stopID) + '/move', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + revision }, body: JSON.stringify({ target_sequence: targetSequence }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) {
      if (response.status === 409 && selected.value) { await loadDetail(selected.value); throw new Error('行程已被其他操作更新，请重新选择后再排序') }
      throw new Error(payload.error?.message || '调整规划点顺序失败')
    }
    applyTripPayload(payload)
    selectedStopId.value = child ? parentID : stopID; selectedSubStopId.value = child ? stopID : ''
    reorderMessage.value = child ? '子规划点顺序已更新' : '规划点顺序已更新，路线已清除，请点击“生成路线”重新规划'
    await renderMap()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '调整规划点顺序失败' } finally { actionLoading.value = false }
}
async function deletePlanningPoint(stop: Stop | SubStop) {
  if (!selected.value) return
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
    poiProviderPriority.value = settingsData.value.poi?.provider_priority === 'baidu' ? 'baidu' : 'amap'
    localDirectoryCount.value = settingsData.value.poi?.local_directory_count || 0
    baiduBrowserKeyInput.value = ''
    baiduServerKeyInput.value = ''
    amapJSKeyInput.value = ''
    amapServerKeyInput.value = ''
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '无法读取设置' }
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
    if (!Object.keys(body).length) { settingsMessage.value = '未填写新的 Key，现有配置保持不变。'; return }
    const response = await apiFetch('/api/v1/settings/map-keys', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    const payload = await response.json() as { error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地图 Key 失败')
    settingsMessage.value = '地图 Key 已保存到 SQLite；浏览器端 Key 已立即生效。'
    baiduServerKeyInput.value = ''; amapJSKeyInput.value = ''; amapServerKeyInput.value = ''
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
onMounted(() => { applyTheme(); mediaQuery = window.matchMedia('(prefers-color-scheme: dark)'); mediaQuery.addEventListener?.('change', systemThemeChanged); if (shareMode) void loadSharedTrip(); else loadTrips() })
onUnmounted(() => mediaQuery?.removeEventListener?.('change', systemThemeChanged))
</script>

<template>
  <IonApp>
    <IonPage class="journey-page">
      <IonHeader translucent>
        <IonToolbar>
          <IonTitle><span class="brand-dot"></span><span class="brand-name">JourneyIn</span><span class="brand-slogan">{{ APP_SLOGAN }}</span><a class="brand-github" :href="GITHUB_URL" target="_blank" rel="noopener noreferrer">GitHub</a><span class="app-version">v{{ capabilities?.version || APP_VERSION }}</span></IonTitle>
          <div v-if="!shareMode" slot="end" class="header-actions">
            <IonButton fill="clear" aria-label="显示或隐藏行程面板" @click="togglePanel"><IonIcon :icon="menuOutline" /></IonButton>
            <IonButton fill="clear" aria-label="打开设置" @click="openSettings()"><IonIcon :icon="settingsOutline" /></IonButton>
            <IonButton fill="clear" aria-label="新建旅行规划" @click="newTripOpen = true"><IonIcon :icon="addOutline" /></IonButton>
          </div>
        </IonToolbar>
      </IonHeader>
      <IonContent :fullscreen="true">

        <main class="map-shell">
          <div class="map-canvas" :class="{ 'map-pick-active': mapPickMode }">
            <div v-if="keyConfigured && tripDocument && !mapError" ref="mapContainer" id="bmap"></div>
            <div v-if="!tripDocument || !keyConfigured || mapError" class="map-fallback">
              <IonIcon :icon="mapOutline" />
              <strong>{{ !tripDocument ? '选择或创建一条旅行规划' : mapError || '百度地图未配置' }}</strong>
              <span>{{ !tripDocument ? '创建或导入 Trip 后显示地图。' : mapError ? '请确认浏览器端 AK、JSAPI 服务、域名白名单和网络连接。当前页面：' + serverURL : '配置浏览器端 Key 后显示真实地图。当前不会伪造道路或路线。' }}</span>
              <IonChip color="warning"><IonIcon :icon="cloudOfflineOutline" /> {{ !tripDocument ? '等待行程' : '降级模式' }}</IonChip>
            </div>
            <div v-if="keyConfigured && tripDocument && !mapError && !mapReady && !mapWarning" class="map-loading"><IonIcon :icon="mapOutline" /><span>正在加载百度地图…</span></div>
            <div v-if="mapWarning" class="map-warning"><span>{{ mapWarning }}</span><button type="button" @click="retryMap">重新加载</button></div>
          </div>
          <div v-if="!selectedStop" class="map-hud">
            <button v-if="!panelOpen" class="panel-open-button" aria-label="显示行程面板" @click="togglePanel"><IonIcon :icon="menuOutline" /> 行程面板</button>
            <button class="map-type-button" :class="{ active: mapType === 'satellite' }" @click="toggleMapType">{{ mapType === 'satellite' ? '标准图' : '卫星图' }}</button>
            <button class="map-label-button" :class="{ active: showMapLabels }" @click="toggleMapLabels">{{ showMapLabels ? '隐藏标签' : '显示标签' }}</button>
            <button v-if="!shareMode" class="map-pick-button" :class="{ active: mapPickMode }" :disabled="!mapReady || !tripDocument" @click="toggleMapPick">{{ mapPickMode ? '取消选点' : '地图选点' }}</button>
            <div class="map-status-pill"><IonIcon :icon="keyConfigured && mapReady && !mapError ? sunnyOutline : cloudOfflineOutline" /> {{ !tripDocument ? '等待行程' : !keyConfigured ? '离线数据可用' : mapError ? '百度地图不可用' : mapWarning ? '百度地图底图加载中' : mapReady ? '百度地图已连接' : '百度地图加载中' }} · {{ visibleStops.length }} 个规划点</div>
          </div>
          <section v-if="error || shareURL" class="map-notices">
            <div v-if="error" class="global-error">{{ error }}<button aria-label="关闭错误" @click="error = ''">×</button></div>
            <div v-if="shareURL" class="share-banner"><span>只读分享：<a :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a><small v-if="shareExpiresAt">有效期至 {{ new Date(shareExpiresAt).toLocaleString() }}</small></span><div class="share-actions"><button type="button" @click="copyShareURL">复制链接</button><button v-if="shareID" type="button" @click="revokeShare">撤销分享</button><button aria-label="关闭分享提示" @click="shareURL = ''">×</button></div></div>
          </section>
          <aside v-if="panelOpen" class="floating-panel plan-panel" aria-label="行程规划面板">
            <div class="panel-head">
              <div><p class="eyebrow">JOURNEYIN</p><h2>{{ tripView === 'detail' && selected ? selected.title : '旅行规划' }}</h2><p class="panel-subtitle">{{ tripView === 'detail' && selected ? selected.start_date + ' — ' + selected.end_date : '选择一条行程查看详情' }}</p></div>
              <button class="panel-close" aria-label="隐藏行程面板" @click="togglePanel">×</button>
            </div>
            <button v-if="!shareMode && tripView === 'detail' && selected" class="panel-back panel-detail-back" @click="tripView = 'list'">‹ 行程列表</button>
            <div class="panel-tabs">
              <button :class="{ selected: panelMode === 'journey' }" @click="panelMode = 'journey'">行程</button>
              <button v-if="!shareMode" :class="{ selected: panelMode === 'search' }" :disabled="!selected" @click="searchParentStopId = ''; panelMode = 'search'"><IonIcon :icon="searchOutline" /> 添加地点</button>
            </div>
            <div v-if="!shareMode" class="data-actions"><button type="button" @click="openImportPicker">导入 Trip</button><button v-if="tripView === 'detail' && selected" type="button" :disabled="actionLoading" @click="downloadTrip">导出 JSON</button><button v-if="tripView === 'detail' && selected" type="button" :disabled="actionLoading" @click="createShare">在线分享</button><input ref="fileInput" class="visually-hidden" type="file" accept="application/json,.json" aria-hidden="true" tabindex="-1" @change="importTrip" /></div>
            <div class="panel-scroll">
              <template v-if="panelMode === 'journey'">
                <template v-if="tripView === 'list'">
                  <div class="section-heading"><div><p class="eyebrow">YOUR JOURNEYS</p><h3>旅行规划</h3></div><IonBadge color="primary">{{ trips.length }}</IonBadge></div>
                  <p v-if="loading" class="muted">正在加载…</p>
                  <div v-else-if="!trips.length" class="empty"><p>还没有旅行规划。</p><IonButton size="small" @click="newTripOpen = true"><IonIcon slot="start" :icon="addOutline" /> 新建规划</IonButton><p class="empty-hint">也可以导入已有 Trip JSON。</p></div>
                  <div v-else class="trip-cards">
                    <article v-for="trip in trips" :key="trip.id" class="trip-card" :class="{ active: selected?.id === trip.id }">
                      <button class="trip-card-main" @click="selectTrip(trip)"><span><b>{{ trip.title }}</b><small>{{ trip.start_date }} — {{ trip.end_date }}</small><small>{{ trip.days ?? '—' }} 天 · {{ trip.stops ?? '—' }} 个规划点</small></span><IonChip color="light">v{{ trip.revision }}</IonChip></button>
                      <button class="icon-delete trip-delete" :aria-label="'删除行程 ' + trip.title" @click="deleteTrip(trip)">×</button>
                    </article>
                  </div>
                </template>
                <template v-else-if="tripView === 'detail' && selected && tripDocument">
                  <div class="trip-detail-kicker"><span class="eyebrow">SELECTED JOURNEY</span><span>{{ selected.start_date }} — {{ selected.end_date }}</span></div>
                  <div class="trip-overview"><div class="description-header"><h3>行程总体说明</h3><span v-if="!shareMode" class="description-header-actions"><button v-if="!tripDescriptionEditing" class="text-button" @click="beginEditTripDescription">编辑行程说明</button><button v-if="tripDescriptionEditing" class="text-button" @click="openTripDescriptionFullscreen">全屏编辑</button></span></div><template v-if="tripDescriptionEditing"><textarea v-model="tripDescriptionDraft" class="description-editor" rows="5" placeholder="补充整个行程的背景、节奏和注意事项"></textarea><div class="description-actions"><button class="text-button" @click="cancelEditTripDescription">取消</button><button class="primary-text-button" :disabled="tripDescriptionSaving" @click="saveTripDescription">{{ tripDescriptionSaving ? '保存中…' : '保存说明' }}</button></div></template><div v-else-if="tripDocument.description_markdown" class="markdown" v-html="renderMarkdown(tripDocument.description_markdown)"></div><p v-else class="muted">{{ shareMode ? '暂无行程总体说明。' : '暂无行程总体说明，点击“编辑行程说明”添加。' }}</p></div>
                  <div class="panel-section">
                    <div class="section-heading compact"><div><p class="eyebrow">ITINERARY</p><h3>规划点</h3></div><div class="itinerary-actions"><span class="count-label">{{ visibleStops.length }} 个</span><button v-if="!shareMode" class="text-button reorder-toggle" :class="{ selected: reorderMode }" :aria-pressed="reorderMode" @click="toggleReorderMode">{{ reorderMode ? '完成排序' : '调整顺序' }}</button></div></div>
                    <div class="day-tabs"><button :class="{ selected: selectedDay === 'all' }" @click="selectedDay = 'all'">全程</button><button v-for="(_, index) in tripDocument.days" :key="index" :class="{ selected: selectedDay === index + 1 }" @click="selectedDay = index + 1">D{{ index + 1 }} · {{ tripDocument.days[index].date }}</button></div>
                    <div v-if="!shareMode" class="plan-controls"><label>路线方式<select v-model="planningMode"><option value="walking">步行</option><option value="driving">驾车</option><option value="cycling">骑行</option><option value="transit">公交</option></select></label><IonButton size="small" :disabled="planningLoading || !plannableDays.length" @click="planRoutes"><IonIcon slot="start" :icon="navigateOutline" /> {{ planningLoading ? '规划中…' : '生成路线' }}</IonButton></div>
                    <div class="route-summary"><div><span class="route-summary-label">{{ selectedDay === 'all' ? '全程路线' : 'D' + selectedDay + ' 当天路线' }}</span><strong v-if="visibleRouteSummary.segments">{{ formatDistance(visibleRouteSummary.distanceM) || '距离未知' }} · {{ formatDuration(visibleRouteSummary.durationS) || '时间未知' }}</strong><span v-else>尚未生成路线</span></div><small v-if="visibleRouteSummary.segments">{{ visibleRouteSummary.segments }} 段相邻路线</small></div>
                    <p v-if="hasCarryOverRoute" class="route-ready">路线从前一天的最后一个规划点“{{ carryOverStop?.title }}”开始。</p>
                    <p v-if="!shareMode" class="order-help">{{ reorderMode ? '排序模式已开启：拖动每行左侧的 ⋮⋮ 手柄到目标位置，松开后立即保存。' : '需要调整规划点顺序？点击上方“调整顺序”，然后拖动左侧手柄。' }}</p>
                    <p v-if="reorderMessage" class="inline-message">{{ reorderMessage }}</p>
                    <p v-if="!plannableDays.length" class="hint">{{ shareMode ? '当前选择范围暂无可生成的路线。' : '添加至少两个相邻的带坐标规划点后，点击“生成路线”。跨天行程会自动从前一天最后一个点接续。' }}</p>
                    <p v-else-if="visibleDays.some(day => day.legs?.some(leg => leg.snapshots?.length))" class="route-ready"><IonIcon :icon="navigateOutline" /> 已有路线快照；重复点击会优先使用缓存。</p>
                    <div v-if="visibleStops.length" class="stop-list"><article v-for="stop in visibleStops" :key="stop.id" class="stop-row" :class="{ selected: selectedStopId === stop.id, 'reorder-dragging': draggedStopID === stop.id, 'reorder-drop-target': dragOverStopID === stop.id }" :draggable="reorderMode" @dragstart="startPlanningPointDrag($event, stop)" @dragover="dragOverPlanningPoint($event, stop)" @dragleave="dragLeavePlanningPoint($event, stop)" @drop="dropPlanningPoint($event, stop)" @dragend="endPlanningPointDrag" @pointerenter="enterPlanningPointPointer($event, stop)" @pointerup="dropPlanningPointPointer($event, stop)"><span v-if="reorderMode" class="drag-handle" aria-hidden="true" @pointerdown.stop="startPlanningPointPointer($event, stop)">⋮⋮</span><button class="stop-row-main" @click="selectStop(stop)"><span class="stop-number">{{ stop.sequence }}</span><span><b>{{ stop.title }}</b><small>{{ stopDate(stop) }} · {{ stop.address || '地址已保存' }}</small></span><span class="stop-arrow">›</span></button><button v-if="!shareMode" class="icon-delete stop-delete" :aria-label="'删除规划点 ' + stop.title" @click.stop="deletePlanningPoint(stop)">×</button></article></div>
                    <p v-else class="empty compact-empty">当前日期还没有规划点。</p>
                    <button v-if="!shareMode" class="add-place-cta" @click="searchParentStopId = ''; panelMode = 'search'"><IonIcon :icon="searchOutline" /> 搜索并添加规划点</button>
                  </div>
                </template>
              </template>
              <template v-else>
                <div class="section-heading"><div><p class="eyebrow">PLAN A STOP</p><h3>{{ searchParentStopId ? '添加子规划点' : '搜索地点' }}</h3></div><button class="panel-back" @click="panelMode = 'journey'">返回行程</button></div>
                <form class="search-form" @submit.prevent="searchPlaces">
                  <label>地点或关键词<input v-model="searchQuery" placeholder="例如：甘加、白石崖、西湖、咖啡馆" autocomplete="off" /></label>
                  <label>城市/区域（可选）<input v-model="searchRegion" placeholder="例如：甘肃省夏河县" autocomplete="address-level2" /></label>
                  <label>搜索类型<select v-model="searchCategory"><option value="all">全部地点</option><option value="旅游景点">景点</option><option value="酒店">酒店</option><option value="餐饮">餐饮</option></select></label>
                  <IonButton type="submit" expand="block" :disabled="searchLoading"><IonIcon slot="start" :icon="searchOutline" /> {{ searchLoading ? '搜索中…' : '搜索地点' }}</IonButton>
                </form>
                <p class="search-help">先查询本地 7 天地点目录，未命中后调用当前优先 Provider（高德或百度）；选择结果后保存名称、地址、坐标、CRS 和 Provider UID。景点类型会使用分类检索，Provider 不可用时自动回退另一家。</p>
                <p v-if="searchMessage" class="inline-message">{{ searchMessage }}</p>
                <div class="search-results"><article v-for="(result, index) in searchResults" :key="result.id || result.name + index" class="search-result"><div><h4>{{ result.name }}</h4><p>{{ result.address || '地址待补充' }}</p><small v-if="result.location">{{ result.location.crs || '坐标' }} · {{ result.location.lat.toFixed(6) }}, {{ result.location.lng.toFixed(6) }}</small></div><IonButton size="small" @click="addPlaceToTrip(result)">{{ searchParentStopId ? '添加子点' : '添加' }}</IonButton></article></div>
              </template>
            </div>
          </aside>
          <aside v-if="selectedStop" class="details-drawer">
            <div class="details-scroll">
            <button class="close-button" aria-label="关闭详情" @click="closeDetail"><IonIcon :icon="closeOutline" /></button>
            <p class="eyebrow">{{ selectedSubStop ? 'SUB-STOP ' + selectedSubStop.sequence : 'STOP ' + selectedStop.sequence }}</p>
            <button v-if="selectedSubStop" class="detail-parent" @click="selectedSubStopId = ''">返回主规划点：{{ selectedStop.title }}</button>
            <h2>{{ selectedTarget?.title }}</h2>
            <p class="address">{{ selectedTarget?.address || '地址待解析' }}</p>
            <p class="stop-date">行程日期：{{ stopDate(selectedTarget || selectedStop) }}</p>
            <div class="saved-location"><span>坐标已保存</span><small>{{ pointFor(selectedTarget || selectedStop)?.crs || '未知 CRS' }} · {{ pointFor(selectedTarget || selectedStop)?.lat.toFixed(6) }}, {{ pointFor(selectedTarget || selectedStop)?.lng.toFixed(6) }}</small></div>
            <div class="weather"><IonIcon :icon="sunnyOutline" /><span>{{ weatherText(selectedTarget || selectedStop) }}<small v-if="weatherUpdatedAt(selectedTarget || selectedStop)">更新于 {{ weatherUpdatedAt(selectedTarget || selectedStop) }}</small></span></div>
            <IonButton v-if="!shareMode" size="small" fill="outline" :disabled="weatherLoading" @click="refreshWeather">{{ weatherLoading ? '天气查询中…' : selectedTarget?.weather ? '刷新天气' : '获取天气' }}</IonButton>
            <div v-if="!selectedSubStop" class="children-panel"><div class="section-heading compact"><div><p class="eyebrow">CHILD POINTS</p><h3>子规划点</h3></div><div class="itinerary-actions"><span class="count-label">{{ selectedStop.children?.length || 0 }} 个</span><span v-if="reorderMode" class="sort-badge">可拖动</span></div></div><p class="children-help">{{ reorderMode ? '拖动子规划点左侧的 ⋮⋮ 手柄调整顺序。' : '进入主规划点详情后，子规划点才会显示在地图上。' }}</p><div v-if="selectedStop.children?.length" class="child-list"><div v-for="child in selectedStop.children" :key="child.id" class="child-row-wrap" :class="{ 'reorder-dragging': draggedStopID === child.id, 'reorder-drop-target': dragOverStopID === child.id }" :draggable="reorderMode" @dragstart="startPlanningPointDrag($event, child)" @dragover="dragOverPlanningPoint($event, child)" @dragleave="dragLeavePlanningPoint($event, child)" @drop="dropPlanningPoint($event, child)" @dragend="endPlanningPointDrag" @pointerenter="enterPlanningPointPointer($event, child)" @pointerup="dropPlanningPointPointer($event, child)"><span v-if="reorderMode" class="drag-handle" aria-hidden="true" @pointerdown.stop="startPlanningPointPointer($event, child)">⋮⋮</span><button class="child-row" :class="{ selected: selectedSubStopId === child.id }" @click="selectSubStop(child, selectedStop)"><span class="child-number">{{ child.sequence }}</span><span><b>{{ child.title }}</b><small>{{ stopDate(child) }} · {{ child.address || '地址已保存' }}</small></span><span>›</span></button><button v-if="!shareMode" class="icon-delete child-delete" :aria-label="'删除子规划点 ' + child.title" @click.stop="deletePlanningPoint(child)">×</button></div></div><button v-if="!shareMode" class="add-place-cta" @click="openChildSearch(selectedStop)"><IonIcon :icon="searchOutline" /> 添加子规划点</button></div>
            <div class="description-section"><div class="description-header"><h3>地点说明</h3><span v-if="!shareMode" class="description-header-actions"><button v-if="descriptionEditing" class="text-button" @click="openDescriptionFullscreen">全屏编辑</button><button v-if="!descriptionEditing" class="text-button" @click="beginEditDescription">编辑地点说明</button></span></div><template v-if="descriptionEditing"><textarea v-model="descriptionDraft" class="description-editor" rows="7" placeholder="补充这个地点的门票、开放时间、行程备注等信息"></textarea><div class="description-actions"><button class="text-button" @click="cancelEditDescription">取消</button><button class="primary-text-button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存说明' }}</button></div></template><template v-else><div v-if="selectedTarget?.description_markdown" class="markdown" v-html="renderMarkdown(selectedTarget.description_markdown)"></div><p v-else class="muted">{{ shareMode ? '暂无地点说明。' : '暂无地点说明，点击“编辑地点说明”添加。' }}</p></template></div>
            <div class="links" v-if="selectedTarget?.links?.length"><a v-for="link in selectedTarget.links" :key="link.id || link.url" :href="safeURL(link.url)" target="_blank" rel="noopener noreferrer"><IonIcon :icon="linkOutline" /> {{ link.title }}</a></div>
            <div class="nav-actions"><IonButton size="small" @click="openNavigation('baidu')"><IonIcon slot="start" :icon="navigateOutline" /> 百度导航</IonButton><IonButton size="small" fill="outline" @click="openNavigation('amap')"><IonIcon slot="start" :icon="navigateOutline" /> 高德导航</IonButton><button v-if="!shareMode" class="danger-button detail-delete" @click="deletePlanningPoint(selectedTarget || selectedStop)">{{ selectedSubStop ? '删除子规划点' : '删除规划点' }}</button></div>
            </div>
          </aside>
        </main>
      </IonContent>
      <div v-if="descriptionFullscreen && descriptionEditing" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-description-title"><header><h2 id="fullscreen-description-title">编辑地点说明</h2><button class="modal-close" aria-label="退出全屏编辑" @click="closeDescriptionFullscreen">×</button></header><textarea v-model="descriptionDraft" class="fullscreen-description-editor" autofocus placeholder="补充门票、开放时间、行程备注等信息"></textarea><div class="description-actions"><button class="text-button" @click="cancelEditDescription">取消</button><button class="primary-text-button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存说明' }}</button></div></section></div>
      <div v-if="tripDescriptionFullscreen && tripDescriptionEditing" class="fullscreen-editor-backdrop"><section class="fullscreen-editor" role="dialog" aria-modal="true" aria-labelledby="fullscreen-trip-description-title"><header><h2 id="fullscreen-trip-description-title">编辑行程总体说明</h2><button class="modal-close" aria-label="退出全屏编辑" @click="closeTripDescriptionFullscreen">×</button></header><textarea v-model="tripDescriptionDraft" class="fullscreen-description-editor" autofocus placeholder="补充整个行程的背景、节奏和注意事项"></textarea><div class="description-actions"><button class="text-button" @click="cancelEditTripDescription">取消</button><button class="primary-text-button" :disabled="tripDescriptionSaving" @click="saveTripDescription">{{ tripDescriptionSaving ? '保存中…' : '保存说明' }}</button></div></section></div>
      <div v-if="mapPickOpen" class="modal-backdrop" @click.self="cancelMapPick"><section class="modal-panel map-pick-panel" role="dialog" aria-modal="true" aria-labelledby="map-pick-title"><button class="modal-close" aria-label="取消地图选点" @click="cancelMapPick">×</button><p class="eyebrow">MAP PICK</p><h2 id="map-pick-title">保存地图选点</h2><p class="map-pick-coordinate">{{ mapPickLocation?.crs }} · {{ mapPickLocation?.lat.toFixed(6) }}, {{ mapPickLocation?.lng.toFixed(6) }}</p><label>地点名称<input v-model="mapPickTitle" required autofocus placeholder="例如：临时观景点" /></label><label>地址或备注（可选）<input v-model="mapPickAddress" placeholder="补充位置说明" /></label><label>加入日期<select v-model="mapPickDayID"><option v-for="day in tripDocument?.days || []" :key="day.id" :value="day.id">{{ day.date }}{{ day.title ? ' · ' + day.title : '' }}</option></select></label><div class="modal-actions"><button type="button" @click="cancelMapPick">取消</button><button type="button" class="primary" :disabled="actionLoading || !mapPickTitle.trim()" @click="saveMapPick">{{ actionLoading ? '保存中…' : '保存规划点' }}</button></div></section></div>
      <div v-if="newTripOpen" class="modal-backdrop" @click.self="newTripOpen = false"><section class="modal-panel" role="dialog" aria-modal="true" aria-labelledby="new-trip-title"><button class="modal-close" aria-label="关闭" @click="newTripOpen = false">×</button><p class="eyebrow">NEW JOURNEY</p><h2 id="new-trip-title">新建旅行规划</h2><form @submit.prevent="createTrip"><label>规划名称<input v-model="newTitle" maxlength="120" required /></label><div class="form-grid"><label>开始日期<input v-model="newStartDate" type="date" required /></label><label>结束日期<input v-model="newEndDate" type="date" required /></label></div><label>时区<input v-model="newTimezone" placeholder="Asia/Shanghai" required /></label><label>总体说明（Markdown）<textarea v-model="newDescription" rows="5" placeholder="写下这次旅行的总体说明"></textarea></label><div class="modal-actions"><button type="button" @click="newTripOpen = false">取消</button><button class="primary" type="submit" :disabled="actionLoading">创建草稿</button></div></form></section></div>
      <div v-if="settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false"><section class="modal-panel settings-panel" role="dialog" aria-modal="true" aria-labelledby="settings-title"><button class="modal-close" aria-label="关闭" @click="settingsOpen = false">×</button><p class="eyebrow">JOURNEYIN SETTINGS</p><h2 id="settings-title">设置</h2><p class="settings-intro">当前主题：{{ themeLabel }}。Key 配置保存到 SQLite，服务端 Key 不会回显。</p><section class="settings-section"><h3>外观</h3><p class="settings-label">主题：{{ themeLabel }}</p><div class="theme-options"><button type="button" :class="{ selected: theme === 'system' }" @click="setTheme('system')">跟随系统</button><button type="button" :class="{ selected: theme === 'light' }" @click="setTheme('light')">浅色</button><button type="button" :class="{ selected: theme === 'dark' }" @click="setTheme('dark')">深色</button></div></section><section class="settings-section"><h3>服务端连接</h3><label>当前服务地址<input v-model="serverURL" readonly /></label><label>兼容 REST API Token<input v-model="authTokenInput" type="password" placeholder="仅用于兼容旧客户端，可留空" autocomplete="off" /></label><div class="modal-actions"><button type="button" @click="logout">清除令牌</button><button type="button" class="primary" @click="saveAuth">保存令牌</button></div><p v-if="settingsMessage" class="settings-message">{{ settingsMessage }}</p></section><section class="settings-section"><h3>百度地图</h3><p class="key-status">浏览器端 Key：<strong>{{ keyConfigured ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.baidu?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>百度浏览器端 Key<input v-model="baiduBrowserKeyInput" type="password" :placeholder="settingsData?.map?.baidu?.browser_key_configured ? '已配置，输入新 Key 可替换' : '用于 JSAPI 4.0/BMap 网页地图'" autocomplete="off" /></label><label>百度服务端 Key<input v-model="baiduServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><p class="key-help">浏览器端 Key 用于地图底图；服务端 Key 用于 POI 搜索、地理编码、路线和天气。请确认当前访问 host 在百度控制台白名单内。</p><a href="https://lbsyun.baidu.com/apiconsole/key" target="_blank" rel="noopener noreferrer">申请/管理百度地图 Key ↗</a></section><section class="settings-section"><h3>高德地图</h3><p class="key-status">JS Key：<strong>{{ settingsData?.map?.amap?.js_key_configured ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.amap?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>高德 JS Key<input v-model="amapJSKeyInput" type="password" placeholder="用于高德 Web 地图" autocomplete="off" /></label><label>高德服务端 Key<input v-model="amapServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><a href="https://console.amap.com/dev/key/app" target="_blank" rel="noopener noreferrer">申请/管理高德 Key ↗</a><p class="key-help">保存后，规划点会优先使用已经保存的坐标，不会因为重新绘制地图重复查询。</p><div class="modal-actions"><button type="button" class="primary" :disabled="settingsSaving" @click="saveMapKeys">{{ settingsSaving ? '保存中…' : '保存地图 Key 到数据库' }}</button></div></section><section class="settings-section"><h3>地点检索</h3><label>优先 Provider<select v-model="poiProviderPriority"><option value="amap">高德优先</option><option value="baidu">百度优先</option></select></label><p class="key-help">当前策略会先查询本地地点目录；未命中后使用所选 Provider，Provider 不可用时自动尝试另一家。新搜索结果只保留 7 天。</p><p class="key-status">本地地点记录：<strong>{{ localDirectoryCount }}</strong> 条</p><div class="modal-actions"><button type="button" @click="savePOIPreferences">保存检索优先级</button><button type="button" @click="clearLocalDirectory">清除本地记录</button></div></section><section class="settings-section"><h3>MCP</h3><p>MCP 地址：{{ capabilities?.mcp?.http_endpoint || '/mcp' }}</p><p class="key-help">Docker 远程部署时设置 JOURNEYIN_MCP_TOKEN；本地 localhost 调试可不设置。</p></section></section></div>
      <div v-if="authOpen" class="modal-backdrop" @click.self="authOpen = false"><section class="modal-panel auth-panel" role="dialog" aria-modal="true" aria-labelledby="auth-title"><IonIcon class="auth-icon" :icon="logInOutline" /><h2 id="auth-title">登录 JourneyIn</h2><p>请输入 Docker 服务配置的账号和密码。登录成功后会在当前浏览器保存一个 HttpOnly 会话。</p><form class="auth-form" @submit.prevent="login"><label>账号<input v-model="loginUsername" type="text" autofocus autocomplete="username" /></label><label>密码<input v-model="loginPassword" type="password" autocomplete="current-password" /></label><p v-if="loginMessage" class="auth-error">{{ loginMessage }}</p><div class="modal-actions"><button type="button" @click="authOpen = false">稍后</button><button type="submit" class="primary" :disabled="loginLoading">{{ loginLoading ? '登录中…' : '登录' }}</button></div></form></section></div>
    </IonPage>
  </IonApp>
</template>
