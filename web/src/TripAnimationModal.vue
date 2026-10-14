<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { selectPosterRoutes, type PosterRouteLeg } from './routeTopologyLayout'

type Coord = { lat: number; lng: number; crs?: string }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord & { crs?: string }> }
type LinkData = { id?: string; title: string; url: string }
type Stop = {
  id: string
  sequence: number
  title: string
  kind?: string
  address?: string
  location?: LocationData
  time_window?: { arrival?: string; departure?: string }
  description_markdown?: string
  links?: LinkData[]
  weather?: Record<string, unknown>
}
type Leg = PosterRouteLeg & {
  id: string
  from_stop_id: string
  to_stop_id: string
  mode?: string
  snapshots?: Array<{
    provider?: string
    coordinate_system?: string
    mode?: string
    geometry?: Array<[number, number]> | Array<Coord>
    distance_m?: number
    duration_s?: number
  }>
}
type Day = {
  id: string
  date: string
  title?: string
  notes_markdown?: string
  stops: Stop[]
  legs?: Leg[]
}
type TripDoc = {
  title: string
  date_range?: { start: string; end: string }
  timezone: string
  description_markdown?: string
  map?: { preferred_provider?: string; default_mode?: string }
  days: Day[]
}

const props = withDefaults(defineProps<{
  isOpen: boolean
  trip: TripDoc | null
  currentTheme?: 'light' | 'dark'
}>(), {
  currentTheme: 'light',
})

const emit = defineEmits<{
  (e: 'close'): void
}>()

// --- 配置状态 ---
type AspectRatio = '16:9' | '9:16' | '4:3' | '1:1'
type MapBgStyle = 'dark' | 'light' | 'satellite' | 'transparent'
type VehicleType = 'auto' | 'car' | 'plane' | 'train' | 'bike' | 'walk'
type TrackColorTheme = '#ff6b35' | '#00d2ff' | '#24695c' | '#f43f5e'

const activeDayIndex = ref<number>(-1) // -1 表示全程
const aspectRatio = ref<AspectRatio>('16:9')
const mapBg = ref<MapBgStyle>('dark')
const showPins = ref(true)
const showPopups = ref(true)
const vehicleType = ref<VehicleType>('auto')
const trackColor = ref<TrackColorTheme>('#ff6b35')
const duration = ref(15) // 秒数
const playbackSpeed = ref(1) // 0.5x, 1x, 2x
const isLoop = ref(true)

// --- 播放状态 ---
const isPlaying = ref(true)
const currentTime = ref(0)
const toastMessage = ref('')
const isExportingVideo = ref(false)
const exportProgress = ref(0)
const svgContainerRef = ref<HTMLElement | null>(null)
const hiddenPathRef = ref<SVGPathElement | null>(null)
const scopePillsRef = ref<HTMLElement | null>(null)

let animFrameId: number | null = null
let lastTimestamp: number | null = null

// 根据传入的主题预设底图
watch(() => props.currentTheme, (th) => {
  if (th === 'dark') mapBg.value = 'dark'
  else if (th === 'light') mapBg.value = 'light'
}, { immediate: true })

// 模态打开时重置播放
watch(() => props.isOpen, (open) => {
  if (open) {
    currentTime.value = 0
    isPlaying.value = true
    lastTimestamp = null
    startLoop()
  } else {
    stopLoop()
  }
})

// 画幅宽高定义 (基准矢量坐标系)
const aspectDimensions: Record<AspectRatio, { w: number; h: number }> = {
  '16:9': { w: 840, h: 472 },
  '9:16': { w: 472, h: 840 },
  '4:3': { w: 760, h: 570 },
  '1:1': { w: 600, h: 600 },
}

const canvasWidth = computed(() => aspectDimensions[aspectRatio.value].w)
const canvasHeight = computed(() => aspectDimensions[aspectRatio.value].h)

// 天数列表
const allDays = computed(() => props.trip?.days || [])

// 选定范围内的 Stops 与 Legs
const activeStops = computed<Stop[]>(() => {
  if (!allDays.value.length) return []
  if (activeDayIndex.value === -1) {
    return allDays.value.flatMap(d => d.stops || [])
  }
  const day = allDays.value[activeDayIndex.value]
  return day ? day.stops || [] : []
})

const activeLegs = computed<Leg[]>(() => {
  if (!allDays.value.length) return []
  if (activeDayIndex.value === -1) {
    return allDays.value.flatMap(d => d.legs || [])
  }
  const day = allDays.value[activeDayIndex.value]
  return day ? day.legs || [] : []
})

// 坐标系适配
const coordinateSystems = ['gcj02', 'bd09ll', 'wgs84'] as const
function getStopCoord(stop: Stop, crs: string): [number, number] | null {
  const point = stop.location?.coordinates?.[crs]
  if (!point || (point.crs && point.crs !== crs) || !Number.isFinite(point.lng) || !Number.isFinite(point.lat) || Math.abs(point.lng) > 180 || Math.abs(point.lat) > 90) return null
  return [point.lng, point.lat]
}

// 真实卫星切片计算函数
interface SatelliteTileItem {
  x: number
  y: number
  z: number
  url: string
  px: number
  py: number
  w: number
  h: number
}

function calculateSatelliteTiles(
  minLng: number,
  maxLng: number,
  minLat: number,
  maxLat: number,
  width: number,
  height: number,
  projectFn: ([lng, lat]: [number, number]) => [number, number]
): SatelliteTileItem[] {
  let zoom = 6
  for (let z = 16; z >= 3; z--) {
    const x1 = ((minLng + 180) / 360) * Math.pow(2, z)
    const x2 = ((maxLng + 180) / 360) * Math.pow(2, z)
    const y1 = ((1 - Math.log(Math.tan((maxLat * Math.PI) / 180) + 1 / Math.cos((maxLat * Math.PI) / 180)) / Math.PI) / 2) * Math.pow(2, z)
    const y2 = ((1 - Math.log(Math.tan((minLat * Math.PI) / 180) + 1 / Math.cos((minLat * Math.PI) / 180)) / Math.PI) / 2) * Math.pow(2, z)
    const pixelW = (x2 - x1) * 256
    const pixelH = (y2 - y1) * 256
    if (pixelW <= width * 0.9 && pixelH <= height * 0.9) {
      zoom = z
      break
    }
  }

  function lng2tile(lng: number, z: number) { return Math.floor(((lng + 180) / 360) * Math.pow(2, z)) }
  function lat2tile(lat: number, z: number) { return Math.floor(((1 - Math.log(Math.tan((lat * Math.PI) / 180) + 1 / Math.cos((lat * Math.PI) / 180)) / Math.PI) / 2) * Math.pow(2, z)) }
  function tile2lng(x: number, z: number) { return (x / Math.pow(2, z)) * 360 - 180 }
  function tile2lat(y: number, z: number) {
    const n = Math.PI - (2 * Math.PI * y) / Math.pow(2, z)
    return (180 / Math.PI) * Math.atan(0.5 * (Math.exp(n) - Math.exp(-n)))
  }

  const minTX = Math.max(0, lng2tile(minLng, zoom) - 1)
  const maxTX = Math.min(Math.pow(2, zoom) - 1, lng2tile(maxLng, zoom) + 1)
  const minTY = Math.max(0, lat2tile(maxLat, zoom) - 1)
  const maxTY = Math.min(Math.pow(2, zoom) - 1, lat2tile(minLat, zoom) + 1)

  const tiles: SatelliteTileItem[] = []
  for (let x = minTX; x <= maxTX; x++) {
    for (let y = minTY; y <= maxTY; y++) {
      const tl = [tile2lng(x, zoom), tile2lat(y, zoom)] as [number, number]
      const br = [tile2lng(x + 1, zoom), tile2lat(y + 1, zoom)] as [number, number]
      const [px1, py1] = projectFn(tl)
      const [px2, py2] = projectFn(br)
      tiles.push({
        x,
        y,
        z: zoom,
        url: `https://webst0${(x % 4) + 1}.is.autonavi.com/appmaptile?style=6&x=${x}&y=${y}&z=${zoom}`,
        px: Math.round(px1),
        py: Math.round(py1),
        w: Math.max(1, Math.round(px2 - px1)),
        h: Math.max(1, Math.round(py2 - py1)),
      })
    }
  }
  return tiles
}

