<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  IonApp, IonBadge, IonButton, IonChip, IonContent, IonHeader, IonIcon,
  IonPage, IonRefresher, IonRefresherContent, IonTitle, IonToolbar,
} from '@ionic/vue'
import BMapLoader from '@baidumap/jsapi-loader'
import { addOutline, closeOutline, cloudOfflineOutline, linkOutline, logInOutline, mapOutline, menuOutline, navigateOutline, searchOutline, settingsOutline, sunnyOutline } from 'ionicons/icons'

type Theme = 'system' | 'light' | 'dark'
type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord>; source?: string; provider_refs?: Record<string, unknown> }
type LinkData = { id?: string; title: string; url: string; kind?: string }
type Stop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown>; children?: SubStop[] }
type SubStop = { id: string; sequence: number; kind?: string; title: string; address?: string; location?: LocationData; time_window?: { arrival?: string; departure?: string }; description_markdown?: string; links?: LinkData[]; weather?: Record<string, unknown> }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; mode?: string; snapshots?: Array<{ provider?: string; coordinate_system?: string; geometry?: Array<[number, number]> | Array<Coord>; distance_m?: number; duration_s?: number; fetched_at?: string }> }
type Day = { id: string; date: string; title?: string; notes_markdown?: string; stops: Stop[]; legs?: Leg[] }
type TripDocument = { title: string; timezone: string; description_markdown?: string; links?: LinkData[]; days: Day[] }
type TripSummary = { id: string; title: string; status: string; start_date: string; end_date: string; timezone: string; revision: number; days?: number; stops?: number }
type Capabilities = { map_providers?: { baidu?: { browser_key_configured?: boolean; browser_key?: string }; amap?: { browser_key_configured?: boolean } }; mcp?: { http_endpoint?: string } }
type KeySettings = { map?: { baidu?: { browser_key_configured?: boolean; server_key_configured?: boolean }; amap?: { js_key_configured?: boolean; server_key_configured?: boolean } } }
type PlaceCandidate = { id?: string; name: string; address?: string; location: Coord & { crs?: string }; provider?: string }
type TravelMode = 'driving' | 'walking' | 'cycling' | 'transit'

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
const selectedDay = ref<number | 'all'>('all')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const mapError = ref('')
const mapReady = ref(false)
const mapContainer = ref<HTMLElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const shareURL = ref('')
const actionLoading = ref(false)
const settingsOpen = ref(false)
const newTripOpen = ref(false)
const authOpen = ref(false)
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
const settingsSaving = ref(false)
const panelOpen = ref(localStorage.getItem('journeyin.panelOpen') !== 'false')
const mapType = ref<'normal' | 'satellite'>((localStorage.getItem('journeyin.mapType') as 'normal' | 'satellite') || 'normal')
const panelMode = ref<'journey' | 'search'>('journey')
const searchQuery = ref('')
const searchRegion = ref('')
const searchCategory = ref<'all' | '旅游景点' | '酒店' | '餐饮'>('all')
const searchResults = ref<PlaceCandidate[]>([])
const searchLoading = ref(false)
const searchMessage = ref('')
const planningMode = ref<TravelMode>('walking')
const planningLoading = ref(false)
let mapInstance: any = null
let mapAPI: any = null
let mapScriptPromise: Promise<void> | null = null
let mediaQuery: MediaQueryList | null = null
let mapReadyTimer: number | null = null
let loadedMapKey = ''

const key = computed(() => capabilities.value?.map_providers?.baidu?.browser_key || '')
const keyConfigured = computed(() => Boolean(key.value))
const visibleDays = computed(() => {
  if (!tripDocument.value) return []
  return selectedDay.value === 'all' ? tripDocument.value.days : tripDocument.value.days.filter((_, index) => index + 1 === selectedDay.value)
})
const visibleStops = computed(() => visibleDays.value.flatMap(day => day.stops || []).sort((a, b) => a.sequence - b.sequence))
const plannableDays = computed(() => visibleDays.value.filter(day => (day.stops || []).length >= 2))
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
  return fetch(input, { ...init, headers }).then(response => {
    if (response.status === 401) { authOpen.value = true; settingsMessage.value = '当前服务需要登录令牌' }
    return response
  })
}

async function loadTrips() {
  loading.value = true
  error.value = ''
  try {
    const [tripResponse, capabilityResponse] = await Promise.all([apiFetch('/api/v1/trips'), apiFetch('/api/v1/capabilities')])
    if (tripResponse.status === 401) return
    if (!tripResponse.ok) throw new Error('无法读取旅行规划')
    trips.value = ((await tripResponse.json()) as { items?: TripSummary[] }).items || []
    capabilities.value = capabilityResponse.ok ? await capabilityResponse.json() as Capabilities : null
    if (trips.value[0]) await loadDetail(trips.value[0])
    else { selected.value = null; tripDocument.value = null; renderMap() }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '网络请求失败'
  } finally {
    loading.value = false
  }
}

async function loadDetail(trip: TripSummary) {
  selected.value = trip
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

function pointFor(stop: Stop | SubStop): (Coord & { crs: string }) | null {
  const coordinates = stop.location?.coordinates
  if (!coordinates) return null
  const preferred = coordinates.bd09ll ? 'bd09ll' : stop.location?.preferred && coordinates[stop.location.preferred] ? stop.location.preferred : Object.keys(coordinates)[0]
  const point = preferred ? coordinates[preferred] : null
  if (!point || !Number.isFinite(point.lat) || !Number.isFinite(point.lng)) return null
  return { ...point, crs: preferred }
}
function routePoint(value: [number, number] | Coord, crs: string) {
  if (Array.isArray(value)) return { lng: value[0], lat: value[1], crs }
  return { lng: value.lng, lat: value.lat, crs: (value as Coord & { crs?: string }).crs || crs }
}
function chooseSnapshot(leg: Leg) { return (leg.snapshots || []).find(snapshot => snapshot.provider === 'baidu' && snapshot.coordinate_system === 'bd09ll' && snapshot.geometry && snapshot.geometry.length > 1) || null }
function selectStop(stop: Stop) { selectedStopId.value = stop.id; selectedSubStopId.value = ''; void renderMap(); if (window.matchMedia('(max-width: 900px)').matches) { panelOpen.value = false; localStorage.setItem('journeyin.panelOpen', 'false') }; if (mapInstance && mapAPI) { const point = pointFor(stop); if (point) mapInstance.panTo(new mapAPI.Point(point.lng, point.lat)) } }
function selectSubStop(child: SubStop, parent: Stop) { selectedStopId.value = parent.id; selectedSubStopId.value = child.id; void renderMap(); if (window.matchMedia('(max-width: 900px)').matches) { panelOpen.value = false; localStorage.setItem('journeyin.panelOpen', 'false') }; if (mapInstance && mapAPI) { const point = pointFor(child); if (point) mapInstance.panTo(new mapAPI.Point(point.lng, point.lat)) } }
function openChildSearch(parent: Stop) { selectedStopId.value = parent.id; selectedSubStopId.value = ''; searchParentStopId.value = parent.id; panelOpen.value = true; panelMode.value = 'search'; searchMessage.value = '为“' + parent.title + '”添加子规划点' }
function beginEditDescription() { descriptionDraft.value = selectedTarget.value?.description_markdown || ''; descriptionEditing.value = true }
function cancelEditDescription() { descriptionEditing.value = false; descriptionDraft.value = '' }
async function saveDescription() {
  if (!selected.value || !tripDocument.value || !selectedTarget.value) return
  const target = selectedTarget.value; const previous = target.description_markdown || ''; target.description_markdown = descriptionDraft.value.trim(); descriptionSaving.value = true; error.value = ''
  const parentID = selectedStop.value?.id || ''; const childID = selectedSubStop.value?.id || ''
  try {
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id), { method: 'PUT', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify(tripDocument.value) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '保存地点说明失败')
    applyTripPayload(payload); selectedStopId.value = parentID; selectedSubStopId.value = childID; descriptionEditing.value = false; descriptionDraft.value = ''
  } catch (cause) { target.description_markdown = previous; error.value = cause instanceof Error ? cause.message : '保存地点说明失败' } finally { descriptionSaving.value = false }
}