// 投影计算
const projectedData = computed(() => {
  const width = canvasWidth.value
  const height = canvasHeight.value
  const preferredProvider = props.trip?.map?.preferred_provider || ''
  const preferredMode = props.trip?.map?.default_mode || ''
  const route = selectPosterRoutes(activeLegs.value, preferredProvider, preferredMode)

  const stopCRS = coordinateSystems.reduce<string>((best, candidate) =>
    activeStops.value.filter(stop => getStopCoord(stop, candidate)).length > activeStops.value.filter(stop => getStopCoord(stop, best)).length ? candidate : best, coordinateSystems[0])
  const routeStopCount = route.crs ? activeStops.value.filter(stop => getStopCoord(stop, route.crs)).length : 0
  const stopCount = activeStops.value.filter(stop => getStopCoord(stop, stopCRS)).length
  const crs = route.crs && routeStopCount >= stopCount ? route.crs : stopCRS
  const routeCoords = route.crs === crs ? route.paths : []

  const validStops = activeStops.value.flatMap((stop, index) => {
    const coord = getStopCoord(stop, crs)
    return coord ? [{ stop, index, coord }] : []
  })

  if (!validStops.length && !routeCoords.length) return null

  const allCoords: Array<[number, number]> = [...validStops.map(s => s.coord), ...routeCoords.flat()]
  let minLng = Infinity, maxLng = -Infinity, minLat = Infinity, maxLat = -Infinity
  for (const [lng, lat] of allCoords) {
    minLng = Math.min(minLng, lng)
    maxLng = Math.max(maxLng, lng)
    minLat = Math.min(minLat, lat)
    maxLat = Math.max(maxLat, lat)
  }

  if (minLng === maxLng) { minLng -= 0.04; maxLng += 0.04 }
  if (minLat === maxLat) { minLat -= 0.03; maxLat += 0.03 }

  const cosLat = Math.max(0.01, Math.cos(((minLat + maxLat) * Math.PI) / 360))
  const padX = width * 0.12
  const padY = height * 0.14
  const innerW = Math.max(1, width - padX * 2)
  const innerH = Math.max(1, height - padY * 2)
  const spanLng = (maxLng - minLng) * cosLat
  const spanLat = maxLat - minLat
  const scale = Math.min(innerW / spanLng, innerH / spanLat)

  const project = ([lng, lat]: [number, number]): [number, number] => [
    Math.round((padX + (innerW - spanLng * scale) / 2 + (lng - minLng) * cosLat * scale) * 10) / 10,
    Math.round((padY + (innerH - spanLat * scale) / 2 + (maxLat - lat) * scale) * 10) / 10,
  ]

  // 1. 投影 Stop 点
  const projectedStops = validStops.map(({ stop, index, coord }) => {
    const [x, y] = project(coord)
    return {
      id: stop.id,
      sequence: index + 1,
      title: stop.title,
      kind: stop.kind,
      x,
      y,
      coord,
      ratio: 0,
    }
  })

  // 2. 拼接平滑完整路径 Path
  let stitchedPathPoints: Array<[number, number]> = []
  if (routeCoords.length > 0) {
    for (const seg of routeCoords) {
      const projSeg = seg.map(project)
      if (!stitchedPathPoints.length) {
        stitchedPathPoints = projSeg
      } else {
        stitchedPathPoints.push(...projSeg)
      }
    }
  } else if (projectedStops.length > 1) {
    stitchedPathPoints = projectedStops.map(s => [s.x, s.y])
  }

  const pathD = stitchedPathPoints.length > 1
    ? stitchedPathPoints.map(([x, y], i) => (i ? 'L' : 'M') + ' ' + x + ' ' + y).join(' ')
    : ''

  // 3. 计算每个 Stop 在整条路线上的位置比例 (ratio)
  if (projectedStops.length > 0) {
    projectedStops[0].ratio = 0
    if (projectedStops.length > 1) {
      projectedStops[projectedStops.length - 1].ratio = 1
      for (let i = 1; i < projectedStops.length - 1; i++) {
        projectedStops[i].ratio = i / (projectedStops.length - 1)
      }
    }
  }

  // 4. 计算真实卫星切片
  const satelliteTiles = calculateSatelliteTiles(minLng, maxLng, minLat, maxLat, width, height, project)

  return {
    width,
    height,
    pathD,
    stitchedPoints: stitchedPathPoints,
    stops: projectedStops,
    satelliteTiles,
    totalStops: projectedStops.length,
  }
})

// 路线总像素长度
const totalPathLength = ref<number>(1000)

watch([() => projectedData.value?.pathD, hiddenPathRef], async () => {
  await nextTick()
  if (hiddenPathRef.value) {
    try {
      const len = hiddenPathRef.value.getTotalLength()
      if (len > 0) totalPathLength.value = Math.round(len)
    } catch {
      totalPathLength.value = 1000
    }
  }
}, { immediate: true })

// 当前播放进度 (0.0 ~ 1.0)
const progress = computed(() => {
  if (duration.value <= 0) return 0
  return Math.min(1, Math.max(0, currentTime.value / duration.value))
})

// 路线描边偏移量 (生长动画)
const currentDashoffset = computed(() => {
  return totalPathLength.value * (1 - progress.value)
})

// 载具当前在路径上的位置与转向角度 (x, y, angle)
const vehicleTransform = computed(() => {
  if (!hiddenPathRef.value || totalPathLength.value <= 0) {
    const firstStop = projectedData.value?.stops?.[0]
    return { x: firstStop?.x || 50, y: firstStop?.y || 50, angle: 0 }
  }
  try {
    const dist = progress.value * totalPathLength.value
    const pt = hiddenPathRef.value.getPointAtLength(dist)
    const nextDist = Math.min(totalPathLength.value, dist + 2)
    const nextPt = hiddenPathRef.value.getPointAtLength(nextDist)
    const dx = nextPt.x - pt.x
    const dy = nextPt.y - pt.y
    const angle = Math.atan2(dy, dx) * (180 / Math.PI)
    return {
      x: Math.round(pt.x * 10) / 10,
      y: Math.round(pt.y * 10) / 10,
      angle: Number.isFinite(angle) ? Math.round(angle * 10) / 10 : 0,
    }
  } catch {
    return { x: 50, y: 50, angle: 0 }
  }
})

// 当前载具类型判定
const activeVehicle = computed(() => {
  if (vehicleType.value !== 'auto') return vehicleType.value
  const defaultMode = props.trip?.map?.default_mode || 'driving'
  if (defaultMode === 'walking') return 'walk'
  if (defaultMode === 'cycling') return 'bike'
  if (defaultMode === 'transit') return 'train'
  return 'car'
})

// 动画主循环
function tick(timestamp: number) {
  if (!isPlaying.value) {
    lastTimestamp = null
    return
  }
  if (lastTimestamp === null) {
    lastTimestamp = timestamp
  }
  const delta = (timestamp - lastTimestamp) / 1000
  lastTimestamp = timestamp

  currentTime.value += delta * playbackSpeed.value
  if (currentTime.value >= duration.value) {
    if (isLoop.value) {
      currentTime.value = 0
    } else {
      currentTime.value = duration.value
      isPlaying.value = false
      lastTimestamp = null
      return
    }
  }
  animFrameId = requestAnimationFrame(tick)
}

function startLoop() {
  if (animFrameId !== null) cancelAnimationFrame(animFrameId)
  lastTimestamp = null
  animFrameId = requestAnimationFrame(tick)
}

function stopLoop() {
  if (animFrameId !== null) {
    cancelAnimationFrame(animFrameId)
    animFrameId = null
  }
  lastTimestamp = null
}

function togglePlay() {
  isPlaying.value = !isPlaying.value
  if (isPlaying.value) {
    if (currentTime.value >= duration.value) {
      currentTime.value = 0
    }
    startLoop()
  } else {
    stopLoop()
  }
}

function replay() {
  currentTime.value = 0
  isPlaying.value = true
  startLoop()
}

function onScrubberInput(e: Event) {
  const target = e.target as HTMLInputElement
  currentTime.value = Number(target.value)
}

function onScopeWheel(e: WheelEvent) {
  if (e.deltaY !== 0 && scopePillsRef.value) {
    e.preventDefault()
    scopePillsRef.value.scrollLeft += e.deltaY
  }
}

function formatTime(sec: number): string {
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s
}

// --- 导出功能 1: 独立可自播 SVG 文件 ---
function generateStandaloneSVG(): string {
  if (!projectedData.value) return ''
  const w = canvasWidth.value
  const h = canvasHeight.value
  const d = projectedData.value.pathD
  const totalL = totalPathLength.value
  const dur = duration.value
  const color = trackColor.value
  const isDark = mapBg.value === 'dark' || mapBg.value === 'satellite'
  const casingColor = isDark ? '#0f172a' : '#ffffff'
  const repeat = isLoop.value ? 'indefinite' : 'freeze'

  // 背景层
  let bgMarkup = ''
  if (mapBg.value === 'dark') {
    bgMarkup = `<rect width="100%" height="100%" fill="#0b1329" />
    <defs>
      <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
        <path d="M 40 0 L 0 0 0 40" fill="none" stroke="#1e293b" stroke-width="0.8" stroke-dasharray="2 3" opacity="0.6" />
      </pattern>
    </defs>
    <rect width="100%" height="100%" fill="url(#grid)" />`
  } else if (mapBg.value === 'light') {
    bgMarkup = `<rect width="100%" height="100%" fill="#f8fafc" />
    <defs>
      <pattern id="gridLight" width="40" height="40" patternUnits="userSpaceOnUse">
        <path d="M 40 0 L 0 0 0 40" fill="none" stroke="#e2e8f0" stroke-width="0.8" stroke-dasharray="2 3" />
      </pattern>
    </defs>
    <rect width="100%" height="100%" fill="url(#gridLight)" />`
  } else if (mapBg.value === 'satellite') {
    const tileImages = (projectedData.value.satelliteTiles || []).map(t =>
      `<image href="${t.url}" x="${t.px}" y="${t.py}" width="${t.w}" height="${t.h}" preserveAspectRatio="none" />`
    ).join('\n')
    bgMarkup = `<rect width="100%" height="100%" fill="#061a14" />
    ${tileImages}
    <rect width="100%" height="100%" fill="rgba(11, 19, 41, 0.2)" />`
  }

  // 载具图标 SVG 代码
  const vehicleMarkup = getVehicleSVGMarkup(activeVehicle.value)

  // 规划点标注
  let pinsMarkup = ''
  if (showPins.value) {
    pinsMarkup = projectedData.value.stops.map(s => {
      const pinColor = color
      return `<g class="stop-pin" transform="translate(${s.x}, ${s.y})">
        <circle r="7" fill="${pinColor}" stroke="#ffffff" stroke-width="2" filter="drop-shadow(0 2px 4px rgba(0,0,0,0.4))" />
        <text y="3" fill="#ffffff" font-size="9" font-weight="900" text-anchor="middle">${s.sequence}</text>
        ${showPopups.value ? `<g transform="translate(0, -14)">
          <rect x="-35" y="-18" width="70" height="18" rx="6" fill="${isDark ? 'rgba(15,23,42,0.85)' : 'rgba(255,255,255,0.92)'}" stroke="${isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.1)'}" />
          <text y="-6" fill="${isDark ? '#ffffff' : '#0f172a'}" font-size="10" font-weight="bold" text-anchor="middle">${escapeXML(s.title)}</text>
        </g>` : ''}
      </g>`
    }).join('\n')
  }

  const titleText = escapeXML(props.trip?.title || '旅行规划路线')
  const dateText = props.trip?.date_range ? `${props.trip.date_range.start} ~ ${props.trip.date_range.end}` : ''

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="100%" height="100%">
  ${bgMarkup}

  <!-- 行程标题标识 -->
  <g transform="translate(24, 36)">
    <text x="0" y="0" fill="${isDark ? '#f8fafc' : '#0f172a'}" font-size="18" font-weight="800">${titleText}</text>
    ${dateText ? `<text x="0" y="18" fill="${isDark ? '#94a3b8' : '#64748b'}" font-size="11" font-weight="600">${dateText}</text>` : ''}
  </g>

  <!-- 路线路径定义 -->
  <defs>
    <path id="route-path" d="${d}" fill="none" />
  </defs>

  <!-- 轨迹外沿立体描边 (Casing) -->
  <use href="#route-path" stroke="${casingColor}" stroke-width="8" stroke-linecap="round" stroke-linejoin="round" opacity="0.85" />

  <!-- 动态延伸生长的轨迹线 -->
  <path d="${d}" fill="none" stroke="${color}" stroke-width="4.5" stroke-linecap="round" stroke-linejoin="round"
        stroke-dasharray="${totalL}" stroke-dashoffset="${totalL}">
    <animate attributeName="stroke-dashoffset" from="${totalL}" to="0" dur="${dur}s" repeatCount="${repeat}" />
  </path>

  <!-- 规划点标记 -->
  ${pinsMarkup}

  <!-- 沿路径自动前进与转向的载具 -->
  <g id="animated-vehicle">
    ${vehicleMarkup}
    <animateMotion dur="${dur}s" repeatCount="${repeat}" rotate="auto">
      <mpath href="#route-path" />
    </animateMotion>
  </g>
</svg>`
}

function getVehicleSVGMarkup(type: string): string {
  if (type === 'plane') {
    return `<path d="M 18 0 L 7 -4 L -3 -20 L -7 -20 L -2 -4 L -11 -4 L -15 -10 L -18 -10 L -16 -1 L -16 0 L -16 1 L -18 10 L -15 10 L -11 4 L -2 4 L -7 20 L -3 20 L 7 4 Z" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" filter="drop-shadow(0 3px 5px rgba(0,0,0,0.4))" />
    <circle cx="14" cy="0" r="1.5" fill="#38bdf8" />`
  }
  if (type === 'train') {
    return `<path d="M -16 -6 L 10 -6 Q 18 -6 18 0 Q 18 6 10 6 L -16 6 Z" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" filter="drop-shadow(0 2px 4px rgba(0,0,0,0.3))" />
    <path d="M 10 -4 L 16 0 L 10 4 Z" fill="#38bdf8" />
    <line x1="-12" y1="-2" x2="6" y2="-2" stroke="#ef4444" stroke-width="1.2" />
    <line x1="-12" y1="2" x2="6" y2="2" stroke="#ef4444" stroke-width="1.2" />`
  }
  if (type === 'bike') {
    return `<circle cx="-7" cy="0" r="4" fill="none" stroke="#ffffff" stroke-width="1.5" />
    <circle cx="7" cy="0" r="4" fill="none" stroke="#ffffff" stroke-width="1.5" />
    <path d="M -7 0 L 0 -5 L 7 0 M 0 -5 L -2 0 M 0 -5 L 3 -7" stroke="#ffffff" stroke-width="1.5" stroke-linecap="round" />
    <circle cx="0" cy="-8" r="2.5" fill="#f59e0b" />`
  }
  if (type === 'walk') {
    return `<circle cx="0" cy="-6" r="3" fill="#ffffff" stroke="#0f172a" stroke-width="1" />
    <rect x="-4" y="-3" width="7" height="9" rx="2" fill="#3b82f6" stroke="#0f172a" stroke-width="1" />
    <rect x="-6" y="-2" width="3" height="6" rx="1" fill="#f59e0b" />
    <line x1="-2" y1="6" x2="-3" y2="12" stroke="#0f172a" stroke-width="2" stroke-linecap="round" />
    <line x1="1" y1="6" x2="3" y2="11" stroke="#0f172a" stroke-width="2" stroke-linecap="round" />`
  }
  return `<rect x="-14" y="-8" width="28" height="16" rx="5" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" filter="drop-shadow(0 2px 4px rgba(0,0,0,0.35))" />
  <rect x="-11" y="-9.5" width="6" height="2" rx="1" fill="#0f172a" />
  <rect x="5" y="-9.5" width="6" height="2" rx="1" fill="#0f172a" />
  <rect x="-11" y="7.5" width="6" height="2" rx="1" fill="#0f172a" />
  <rect x="5" y="7.5" width="6" height="2" rx="1" fill="#0f172a" />
  <path d="M -3 -6 L 5 -5 L 5 5 L -3 6 Z" fill="#38bdf8" />
  <rect x="-9" y="-5.5" width="4" height="11" rx="1.5" fill="#0284c7" />
  <circle cx="-13" cy="-5" r="1.2" fill="#ef4444" />
  <circle cx="-13" cy="5" r="1.2" fill="#ef4444" />`
}

function escapeXML(str: string): string {
  return str.replace(/[<>&'"]/g, c => {
    switch (c) {
      case '<': return '&lt;'
      case '>': return '&gt;'
      case '&': return '&amp;'
      case "'": return '&apos;'
      case '"': return '&quot;'
      default: return c
    }
  })
}

// 触发下载 SVG
function downloadSVG() {
  const svgText = generateStandaloneSVG()
  if (!svgText) {
    toastMessage.value = '暂无可导出的路线数据'
    return
  }
  const blob = new Blob([svgText], { type: 'image/svg+xml;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  const sanitizedTitle = (props.trip?.title || 'trip-route').replace(/[/\\?%*:|"<>]/g, '-')
  const daySuffix = activeDayIndex.value >= 0 ? `-Day${activeDayIndex.value + 1}` : '-All'
  a.download = `${sanitizedTitle}${daySuffix}-animation.svg`
  a.href = url
  a.click()
  URL.revokeObjectURL(url)
  toastMessage.value = '独立 SVG 动画已下载！可直接在任意浏览器打开即播。'
}

// 复制 SVG 代码
async function copySVGCode() {
  const svgText = generateStandaloneSVG()
  if (!svgText) return
  try {
    if (navigator.clipboard?.writeText && window.isSecureContext) {
      await navigator.clipboard.writeText(svgText)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = svgText
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    toastMessage.value = 'SVG 源码已复制到剪贴板！'
  } catch {
    toastMessage.value = '复制失败，请尝试直接下载 SVG 文件。'
  }
}

// --- 导出功能 2: 纯浏览器端高清视频录制 (MP4 / WebM) ---
async function exportVideo() {
  if (isExportingVideo.value || !projectedData.value || !hiddenPathRef.value) return
  isExportingVideo.value = true
  exportProgress.value = 0
  toastMessage.value = '正在预加载卫星切片并逐帧录制高清视频…'

  const wasPlaying = isPlaying.value
  stopLoop()
  isPlaying.value = false

  try {
    const targetW = aspectRatio.value === '9:16' ? 1080 : 1920
    const targetH = aspectRatio.value === '9:16' ? 1920 : 1080
    const canvas = document.createElement('canvas')
    canvas.width = targetW
    canvas.height = targetH
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('无法初始化 Canvas 绘图环境')

    // 如果是卫星图，预先并发加载切片图片
    let preloadedSatelliteImgs: HTMLImageElement[] = []
    if (mapBg.value === 'satellite' && projectedData.value.satelliteTiles.length > 0) {
      preloadedSatelliteImgs = await Promise.all(
        projectedData.value.satelliteTiles.map(t => new Promise<HTMLImageElement>(resolve => {
          const img = new Image()
          img.crossOrigin = 'anonymous'
          img.onload = () => resolve(img)
          img.onerror = () => resolve(img)
          img.src = t.url
        }))
      )
    }

    // 探测支持的视频编码
    const mimeCandidates = [
      'video/mp4;codecs=avc1',
      'video/mp4',
      'video/webm;codecs=vp9',
      'video/webm;codecs=vp8',
      'video/webm',
    ]
    const mimeType = (typeof MediaRecorder !== 'undefined' && mimeCandidates.find(m => MediaRecorder.isTypeSupported(m))) || 'video/webm'
    const ext = mimeType.includes('mp4') ? 'mp4' : 'webm'

    const stream = canvas.captureStream(30)
    const recorder = new MediaRecorder(stream, { mimeType, videoBitsPerSecond: 8_000_000 })
    const chunks: Blob[] = []
    recorder.ondataavailable = e => { if (e.data && e.data.size > 0) chunks.push(e.data) }

    recorder.start()

    const fps = 30
    const totalFrames = fps * duration.value
    const scaleFactorX = targetW / canvasWidth.value
    const scaleFactorY = targetH / canvasHeight.value

    for (let f = 0; f <= totalFrames; f++) {
      const t = f / totalFrames
      renderCanvasFrame(ctx, targetW, targetH, t, scaleFactorX, scaleFactorY, preloadedSatelliteImgs)
      exportProgress.value = Math.round((f / totalFrames) * 100)
      await new Promise(r => setTimeout(r, 6))
    }

    recorder.stop()
    await new Promise(resolve => { recorder.onstop = resolve })

    const videoBlob = new Blob(chunks, { type: mimeType })
    const url = URL.createObjectURL(videoBlob)
    const a = document.createElement('a')
    const sanitizedTitle = (props.trip?.title || 'trip-route').replace(/[/\\?%*:|"<>]/g, '-')
    const daySuffix = activeDayIndex.value >= 0 ? `-Day${activeDayIndex.value + 1}` : '-All'
    a.download = `${sanitizedTitle}${daySuffix}-animation.${ext}`
    a.href = url
    a.click()
    URL.revokeObjectURL(url)

    toastMessage.value = `高清短视频已生成 (.${ext})！可直接分享小红书或朋友圈。`
  } catch (err) {
    console.error('Video record error:', err)
    toastMessage.value = '视频录制异常：' + (err instanceof Error ? err.message : String(err))
  } finally {
    isExportingVideo.value = false
    exportProgress.value = 0
    if (wasPlaying) {
      isPlaying.value = true
      startLoop()
    }
  }
}

// 离屏 Canvas 单帧渲染算法
function renderCanvasFrame(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  t: number,
  scaleX: number,
  scaleY: number,
  satelliteImgs: HTMLImageElement[] = []
) {
  if (!projectedData.value || !hiddenPathRef.value) return
  ctx.clearRect(0, 0, width, height)

  // 1. 绘制底图背景
  if (mapBg.value === 'dark') {
    const grad = ctx.createLinearGradient(0, 0, 0, height)
    grad.addColorStop(0, '#0b1329')
    grad.addColorStop(1, '#0f172a')
    ctx.fillStyle = grad
    ctx.fillRect(0, 0, width, height)

    ctx.strokeStyle = '#1e293b'
    ctx.lineWidth = 1
    ctx.setLineDash([4, 6])
    for (let x = 0; x < width; x += 80) { ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, height); ctx.stroke() }
    for (let y = 0; y < height; y += 80) { ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(width, y); ctx.stroke() }
    ctx.setLineDash([])
  } else if (mapBg.value === 'light') {
    ctx.fillStyle = '#f8fafc'
    ctx.fillRect(0, 0, width, height)
    ctx.strokeStyle = '#e2e8f0'
    ctx.lineWidth = 1
    ctx.setLineDash([4, 6])
    for (let x = 0; x < width; x += 80) { ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, height); ctx.stroke() }
    for (let y = 0; y < height; y += 80) { ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(width, y); ctx.stroke() }
    ctx.setLineDash([])
  } else if (mapBg.value === 'satellite') {
    ctx.fillStyle = '#061a14'
    ctx.fillRect(0, 0, width, height)
    if (satelliteImgs.length > 0) {
      for (let i = 0; i < projectedData.value.satelliteTiles.length; i++) {
        const tile = projectedData.value.satelliteTiles[i]
        const img = satelliteImgs[i]
        if (img && img.complete && img.naturalWidth > 0) {
          ctx.drawImage(img, tile.px * scaleX, tile.py * scaleY, tile.w * scaleX, tile.h * scaleY)
        }
      }
      ctx.fillStyle = 'rgba(11, 19, 41, 0.2)'
      ctx.fillRect(0, 0, width, height)
    }
  }

  // 2. 标题文字
  const isDark = mapBg.value === 'dark' || mapBg.value === 'satellite'
  ctx.fillStyle = isDark ? '#f8fafc' : '#0f172a'
  ctx.font = 'bold 32px sans-serif'
  ctx.fillText(props.trip?.title || '旅行规划路线', 48, 64)
  if (props.trip?.date_range) {
    ctx.fillStyle = isDark ? '#94a3b8' : '#64748b'
    ctx.font = '500 20px sans-serif'
    ctx.fillText(`${props.trip.date_range.start} ~ ${props.trip.date_range.end}`, 48, 98)
  }

  // 3. 绘制截至当前进展 t 的折线段 (严密锁死载具实时坐标，彻底消除延迟脱节)
  const totalL = totalPathLength.value
  const currentDist = t * totalL
  if (currentDist > 0 && hiddenPathRef.value) {
    ctx.lineWidth = 14
    ctx.strokeStyle = isDark ? '#0f172a' : '#ffffff'
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    drawSubPath(ctx, hiddenPathRef.value, currentDist, totalL, scaleX, scaleY)
    ctx.stroke()

    ctx.lineWidth = 8
    ctx.strokeStyle = trackColor.value
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    drawSubPath(ctx, hiddenPathRef.value, currentDist, totalL, scaleX, scaleY)
    ctx.stroke()
  }

  // 4. 绘制已到达规划点
  if (showPins.value) {
    for (const s of projectedData.value.stops) {
      if (t >= s.ratio) {
        const sx = s.x * scaleX
        const sy = s.y * scaleY
        ctx.save()
        ctx.fillStyle = trackColor.value
        ctx.strokeStyle = '#ffffff'
        ctx.lineWidth = 3
        ctx.beginPath()
        ctx.arc(sx, sy, 12, 0, Math.PI * 2)
        ctx.fill()
        ctx.stroke()

        ctx.fillStyle = '#ffffff'
        ctx.font = 'bold 13px sans-serif'
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'
        ctx.fillText(String(s.sequence), sx, sy)

        if (showPopups.value) {
          ctx.font = 'bold 16px sans-serif'
          const tw = ctx.measureText(s.title).width + 24
          const th = 30
          ctx.fillStyle = isDark ? 'rgba(15,23,42,0.9)' : 'rgba(255,255,255,0.95)'
          ctx.strokeStyle = isDark ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.15)'
          ctx.lineWidth = 1.5
          roundRect(ctx, sx - tw / 2, sy - 44, tw, th, 8)
          ctx.fill()
          ctx.stroke()
          ctx.fillStyle = isDark ? '#ffffff' : '#0f172a'
          ctx.fillText(s.title, sx, sy - 29)
        }
        ctx.restore()
      }
    }
  }

  // 5. 绘制载具图标
  try {
    const pt = hiddenPathRef.value.getPointAtLength(currentDist)
    const nextPt = hiddenPathRef.value.getPointAtLength(Math.min(totalL, currentDist + 2))
    const angle = Math.atan2(nextPt.y - pt.y, nextPt.x - pt.x)
    const vx = pt.x * scaleX
    const vy = pt.y * scaleY

    ctx.save()
    ctx.translate(vx, vy)
    ctx.rotate(angle)

    ctx.fillStyle = '#ffffff'
    ctx.strokeStyle = '#0f172a'
    ctx.lineWidth = 2
    roundRect(ctx, -24, -14, 48, 28, 8)
    ctx.fill()
    ctx.stroke()
    ctx.fillStyle = '#38bdf8'
    ctx.fillRect(-6, -10, 14, 20)
    ctx.fillStyle = '#ef4444'
    ctx.fillRect(-23, -10, 3, 5)
    ctx.fillRect(-23, 5, 3, 5)
    ctx.restore()
  } catch {
    // 容错处理
  }
}

function drawSubPath(
  ctx: CanvasRenderingContext2D,
  pathEl: SVGPathElement,
  currentDist: number,
  totalDist: number,
  scaleX: number,
  scaleY: number
) {
  if (currentDist <= 0) return
  ctx.beginPath()
  const p0 = pathEl.getPointAtLength(0)
  ctx.moveTo(p0.x * scaleX, p0.y * scaleY)

  const step = Math.max(3, Math.min(8, totalDist / 260))
  for (let d = step; d < currentDist; d += step) {
    const p = pathEl.getPointAtLength(d)
    ctx.lineTo(p.x * scaleX, p.y * scaleY)
  }
  // 路线末端严格连接至当前载具真实中心点，保证绘制与车头零延迟贴合
  const curP = pathEl.getPointAtLength(currentDist)
  ctx.lineTo(curP.x * scaleX, curP.y * scaleY)
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.moveTo(x + r, y)
  ctx.arcTo(x + w, y, x + w, y + h, r)
  ctx.arcTo(x + w, y + h, x, y + h, r)
  ctx.arcTo(x, y + h, x, y, r)
  ctx.arcTo(x, y, x + w, y, r)
  ctx.closePath()
}

onMounted(() => {
  if (props.isOpen) {
    startLoop()
  }
})

onUnmounted(() => {
  stopLoop()
})
</script>

<template>
  <div v-if="isOpen" class="modal-backdrop animation-modal-backdrop" @click.self="emit('close')">
    <div class="animation-modal" role="dialog" aria-modal="true" aria-labelledby="animation-title">
      <!-- 头部 -->
      <header class="animation-modal-header">
        <div>
          <p class="eyebrow">TRIP ROUTE ANIMATION</p>
          <h2 id="animation-title">行程动态路线动画</h2>
          <p class="modal-subtitle">{{ trip?.title || '未命名行程' }}</p>
        </div>
        <button class="modal-close" type="button" aria-label="关闭动画弹窗" @click="emit('close')">×</button>
      </header>

      <!-- 顶栏快捷范围选择 (两行布局，保证行程范围有充裕横向滑动空间) -->
      <div class="animation-top-bar">
        <div class="top-bar-row">
          <span class="bar-label">行程范围</span>
          <div ref="scopePillsRef" class="scope-pills" @wheel="onScopeWheel">
            <button
              type="button"
              class="scope-btn"
              :class="{ active: activeDayIndex === -1 }"
              @click="activeDayIndex = -1"
            >
              全程漫游
            </button>
            <button
              v-for="(d, idx) in allDays"
              :key="d.id"
              type="button"
              class="scope-btn"
              :class="{ active: activeDayIndex === idx }"
              @click="activeDayIndex = idx"
            >
              Day {{ idx + 1 }}
            </button>
          </div>
        </div>

        <div class="top-bar-row">
          <span class="bar-label">画幅比例</span>
          <div class="aspect-pills">
            <button
              v-for="asp in (['16:9', '9:16', '4:3', '1:1'] as AspectRatio[])"
              :key="asp"
              type="button"
              class="aspect-btn"
              :class="{ active: aspectRatio === asp }"
              @click="aspectRatio = asp"
            >
              {{ asp }}
            </button>
          </div>
        </div>
      </div>

      <!-- 核心工作区 (左侧预览播放，右侧配置) -->
      <div class="animation-workspace">
        <!-- 左侧：动画预览播放器 -->
        <div class="player-container">
          <div
            ref="svgContainerRef"
            class="svg-stage"
            :class="['bg-' + mapBg]"
            :style="{ aspectRatio: aspectRatio === '16:9' ? '16/9' : aspectRatio === '9:16' ? '9/16' : aspectRatio === '4:3' ? '4/3' : '1/1' }"
          >
            <!-- 隐藏的 Path 元素，用于几何长度测量与插值 -->
            <svg style="position: absolute; width: 0; height: 0; pointer-events: none;" aria-hidden="true">
              <path ref="hiddenPathRef" :d="projectedData?.pathD || ''" fill="none" />
            </svg>

            <!-- 主互动渲染 SVG -->
            <svg
              v-if="projectedData"
              :viewBox="`0 0 ${projectedData.width} ${projectedData.height}`"
              class="interactive-svg"
              xmlns="http://www.w3.org/2000/svg"
            >
              <defs>
                <clipPath id="stageClip">
                  <rect width="100%" height="100%" rx="16" />
                </clipPath>
              </defs>

              <!-- 极简背景装饰 -->
              <template v-if="mapBg === 'dark'">
                <rect width="100%" height="100%" fill="#0b1329" />
                <line v-for="i in 5" :key="'vl'+i" :x1="projectedData.width * (i/6)" y1="0" :x2="projectedData.width * (i/6)" :y2="projectedData.height" stroke="#1e293b" stroke-width="0.8" stroke-dasharray="3 4" opacity="0.6" />
                <line v-for="i in 5" :key="'hl'+i" x1="0" :y1="projectedData.height * (i/6)" :x2="projectedData.width" :y2="projectedData.height * (i/6)" stroke="#1e293b" stroke-width="0.8" stroke-dasharray="3 4" opacity="0.6" />
              </template>
              <template v-else-if="mapBg === 'light'">
                <rect width="100%" height="100%" fill="#f8fafc" />
                <line v-for="i in 5" :key="'vl'+i" :x1="projectedData.width * (i/6)" y1="0" :x2="projectedData.width * (i/6)" :y2="projectedData.height" stroke="#e2e8f0" stroke-width="0.8" stroke-dasharray="3 4" />
                <line v-for="i in 5" :key="'hl'+i" x1="0" :y1="projectedData.height * (i/6)" :x2="projectedData.width" :y2="projectedData.height * (i/6)" stroke="#e2e8f0" stroke-width="0.8" stroke-dasharray="3 4" />
              </template>
              <template v-else-if="mapBg === 'satellite'">
                <!-- 真实卫星切片图层 -->
                <rect width="100%" height="100%" fill="#061a14" />
                <g clip-path="url(#stageClip)">
                  <image
                    v-for="(tile, i) in projectedData.satelliteTiles"
                    :key="'sat-'+i"
                    :href="tile.url"
                    :x="tile.px"
                    :y="tile.py"
                    :width="tile.w"
                    :height="tile.h"
                    preserveAspectRatio="none"
                  />
                  <!-- 柔和遮罩增强路线对比度 -->
                  <rect width="100%" height="100%" fill="rgba(11, 19, 41, 0.2)" />
                </g>
              </template>

              <!-- 左上角标题徽标 -->
              <g transform="translate(20, 32)">
                <text x="0" y="0" :fill="mapBg === 'light' ? '#0f172a' : '#f8fafc'" font-size="16" font-weight="800">{{ trip?.title || '旅行规划' }}</text>
                <text v-if="trip?.date_range" x="0" y="16" :fill="mapBg === 'light' ? '#64748b' : '#94a3b8'" font-size="10" font-weight="600">{{ trip.date_range.start }} ~ {{ trip.date_range.end }}</text>
              </g>

              <!-- 轨迹外层描边 (Casing) -->
              <path
                :d="projectedData.pathD"
                fill="none"
                :stroke="mapBg === 'light' ? '#ffffff' : '#0f172a'"
                stroke-width="8"
                stroke-linecap="round"
                stroke-linejoin="round"
                opacity="0.88"
              />

              <!-- 动态生长轨迹核心线 -->
              <path
                :d="projectedData.pathD"
                fill="none"
                :stroke="trackColor"
                stroke-width="4.5"
                stroke-linecap="round"
                stroke-linejoin="round"
                :stroke-dasharray="totalPathLength"
                :stroke-dashoffset="currentDashoffset"
              />

              <!-- 规划点图钉 -->
              <template v-if="showPins">
                <g
                  v-for="s in projectedData.stops"
                  :key="s.id"
                  class="pin-group"
                  :class="{ reached: progress >= s.ratio }"
                  :transform="`translate(${s.x}, ${s.y})`"
                >
                  <!-- 到达时的涟漪光圈 -->
                  <circle
                    v-if="Math.abs(progress - s.ratio) < 0.03"
                    r="16"
                    fill="none"
                    :stroke="trackColor"
                    stroke-width="1.5"
                    class="radar-ripple"
                  />
                  <!-- 图钉圆圈 -->
                  <circle
                    r="8"
                    :fill="progress >= s.ratio ? trackColor : (mapBg === 'light' ? '#cbd5e1' : '#334155')"
                    :stroke="mapBg === 'light' ? '#ffffff' : '#0f172a'"
                    stroke-width="2"
                  />
                  <text y="3" fill="#ffffff" font-size="9" font-weight="900" text-anchor="middle">{{ s.sequence }}</text>

                  <!-- 到达时展开的地名卡片 -->
                  <g v-if="showPopups && progress >= s.ratio" transform="translate(0, -16)" class="pin-popup">
                    <rect
                      :x="-(s.title.length * 6 + 14)"
                      y="-18"
                      :width="s.title.length * 12 + 28"
                      height="20"
                      rx="6"
                      :fill="mapBg === 'light' ? 'rgba(255,255,255,0.92)' : 'rgba(15,23,42,0.85)'"
                      :stroke="mapBg === 'light' ? 'rgba(0,0,0,0.1)' : 'rgba(255,255,255,0.2)'"
                    />
                    <text y="-4" :fill="mapBg === 'light' ? '#0f172a' : '#ffffff'" font-size="11" font-weight="700" text-anchor="middle">
                      {{ s.title }}
                    </text>
                  </g>
                </g>
              </template>

              <!-- 交通载具 (随路径平滑行进与转向) -->
              <g
                class="moving-vehicle"
                :transform="`translate(${vehicleTransform.x}, ${vehicleTransform.y}) rotate(${vehicleTransform.angle})`"
              >
                <!-- 越野车 / 轿车 (纯净车身，无前大灯光晕) -->
                <g v-if="activeVehicle === 'car'">
                  <rect x="-14" y="-8" width="28" height="16" rx="5" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" />
                  <rect x="-11" y="-9.5" width="6" height="2" rx="1" fill="#0f172a" />
                  <rect x="5" y="-9.5" width="6" height="2" rx="1" fill="#0f172a" />
                  <rect x="-11" y="7.5" width="6" height="2" rx="1" fill="#0f172a" />
                  <rect x="5" y="7.5" width="6" height="2" rx="1" fill="#0f172a" />
                  <path d="M -3 -6 L 5 -5 L 5 5 L -3 6 Z" fill="#38bdf8" />
                  <rect x="-9" y="-5.5" width="4" height="11" rx="1.5" fill="#0284c7" />
                  <circle cx="-13" cy="-5" r="1.2" fill="#ef4444" />
                  <circle cx="-13" cy="5" r="1.2" fill="#ef4444" />
                </g>
                <!-- 飞机 -->
                <g v-else-if="activeVehicle === 'plane'">
                  <path d="M 18 0 L 7 -4 L -3 -20 L -7 -20 L -2 -4 L -11 -4 L -15 -10 L -18 -10 L -16 -1 L -16 0 L -16 1 L -18 10 L -15 10 L -11 4 L -2 4 L -7 20 L -3 20 L 7 4 Z" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" />
                  <circle cx="14" cy="0" r="1.5" fill="#38bdf8" />
                </g>
                <!-- 高铁 / 列车 -->
                <g v-else-if="activeVehicle === 'train'">
                  <path d="M -16 -6 L 10 -6 Q 18 -6 18 0 Q 18 6 10 6 L -16 6 Z" fill="#ffffff" stroke="#0f172a" stroke-width="1.2" />
                  <path d="M 10 -4 L 16 0 L 10 4 Z" fill="#38bdf8" />
                  <line x1="-12" y1="-2" x2="6" y2="-2" stroke="#ef4444" stroke-width="1.2" />
                  <line x1="-12" y1="2" x2="6" y2="2" stroke="#ef4444" stroke-width="1.2" />
                </g>
                <!-- 自行车 -->
                <g v-else-if="activeVehicle === 'bike'">
                  <circle cx="-7" cy="0" r="4" fill="none" stroke="#ffffff" stroke-width="1.5" />
                  <circle cx="7" cy="0" r="4" fill="none" stroke="#ffffff" stroke-width="1.5" />
                  <path d="M -7 0 L 0 -5 L 7 0 M 0 -5 L -2 0 M 0 -5 L 3 -7" stroke="#ffffff" stroke-width="1.5" stroke-linecap="round" />
                  <circle cx="0" cy="-8" r="2.5" fill="#f59e0b" />
                </g>
                <!-- 徒步漫游 -->
                <g v-else-if="activeVehicle === 'walk'">
                  <circle cx="0" cy="-6" r="3" fill="#ffffff" stroke="#0f172a" stroke-width="1" />
                  <rect x="-4" y="-3" width="7" height="9" rx="2" fill="#3b82f6" stroke="#0f172a" stroke-width="1" />
                  <rect x="-6" y="-2" width="3" height="6" rx="1" fill="#f59e0b" />
                  <line x1="-2" y1="6" x2="-3" y2="12" stroke="#0f172a" stroke-width="2" stroke-linecap="round" />
                  <line x1="1" y1="6" x2="3" y2="11" stroke="#0f172a" stroke-width="2" stroke-linecap="round" />
                </g>
              </g>
            </svg>
            <div v-else class="stage-empty">
              <span>暂无有效路线坐标，请先在行程中添加带位置的规划点</span>
            </div>
          </div>

          <!-- 播放控制工具条 (Scrubber & Buttons) -->
          <div class="player-controls">
            <button type="button" class="ctrl-btn play-btn" :aria-label="isPlaying ? '暂停' : '播放'" @click="togglePlay">
              <svg v-if="isPlaying" width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                <rect x="5" y="3" width="5" height="18" rx="2" />
                <rect x="14" y="3" width="5" height="18" rx="2" />
              </svg>
              <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                <polygon points="6,3 21,12 6,21" />
              </svg>
            </button>
            <button type="button" class="ctrl-btn replay-btn" title="重播" @click="replay">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
                <path d="M3 3v5h5" />
              </svg>
            </button>
            <span class="time-counter">{{ formatTime(currentTime) }} / {{ formatTime(duration) }}</span>
            <input
              type="range"
              class="scrubber-slider"
              min="0"
              :max="duration"
              step="0.05"
              :value="currentTime"
              @input="onScrubberInput"
            />
            <div class="speed-selector">
              <button
                v-for="sp in [0.5, 1, 2]"
                :key="sp"
                type="button"
                class="speed-btn"
                :class="{ active: playbackSpeed === sp }"
                @click="playbackSpeed = sp"
              >
                {{ sp }}x
              </button>
            </div>
            <button
              type="button"
              class="ctrl-btn loop-btn"
              :class="{ active: isLoop }"
              :title="isLoop ? '循环播放已开启' : '循环播放已关闭'"
              @click="isLoop = !isLoop"
            >
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 2l4 4-4 4" />
                <path d="M3 11v-1a4 4 0 0 1 4-4h14" />
                <path d="M7 22l-4-4 4-4" />
                <path d="M21 13v1a4 4 0 0 1-4 4H3" />
              </svg>
            </button>
          </div>
        </div>

        <!-- 右侧：个性化设置面板 (独立平滑滚动) -->
        <div class="options-container">
          <!-- 1. 底图样式 -->
          <div class="opt-group">
            <label class="opt-label">地图底图样式</label>
            <div class="pill-grid">
              <button
                v-for="bg in ([{ id: 'dark', label: '极简暗夜' }, { id: 'light', label: '高雅明亮' }, { id: 'satellite', label: '卫星实景' }, { id: 'transparent', label: '透底画板' }] as const)"
                :key="bg.id"
                type="button"
                class="opt-pill-btn"
                :class="{ active: mapBg === bg.id }"
                @click="mapBg = bg.id"
              >
                {{ bg.label }}
              </button>
            </div>
          </div>

          <!-- 2. 规划点标记与气泡 -->
          <div class="opt-group">
            <label class="opt-label">规划点图钉展示</label>
            <div class="toggle-row">
              <label class="checkbox-label">
                <input v-model="showPins" type="checkbox" />
                <span>显示站点标记图钉</span>
              </label>
              <label class="checkbox-label">
                <input v-model="showPopups" type="checkbox" :disabled="!showPins" />
                <span>到达站点弹出地名气泡</span>
              </label>
            </div>
          </div>

          <!-- 3. 交通载具类型 -->
          <div class="opt-group">
            <label class="opt-label">交通工具载具</label>
            <div class="pill-grid vehicle-grid">
              <button
                v-for="v in ([{ id: 'auto', label: '⚡ 智能' }, { id: 'car', label: '🚗 越野' }, { id: 'plane', label: '✈️ 飞机' }, { id: 'train', label: '🚄 高铁' }, { id: 'bike', label: '🚴 骑行' }, { id: 'walk', label: '🚶 徒步' }] as const)"
                :key="v.id"
                type="button"
                class="opt-pill-btn"
                :class="{ active: vehicleType === v.id }"
                @click="vehicleType = v.id"
              >
                {{ v.label }}
              </button>
            </div>
          </div>

          <!-- 4. 轨迹线色彩 -->
          <div class="opt-group">
            <label class="opt-label">轨迹线主题色</label>
            <div class="color-options">
              <button
                v-for="c in ([{ id: '#ff6b35', label: '落日暖橙' }, { id: '#00d2ff', label: '极光天青' }, { id: '#24695c', label: '经典青绿' }, { id: '#f43f5e', label: '探险洋红' }] as const)"
                :key="c.id"
                type="button"
                class="color-btn"
                :class="{ active: trackColor === c.id }"
                :style="{ '--c-accent': c.id }"
                @click="trackColor = c.id"
              >
                <span class="color-dot" :style="{ backgroundColor: c.id }"></span>
                <span class="color-text">{{ c.label }}</span>
              </button>
            </div>
          </div>

          <!-- 5. 动画时长 -->
          <div class="opt-group">
            <label class="opt-label">动画行进时长</label>
            <div class="pill-grid">
              <button
                v-for="d in ([{ sec: 10, label: '10s 快放' }, { sec: 15, label: '15s 推荐' }, { sec: 30, label: '30s 漫游' }] as const)"
                :key="d.sec"
                type="button"
                class="opt-pill-btn"
                :class="{ active: duration === d.sec }"
                @click="duration = d.sec; replay()"
              >
                {{ d.label }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部固定操作栏 (Sticky Footer, 无论屏幕高低绝不溢出) -->
      <footer class="animation-modal-footer">
        <div class="footer-status">
          <span v-if="toastMessage" class="toast-tip">{{ toastMessage }}</span>
        </div>
        <div class="footer-actions">
          <button type="button" class="export-btn secondary-btn" @click="copySVGCode">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
            <span>复制代码</span>
          </button>
          <button type="button" class="export-btn primary-btn" @click="downloadSVG">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            <span>下载 SVG 动图</span>
          </button>
          <button
            type="button"
            class="export-btn video-btn"
            :disabled="isExportingVideo"
            @click="exportVideo"
          >
            <svg v-if="!isExportingVideo" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polygon points="23 7 16 12 23 17 23 7" />
              <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
            </svg>
            <span v-if="isExportingVideo">正在录制 {{ exportProgress }}%…</span>
            <span v-else>导出 MP4 / WebM</span>
          </button>
        </div>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.animation-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, #000 68%, transparent);
  backdrop-filter: blur(8px);
  padding: 14px;
  box-sizing: border-box;
}

.animation-modal {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(1040px, 100%);
  max-width: 1040px;
  height: min(92vh, 880px);
  max-height: calc(100vh - 28px);
  background: var(--surface);
  color: var(--ink);
  border-radius: 20px;
  box-shadow: 0 20px 60px -10px rgba(0, 0, 0, 0.55);
  overflow: hidden;
  border: 1px solid var(--line);
  box-sizing: border-box;
  margin: auto;
}

.animation-modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 14px 20px 12px;
  background: var(--surface);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.animation-modal-header .eyebrow {
  margin: 0 0 2px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: var(--p);
}

.animation-modal-header h2 {
  margin: 0;
  font-size: 19px;
  font-weight: 800;
  color: var(--ink) !important;
  letter-spacing: -0.01em;
}

.animation-modal-header .modal-subtitle {
  margin: 2px 0 0;
  color: var(--muted);
  font-size: 12px;
}

.modal-close {
  width: 34px;
  height: 34px;
  border: 1px solid var(--line);
  border-radius: 50%;
  background: transparent;
  color: var(--ink);
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.15s ease;
  flex-shrink: 0;
}

.modal-close:hover {
  background: var(--panel);
}

.animation-top-bar {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 20px;
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.top-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-width: 0;
}

.bar-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--muted);
  white-space: nowrap;
  flex-shrink: 0;
  width: 52px;
}

.scope-pills {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  scrollbar-color: var(--line) transparent;
  -webkit-overflow-scrolling: touch;
  padding: 2px 2px 4px;
  flex: 1;
  min-width: 0;
}

.scope-pills::-webkit-scrollbar {
  height: 4px;
}
.scope-pills::-webkit-scrollbar-track {
  background: transparent;
}
.scope-pills::-webkit-scrollbar-thumb {
  background: var(--line);
  border-radius: 999px;
}

.aspect-pills {
  display: flex;
  gap: 6px;
  flex: 1;
  overflow-x: auto;
}

.scope-btn, .aspect-btn {
  padding: 4px 10px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface);
  color: var(--ink);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
  flex-shrink: 0;
}

.scope-btn:hover, .aspect-btn:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--surface));
}

.scope-btn.active, .aspect-btn.active {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
  border-color: var(--p);
}

.animation-workspace {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px 20px;
  overflow-y: auto;
  overflow-x: hidden;
  flex: 1;
  min-height: 0;
  box-sizing: border-box;
}

@media (min-width: 860px) {
  .animation-workspace {
    display: grid;
    grid-template-columns: minmax(0, 1.25fr) minmax(280px, 320px);
    gap: 20px;
  }
}

.player-container {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
  width: 100%;
}

.svg-stage {
  position: relative;
  width: 100%;
  box-sizing: border-box;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: inset 0 0 0 1px var(--line);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.3s ease;
}

.svg-stage.bg-dark {
  background: #0b1329;
}

.svg-stage.bg-light {
  background: #f8fafc;
}

.svg-stage.bg-satellite {
  background: #061a14;
}

.svg-stage.bg-transparent {
  background: repeating-conic-gradient(#1e293b 0% 25%, #0f172a 0% 50%) 50% / 20px 20px;
}

.interactive-svg {
  width: 100%;
  height: 100%;
  display: block;
}

.stage-empty {
  color: var(--muted);
  font-size: 13px;
  font-weight: 600;
  padding: 40px;
  text-align: center;
}

.player-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 14px;
  color: var(--ink);
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  overflow: hidden;
}

.ctrl-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
  min-width: 32px;
  min-height: 32px;
  aspect-ratio: 1;
  border-radius: 50%;
  border: 0;
  cursor: pointer;
  box-sizing: border-box;
  transition: transform 0.1s ease;
}

.play-btn {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
}

.replay-btn {
  background: var(--surface);
  color: var(--ink);
  border: 1px solid var(--line);
}

.ctrl-btn:active {
  transform: scale(0.92);
}

.time-counter {
  font-size: 11px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--ink);
  white-space: nowrap;
  flex-shrink: 0;
}

.scrubber-slider {
  flex: 1;
  min-width: 40px;
  width: 0;
  accent-color: var(--p);
  cursor: pointer;
  height: 6px;
}

.speed-selector {
  display: flex;
  gap: 2px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 2px;
  flex-shrink: 0;
}

.speed-btn {
  padding: 2px 6px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  font-size: 10px;
  font-weight: 700;
  cursor: pointer;
}

.speed-btn.active {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
}

.loop-btn {
  border: 0;
  background: transparent;
  cursor: pointer;
  opacity: 0.55;
  color: var(--ink);
  transition: opacity 0.2s ease, color 0.2s ease;
  flex-shrink: 0;
}

.loop-btn.active {
  opacity: 1;
  color: var(--p);
  background: color-mix(in srgb, var(--p) 12%, var(--surface));
}

.options-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: visible;
  min-height: auto;
  padding: 2px 4px;
  box-sizing: border-box;
}

.opt-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.opt-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.pill-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 6px;
  padding: 2px;
  box-sizing: border-box;
}

.vehicle-grid {
  grid-template-columns: repeat(3, 1fr);
  padding: 2px;
  box-sizing: border-box;
}

.opt-pill-btn {
  padding: 6px 8px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  color: var(--ink);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  text-align: center;
  transition: all 0.15s ease;
  white-space: nowrap;
  box-sizing: border-box;
}

.opt-pill-btn:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--surface));
}

.opt-pill-btn.active {
  background: color-mix(in srgb, var(--p) 20%, var(--surface));
  border: 1px solid var(--p);
  color: var(--ink);
  outline: 2px solid var(--p);
  outline-offset: -1px;
}

.toggle-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 8px 10px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink);
  cursor: pointer;
}

.checkbox-label span {
  color: var(--ink);
}

.checkbox-label input {
  accent-color: var(--p);
  width: 16px;
  height: 16px;
}

.color-options {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 6px;
  padding: 2px;
  box-sizing: border-box;
}

.color-btn {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 6px 8px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
  color: var(--ink);
  transition: all 0.15s ease;
  box-sizing: border-box;
}

.color-btn:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--surface));
}

.color-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex-shrink: 0;
  box-shadow: 0 1px 3px rgba(0,0,0,0.3);
  border: 1px solid rgba(255, 255, 255, 0.4);
}

.color-btn.active {
  border: 1px solid var(--c-accent);
  outline: 2px solid var(--c-accent);
  outline-offset: -1px;
  background: color-mix(in srgb, var(--c-accent) 20%, var(--surface));
  color: var(--ink);
}

/* 底部固定操作栏 */
.animation-modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 20px;
  background: var(--surface);
  border-top: 1px solid var(--line);
  flex-shrink: 0;
}

.footer-status {
  flex: 1;
  min-width: 0;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.export-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 38px;
  padding: 0 14px;
  border-radius: 10px;
  border: 0;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.2s ease;
  white-space: nowrap;
}

.primary-btn {
  background: var(--p);
  color: var(--md-on-primary, #ffffff);
}

.primary-btn:hover {
  background: color-mix(in srgb, var(--p) 86%, #000);
}

.secondary-btn {
  background: var(--panel);
  color: var(--ink);
  border: 1px solid var(--line);
}

.secondary-btn:hover {
  background: color-mix(in srgb, var(--ink) 8%, var(--panel));
}

.video-btn {
  background: #ea580c;
  color: #ffffff;
}

.video-btn:hover:not(:disabled) {
  background: #c2410c;
}

.video-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.toast-tip {
  font-size: 11px;
  color: var(--p);
  font-weight: 600;
  line-height: 1.4;
  margin: 0;
  padding: 4px 8px;
  background: color-mix(in srgb, var(--p) 12%, var(--surface));
  border-radius: 6px;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes radarPulse {
  0% { transform: scale(0.6); opacity: 0.9; }
  100% { transform: scale(2.2); opacity: 0; }
}

.radar-ripple {
  transform-origin: center;
  animation: radarPulse 1.2s cubic-bezier(0.2, 0, 0, 1) infinite;
}

.pin-popup {
  transition: opacity 0.2s ease;
}

@media (max-width: 540px) {
  .animation-modal-backdrop {
    padding: 8px;
  }
  .animation-modal {
    max-height: calc(100vh - 16px);
    border-radius: 16px;
  }
  .animation-modal-footer {
    flex-direction: column;
    align-items: stretch;
    padding: 8px 14px;
  }
  .footer-actions {
    width: 100%;
  }
  .footer-actions .export-btn {
    flex: 1;
    padding: 0 6px;
    font-size: 11px;
  }
}
</style>