function resetBaiduMapSDK() {
  try { mapInstance?.destroy?.() } catch { /* SDK cleanup is best effort */ }
  mapInstance = null
  mapAPI = null
  mapReady.value = false
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
  try {
    await loadBaiduMap()
    if (!mapAPI || typeof mapAPI.Map !== 'function' || !mapContainer.value) throw new Error('百度 JSAPI 未提供可用的 Map 构造器；请检查浏览器端 AK、服务权限、域名白名单和当前浏览器环境')
    if (!mapInstance) {
      mapReady.value = false
      mapInstance = new mapAPI.Map(mapContainer.value, { enableIconClick: false, fixCenterWhenResize: true })
      mapInstance.enableScrollWheelZoom()
      mapInstance.addEventListener?.('tilesloaded', () => { mapReady.value = true; mapError.value = '' })
      if (mapReadyTimer !== null) window.clearTimeout(mapReadyTimer)
      mapReadyTimer = window.setTimeout(() => {
        if (!mapReady.value) mapError.value = '百度 JSAPI 已初始化，但底图未加载；请检查浏览器端 AK、JSAPI 服务、域名白名单和网络连接'
      }, 8000)
    }
    mapInstance.clearOverlays()
    const points: any[] = []
    for (const stop of visibleStops.value) {
      const point = pointFor(stop)
      if (!point || point.crs !== 'bd09ll') continue
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      points.push(mapPoint)
      const marker = new mapAPI.Marker(mapPoint)
      marker.__journeyinStopId = stop.id
      marker.addEventListener?.('click', () => selectStop(stop))
      mapInstance.addOverlay(marker)
    }
    if (selectedStop.value?.children?.length) for (const child of selectedStop.value.children) {
      const point = pointFor(child)
      if (!point || point.crs !== 'bd09ll') continue
      const mapPoint = new mapAPI.Point(point.lng, point.lat)
      const marker = new mapAPI.Marker(mapPoint)
      marker.__journeyinSubStopId = child.id
      marker.addEventListener?.('click', () => selectSubStop(child, selectedStop.value!))
      mapInstance.addOverlay(marker)
    }
    for (const day of visibleDays.value) for (const leg of day.legs || []) {
      const snapshot = chooseSnapshot(leg)
      if (!snapshot?.geometry) continue
      const line = snapshot.geometry.map(value => { const point = routePoint(value, snapshot.coordinate_system || 'bd09ll'); return new mapAPI.Point(point.lng, point.lat) })
      if (line.length > 1) mapInstance.addOverlay(new mapAPI.Polyline(line, { strokeColor: '#006874', strokeWeight: 5, strokeOpacity: .82 }))
    }
    if (points.length) mapInstance.setViewport(points)
    else mapInstance.centerAndZoom('中国', 5)
    applyMapType()
    mapError.value = ''
  } catch (cause) { mapReady.value = false; mapError.value = cause instanceof Error ? cause.message : '地图初始化失败' }
}

function togglePanel() { panelOpen.value = !panelOpen.value; localStorage.setItem('journeyin.panelOpen', String(panelOpen.value)) }
function applyMapType() {
  if (!mapInstance || !mapAPI || typeof mapInstance.setMapType !== 'function') return
  const type = mapType.value === 'satellite' ? (window as any).BMAP_SATELLITE_MAP : (window as any).BMAP_NORMAL_MAP
  if (type) mapInstance.setMapType(type)
}
function toggleMapType() { mapType.value = mapType.value === 'normal' ? 'satellite' : 'normal'; localStorage.setItem('journeyin.mapType', mapType.value); applyMapType() }
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
    const response = await apiFetch('/api/v1/maps/pois/search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider: 'baidu', query: searchQuery.value.trim(), region: searchRegion.value.trim(), category: searchCategory.value === 'all' ? undefined : searchCategory.value, page: 1, page_size: 10 }) })
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
    const response = await apiFetch(endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ stop: { title: candidate.name, address: candidate.address, location: { preferred: 'bd09ll', coordinates: { bd09ll: { lat: location.lat, lng: location.lng, crs: 'bd09ll' } }, source: 'baidu-place-search', provider_refs: candidate.id ? { baidu_uid: candidate.id } : {}, geocoded_at: new Date().toISOString(), precision: 'poi' } } }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '添加规划点失败')
    applyTripPayload(payload)
    if (parentID) { selectedStopId.value = parentID; selectedSubStopId.value = ''; searchMessage.value = '已添加“' + candidate.name + '”为子规划点'; } else { searchMessage.value = '已添加“' + candidate.name + '”，路线尚未生成' }
    searchParentStopId.value = ''; panelMode.value = 'journey'; searchResults.value = []
  } catch (cause) { searchMessage.value = cause instanceof Error ? cause.message : '添加规划点失败' } finally { actionLoading.value = false }
}

async function planRoutes() {
  if (!selected.value || !tripDocument.value) { error.value = '请先选择一条旅行规划'; return }
  if (!plannableDays.value.length) { error.value = '同一天至少添加两个带坐标的规划点后才能生成路线'; return }
  planningLoading.value = true; error.value = ''
  try {
    const day = selectedDay.value === 'all' ? undefined : tripDocument.value.days[selectedDay.value - 1]?.id
    const response = await apiFetch('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/plan', { method: 'POST', headers: { 'Content-Type': 'application/json', 'If-Match': 'revision-' + selected.value.revision }, body: JSON.stringify({ provider: 'baidu', mode: planningMode.value, day_id: day }) })
    const payload = await response.json() as { document?: TripDocument; revision?: number; stops?: number; days?: number; error?: { message?: string } }
    if (!response.ok) throw new Error(payload.error?.message || '路线生成失败')
    applyTripPayload(payload)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '路线生成失败' } finally { planningLoading.value = false }
}

async function importTrip(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  actionLoading.value = true
  try {
    const response = await apiFetch('/api/v1/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: await file.text() })
    if (!response.ok) { const payload = await response.json() as { error?: { message?: string } }; throw new Error(payload.error?.message || '导入失败') }
    await loadTrips()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导入失败' } finally { actionLoading.value = false; input.value = '' }
}
function downloadTrip() { if (selected.value) window.open('/api/v1/trips/' + encodeURIComponent(selected.value.id) + '/export.json', '_blank', 'noopener,noreferrer') }
async function createShare() {
  if (!selected.value) return
  actionLoading.value = true; shareURL.value = ''
  try {
    const response = await apiFetch('/api/v1/shares', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ trip_id: selected.value.id }) })
    const payload = await response.json() as { url?: string; error?: { message?: string } }
    if (!response.ok || !payload.url) throw new Error(payload.error?.message || '分享链接创建失败')
    shareURL.value = payload.url
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '分享链接创建失败' } finally { actionLoading.value = false }
}
function safeURL(raw: string) { try { const parsed = new URL(raw); return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : '#' } catch { return '#' } }
function escapeHTML(value: string) { return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('\\\"', '&quot;') }
function renderMarkdown(source: string) {
  const escaped = escapeHTML(source)
  const formatted = escaped.replace(/^### (.+)$/gm, '<h4>$1</h4>').replace(/^## (.+)$/gm, '<h3>$1</h3>').replace(/^# (.+)$/gm, '<h2>$1</h2>').replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>').replace(/\*([^*]+?)\*/g, '<em>$1</em>').replace(/\[([^\]]+)\]\((https?:\/\/[^) ]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
  return formatted.split(/\n{2,}/).map(block => block.startsWith('<h') ? block : '<p>' + block.replaceAll('\n', '<br>') + '</p>').join('')
}
async function openNavigation(provider: 'baidu' | 'amap') {
  const stop = selectedTarget.value; const point = stop && pointFor(stop)
  if (!stop || !point) { error.value = '该规划点没有可靠坐标，无法生成导航链接'; return }
  try { const response = await apiFetch('/api/v1/maps/navigation', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider, target: { name: stop.title, address: stop.address, location: point }, mode: 'walking', platform: /Android/i.test(navigator.userAgent) ? 'android' : 'web' }) }); const payload = await response.json() as { url?: string; error?: { message?: string } }; if (!response.ok || !payload.url) throw new Error(payload.error?.message || '导航链接生成失败'); window.open(payload.url, '_blank', 'noopener,noreferrer') } catch (cause) { error.value = cause instanceof Error ? cause.message : '导航失败' }
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
function refresh(event: CustomEvent) { loadTrips().finally(() => event.detail.complete()) }
function closeDetail() { selectedStopId.value = ''; selectedSubStopId.value = ''; void renderMap() }
async function openSettings() {
  settingsOpen.value = true
  try {
    const response = await apiFetch('/api/v1/settings')
    if (!response.ok) throw new Error('无法读取设置')
    settingsData.value = await response.json() as KeySettings
    baiduBrowserKeyInput.value = ''
    baiduServerKeyInput.value = ''
    amapJSKeyInput.value = ''
    amapServerKeyInput.value = ''
  } catch (cause) { settingsMessage.value = cause instanceof Error ? cause.message : '无法读取设置' }
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
function saveAuth() { authTokenInput.value = authTokenInput.value.trim(); if (authTokenInput.value) localStorage.setItem('journeyin.apiToken', authTokenInput.value); else localStorage.removeItem('journeyin.apiToken'); authOpen.value = false; settingsMessage.value = '访问令牌已保存'; loadTrips() }
function logout() { authTokenInput.value = ''; localStorage.removeItem('journeyin.apiToken'); authOpen.value = false; settingsMessage.value = '已清除访问令牌'; loadTrips() }
function applyTheme() { const actual = theme.value === 'system' ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme.value; document.documentElement.dataset.theme = actual; localStorage.setItem('journeyin.theme', theme.value) }
function setTheme(value: Theme) { theme.value = value; applyTheme() }
function systemThemeChanged() { if (theme.value === 'system') applyTheme() }
watch([selectedDay, tripDocument], () => { renderMap() }, { deep: true })
onMounted(() => { applyTheme(); mediaQuery = window.matchMedia('(prefers-color-scheme: dark)'); mediaQuery.addEventListener?.('change', systemThemeChanged); loadTrips() })
onUnmounted(() => mediaQuery?.removeEventListener?.('change', systemThemeChanged))
</script>

<template>
  <IonApp>
    <IonPage class="journey-page">
      <IonHeader translucent>
        <IonToolbar>
          <IonTitle><span class="brand-dot"></span>JourneyIn</IonTitle>
          <div slot="end" class="header-actions">
            <IonButton fill="clear" aria-label="显示或隐藏行程面板" @click="togglePanel"><IonIcon :icon="menuOutline" /></IonButton>
            <IonButton fill="clear" aria-label="打开设置" @click="openSettings()"><IonIcon :icon="settingsOutline" /></IonButton>
            <IonButton fill="clear" aria-label="新建旅行规划" @click="newTripOpen = true"><IonIcon :icon="addOutline" /></IonButton>
          </div>
        </IonToolbar>
      </IonHeader>
      <IonContent :fullscreen="true">
        <IonRefresher slot="fixed" @ionRefresh="refresh"><IonRefresherContent /></IonRefresher>
        <main class="map-shell">
          <div class="map-canvas">
            <div v-if="keyConfigured && tripDocument && !mapError" ref="mapContainer" id="bmap"></div>
            <div v-if="!tripDocument || !keyConfigured || mapError" class="map-fallback">
              <IonIcon :icon="mapOutline" />
              <strong>{{ !tripDocument ? '选择或创建一条旅行规划' : mapError || '百度地图未配置' }}</strong>
              <span>{{ !tripDocument ? '创建或导入 Trip 后显示地图。' : mapError ? '请确认浏览器端 AK、JSAPI 服务、域名白名单和网络连接。当前页面：' + serverURL : '配置浏览器端 Key 后显示真实地图。当前不会伪造道路或路线。' }}</span>
              <IonChip color="warning"><IonIcon :icon="cloudOfflineOutline" /> {{ !tripDocument ? '等待行程' : '降级模式' }}</IonChip>
            </div>
            <div v-if="keyConfigured && tripDocument && !mapError && !mapReady" class="map-loading"><IonIcon :icon="mapOutline" /><span>正在加载百度地图…</span></div>
          </div>
          <div v-if="!selectedStop" class="map-hud">
            <button v-if="!panelOpen" class="panel-open-button" aria-label="显示行程面板" @click="togglePanel"><IonIcon :icon="menuOutline" /> 行程面板</button>
            <button class="map-type-button" :class="{ active: mapType === 'satellite' }" @click="toggleMapType">{{ mapType === 'satellite' ? '标准图' : '卫星图' }}</button>
            <div class="map-status-pill"><IonIcon :icon="keyConfigured && mapReady && !mapError ? sunnyOutline : cloudOfflineOutline" /> {{ !tripDocument ? '等待行程' : !keyConfigured ? '离线数据可用' : mapError ? '百度地图不可用' : mapReady ? '百度地图已连接' : '百度地图加载中' }} · {{ visibleStops.length }} 个规划点</div>
          </div>
          <section v-if="error || shareURL" class="map-notices">
            <div v-if="error" class="global-error">{{ error }}<button aria-label="关闭错误" @click="error = ''">×</button></div>
            <div v-if="shareURL" class="share-banner"><span>只读分享：<a :href="shareURL" target="_blank" rel="noopener noreferrer">{{ shareURL }}</a></span><button aria-label="关闭分享提示" @click="shareURL = ''">×</button></div>
          </section>
          <aside v-if="panelOpen" class="floating-panel plan-panel" aria-label="行程规划面板">
            <div class="panel-head">
              <div><p class="eyebrow">JOURNEYIN</p><h2>{{ selected?.title || '旅行规划' }}</h2><p class="panel-subtitle">{{ selected ? selected.start_date + ' — ' + selected.end_date : '本地行程工作区' }}</p></div>
              <button class="panel-close" aria-label="隐藏行程面板" @click="togglePanel">×</button>
            </div>
            <div class="panel-tabs">
              <button :class="{ selected: panelMode === 'journey' }" @click="panelMode = 'journey'">行程</button>
              <button :class="{ selected: panelMode === 'search' }" :disabled="!selected" @click="searchParentStopId = ''; panelMode = 'search'"><IonIcon :icon="searchOutline" /> 添加地点</button>
            </div>
            <div class="panel-scroll">
              <template v-if="panelMode === 'journey'">
                <div class="section-heading"><div><p class="eyebrow">YOUR JOURNEYS</p><h3>旅行规划</h3></div><IonBadge color="primary">{{ trips.length }}</IonBadge></div>
                <p v-if="loading" class="muted">正在加载…</p>
                <div v-else-if="!trips.length" class="empty"><p>还没有旅行规划。</p><IonButton size="small" @click="newTripOpen = true"><IonIcon slot="start" :icon="addOutline" /> 新建规划</IonButton><p class="empty-hint">也可以导入已有 Trip JSON。</p></div>
                <div v-else class="trip-cards">
                  <button v-for="trip in trips" :key="trip.id" class="trip-card" :class="{ active: selected?.id === trip.id }" @click="loadDetail(trip)">
                    <span><b>{{ trip.title }}</b><small>{{ trip.start_date }} — {{ trip.end_date }}</small><small>{{ trip.days ?? '—' }} 天 · {{ trip.stops ?? '—' }} 个规划点</small></span><IonChip color="light">v{{ trip.revision }}</IonChip>
                  </button>
                </div>
                <template v-if="selected && tripDocument">
                  <div class="panel-section">
                    <div class="section-heading compact"><div><p class="eyebrow">ITINERARY</p><h3>规划点</h3></div><span class="count-label">{{ visibleStops.length }}</span></div>
                    <div class="day-tabs"><button :class="{ selected: selectedDay === 'all' }" @click="selectedDay = 'all'">全程</button><button v-for="(_, index) in tripDocument.days" :key="index" :class="{ selected: selectedDay === index + 1 }" @click="selectedDay = index + 1">D{{ index + 1 }} · {{ tripDocument.days[index].date }}</button></div>
                    <div class="plan-controls"><label>路线方式<select v-model="planningMode"><option value="walking">步行</option><option value="driving">驾车</option><option value="cycling">骑行</option><option value="transit">公交</option></select></label><IonButton size="small" :disabled="planningLoading || !plannableDays.length" @click="planRoutes"><IonIcon slot="start" :icon="navigateOutline" /> {{ planningLoading ? '规划中…' : '生成路线' }}</IonButton></div>
                    <p v-if="!plannableDays.length" class="hint">同一天添加至少两个地点后，点击“生成路线”。</p>
                    <p v-else-if="visibleDays.some(day => day.legs?.some(leg => leg.snapshots?.length))" class="route-ready"><IonIcon :icon="navigateOutline" /> 已有路线快照；重复点击会优先使用缓存。</p>
                    <div v-if="visibleStops.length" class="stop-list"><button v-for="stop in visibleStops" :key="stop.id" class="stop-row" :class="{ selected: selectedStopId === stop.id }" @click="selectStop(stop)"><span class="stop-number">{{ stop.sequence }}</span><span><b>{{ stop.title }}</b><small>{{ stopDate(stop) }} · {{ stop.address || '地址已保存' }}</small></span><span class="stop-arrow">›</span></button></div>
                    <p v-else class="empty compact-empty">当前日期还没有规划点。</p>
                    <button class="add-place-cta" @click="searchParentStopId = ''; panelMode = 'search'"><IonIcon :icon="searchOutline" /> 搜索并添加规划点</button>
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
                <p class="search-help">只在点击搜索时调用百度 POI 接口。选择结果后保存名称、地址、坐标、CRS 和百度 UID；景点类型会使用旅游景点分类，无 POI 结果时降级到缓存地理编码。</p>
                <p v-if="searchMessage" class="inline-message">{{ searchMessage }}</p>
                <div class="search-results"><article v-for="(result, index) in searchResults" :key="result.id || result.name + index" class="search-result"><div><h4>{{ result.name }}</h4><p>{{ result.address || '地址待补充' }}</p><small v-if="result.location">BD-09LL · {{ result.location.lat.toFixed(6) }}, {{ result.location.lng.toFixed(6) }}</small></div><IonButton size="small" @click="addPlaceToTrip(result)">{{ searchParentStopId ? '添加子点' : '添加' }}</IonButton></article></div>
              </template>
            </div>
          </aside>
          <aside v-if="selectedStop" class="details-drawer">
            <button class="close-button" aria-label="关闭详情" @click="closeDetail"><IonIcon :icon="closeOutline" /></button>
            <p class="eyebrow">{{ selectedSubStop ? 'SUB-STOP ' + selectedSubStop.sequence : 'STOP ' + selectedStop.sequence }}</p>
            <button v-if="selectedSubStop" class="detail-parent" @click="selectedSubStopId = ''">返回主规划点：{{ selectedStop.title }}</button>
            <h2>{{ selectedTarget?.title }}</h2>
            <p class="address">{{ selectedTarget?.address || '地址待解析' }}</p>
            <p class="stop-date">行程日期：{{ stopDate(selectedTarget || selectedStop) }}</p>
            <div class="saved-location"><span>坐标已保存</span><small>{{ pointFor(selectedTarget || selectedStop)?.crs || '未知 CRS' }} · {{ pointFor(selectedTarget || selectedStop)?.lat.toFixed(6) }}, {{ pointFor(selectedTarget || selectedStop)?.lng.toFixed(6) }}</small></div>
            <div class="weather"><IonIcon :icon="sunnyOutline" /><span>{{ weatherText(selectedTarget || selectedStop) }}<small v-if="weatherUpdatedAt(selectedTarget || selectedStop)">更新于 {{ weatherUpdatedAt(selectedTarget || selectedStop) }}</small></span></div>
            <IonButton size="small" fill="outline" :disabled="weatherLoading" @click="refreshWeather">{{ weatherLoading ? '天气查询中…' : selectedTarget?.weather ? '刷新天气' : '获取天气' }}</IonButton>
            <div v-if="!selectedSubStop" class="children-panel"><div class="section-heading compact"><div><p class="eyebrow">CHILD POINTS</p><h3>子规划点</h3></div><span class="count-label">{{ selectedStop.children?.length || 0 }}</span></div><p class="children-help">进入主规划点详情后，子规划点才会显示在地图上。</p><div v-if="selectedStop.children?.length" class="child-list"><button v-for="child in selectedStop.children" :key="child.id" class="child-row" :class="{ selected: selectedSubStopId === child.id }" @click="selectSubStop(child, selectedStop)"><span class="child-number">{{ child.sequence }}</span><span><b>{{ child.title }}</b><small>{{ stopDate(child) }} · {{ child.address || '地址已保存' }}</small></span><span>›</span></button></div><button class="add-place-cta" @click="openChildSearch(selectedStop)"><IonIcon :icon="searchOutline" /> 添加子规划点</button></div>
            <div class="description-section"><div class="description-header"><h3>地点说明</h3><button v-if="!descriptionEditing" class="text-button" @click="beginEditDescription">编辑地点说明</button></div><template v-if="descriptionEditing"><textarea v-model="descriptionDraft" class="description-editor" rows="7" placeholder="补充这个地点的门票、开放时间、行程备注等信息"></textarea><div class="description-actions"><button class="text-button" @click="cancelEditDescription">取消</button><button class="primary-text-button" :disabled="descriptionSaving" @click="saveDescription">{{ descriptionSaving ? '保存中…' : '保存说明' }}</button></div></template><template v-else><div v-if="selectedTarget?.description_markdown" class="markdown" v-html="renderMarkdown(selectedTarget.description_markdown)"></div><p v-else class="muted">暂无地点说明，点击“编辑地点说明”添加。</p></template></div>
            <div class="links" v-if="selectedTarget?.links?.length"><a v-for="link in selectedTarget.links" :key="link.id || link.url" :href="safeURL(link.url)" target="_blank" rel="noopener noreferrer"><IonIcon :icon="linkOutline" /> {{ link.title }}</a></div>
            <div class="nav-actions"><IonButton size="small" @click="openNavigation('baidu')"><IonIcon slot="start" :icon="navigateOutline" /> 百度导航</IonButton><IonButton size="small" fill="outline" @click="openNavigation('amap')"><IonIcon slot="start" :icon="navigateOutline" /> 高德导航</IonButton></div>
          </aside>
        </main>
      </IonContent>
      <div v-if="newTripOpen" class="modal-backdrop" @click.self="newTripOpen = false"><section class="modal-panel" role="dialog" aria-modal="true" aria-labelledby="new-trip-title"><button class="modal-close" aria-label="关闭" @click="newTripOpen = false">×</button><p class="eyebrow">NEW JOURNEY</p><h2 id="new-trip-title">新建旅行规划</h2><form @submit.prevent="createTrip"><label>规划名称<input v-model="newTitle" maxlength="120" required /></label><div class="form-grid"><label>开始日期<input v-model="newStartDate" type="date" required /></label><label>结束日期<input v-model="newEndDate" type="date" required /></label></div><label>时区<input v-model="newTimezone" placeholder="Asia/Shanghai" required /></label><label>总体说明（Markdown）<textarea v-model="newDescription" rows="5" placeholder="写下这次旅行的总体说明"></textarea></label><div class="modal-actions"><button type="button" @click="newTripOpen = false">取消</button><button class="primary" type="submit" :disabled="actionLoading">创建草稿</button></div></form></section></div>
      <div v-if="settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false"><section class="modal-panel settings-panel" role="dialog" aria-modal="true" aria-labelledby="settings-title"><button class="modal-close" aria-label="关闭" @click="settingsOpen = false">×</button><p class="eyebrow">JOURNEYIN SETTINGS</p><h2 id="settings-title">设置</h2><p class="settings-intro">当前主题：{{ themeLabel }}。Key 配置保存到 SQLite，服务端 Key 不会回显。</p><section class="settings-section"><h3>外观</h3><p class="settings-label">主题：{{ themeLabel }}</p><div class="theme-options"><button type="button" :class="{ selected: theme === 'system' }" @click="setTheme('system')">跟随系统</button><button type="button" :class="{ selected: theme === 'light' }" @click="setTheme('light')">浅色</button><button type="button" :class="{ selected: theme === 'dark' }" @click="setTheme('dark')">深色</button></div></section><section class="settings-section"><h3>服务端连接</h3><label>当前服务地址<input v-model="serverURL" readonly /></label><label>REST/MCP 访问令牌<input v-model="authTokenInput" type="password" placeholder="Docker 开启认证时填写" autocomplete="off" /></label><div class="modal-actions"><button type="button" @click="logout">清除令牌</button><button type="button" class="primary" @click="saveAuth">保存令牌</button></div><p v-if="settingsMessage" class="settings-message">{{ settingsMessage }}</p></section><section class="settings-section"><h3>百度地图</h3><p class="key-status">浏览器端 Key：<strong>{{ keyConfigured ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.baidu?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>百度浏览器端 Key<input v-model="baiduBrowserKeyInput" type="password" :placeholder="settingsData?.map?.baidu?.browser_key_configured ? '已配置，输入新 Key 可替换' : '用于 JSAPI 4.0/BMap 网页地图'" autocomplete="off" /></label><label>百度服务端 Key<input v-model="baiduServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><p class="key-help">浏览器端 Key 用于地图底图；服务端 Key 用于 POI 搜索、地理编码、路线和天气。请确认当前访问 host 在百度控制台白名单内。</p><a href="https://lbsyun.baidu.com/apiconsole/key" target="_blank" rel="noopener noreferrer">申请/管理百度地图 Key ↗</a></section><section class="settings-section"><h3>高德地图</h3><p class="key-status">JS Key：<strong>{{ settingsData?.map?.amap?.js_key_configured ? '已配置' : '未配置' }}</strong> · 服务端 Key：<strong>{{ settingsData?.map?.amap?.server_key_configured ? '已配置' : '未配置' }}</strong></p><label>高德 JS Key<input v-model="amapJSKeyInput" type="password" placeholder="用于高德 Web 地图" autocomplete="off" /></label><label>高德服务端 Key<input v-model="amapServerKeyInput" type="password" placeholder="已配置时输入新 Key 可替换；留空保持当前值" autocomplete="off" /></label><a href="https://console.amap.com/dev/key/app" target="_blank" rel="noopener noreferrer">申请/管理高德 Key ↗</a><p class="key-help">保存后，规划点会优先使用已经保存的坐标，不会因为重新绘制地图重复查询。</p><div class="modal-actions"><button type="button" class="primary" :disabled="settingsSaving" @click="saveMapKeys">{{ settingsSaving ? '保存中…' : '保存地图 Key 到数据库' }}</button></div></section><section class="settings-section"><h3>MCP</h3><p>MCP 地址：{{ capabilities?.mcp?.http_endpoint || '/mcp' }}</p><p class="key-help">Docker 远程部署时设置 JOURNEYIN_MCP_TOKEN；本地 localhost 调试可不设置。</p></section></section></div>
      <div v-if="authOpen" class="modal-backdrop" @click.self="authOpen = false"><section class="modal-panel auth-panel" role="dialog" aria-modal="true" aria-labelledby="auth-title"><IonIcon class="auth-icon" :icon="logInOutline" /><h2 id="auth-title">连接到 JourneyIn</h2><p>当前服务启用了访问认证，请输入服务端访问令牌。令牌只保存在本机浏览器，不会写入旅行规划 JSON。</p><label>访问令牌<input v-model="authTokenInput" type="password" autofocus autocomplete="off" /></label><div class="modal-actions"><button type="button" @click="authOpen = false">稍后</button><button type="button" class="primary" @click="saveAuth">登录并重试</button></div></section></div>
    </IonPage>
  </IonApp>
</template>
