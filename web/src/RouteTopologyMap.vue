<script setup lang="ts">
import { computed } from 'vue'

type Coord = { lat: number; lng: number }
type LocationData = { preferred?: string; coordinates?: Record<string, Coord & { crs?: string }> }
type Stop = { id: string; sequence: number; title: string; location?: LocationData }
type Leg = { id: string; from_stop_id: string; to_stop_id: string; snapshots?: Array<{ geometry?: Array<[number, number]> | Array<Coord> }> }

const props = withDefaults(defineProps<{
  stops: Stop[]
  legs?: Leg[]
  theme?: 'light' | 'dark'
  width?: number
  height?: number
}>(), {
  theme: 'light',
  width: 560,
  height: 300,
  legs: () => [],
})

function getStopCoord(stop: Stop): [number, number] | null {
  const coords = stop.location?.coordinates
  if (!coords) return null
  const preferred = stop.location?.preferred
  const c = (preferred && coords[preferred]) || coords.gcj02 || coords.bd09ll || coords.wgs84 || Object.values(coords)[0]
  if (c && typeof c.lng === 'number' && typeof c.lat === 'number' && !Number.isNaN(c.lng) && !Number.isNaN(c.lat)) {
    return [c.lng, c.lat]
  }
  return null
}

const projectedData = computed(() => {
  const validStops: Array<{ stop: Stop; lng: number; lat: number; index: number }> = []
  props.stops.forEach((stop, idx) => {
    const pt = getStopCoord(stop)
    if (pt) {
      validStops.push({ stop, lng: pt[0], lat: pt[1], index: idx })
    }
  })

  if (validStops.length === 0) {
    return null
  }

  // 收集所有用于计算边界的点（包括 leg geometry）
  const allPts: Array<[number, number]> = validStops.map(s => [s.lng, s.lat])

  // 提取 leg geometry 中的坐标
  const legPathsCoords: Array<Array<[number, number]>> = []
  const stopMap = new Map(validStops.map(s => [s.stop.id, s]))

  if (props.legs && props.legs.length > 0) {
    props.legs.forEach(leg => {
      const snap = leg.snapshots?.[0]
      if (snap?.geometry && Array.isArray(snap.geometry) && snap.geometry.length > 1) {
        const pts: Array<[number, number]> = []
        snap.geometry.forEach(item => {
          if (Array.isArray(item) && typeof item[0] === 'number' && typeof item[1] === 'number') {
            pts.push([item[0], item[1]])
            allPts.push([item[0], item[1]])
          } else if (item && typeof (item as Coord).lng === 'number' && typeof (item as Coord).lat === 'number') {
            pts.push([(item as Coord).lng, (item as Coord).lat])
            allPts.push([(item as Coord).lng, (item as Coord).lat])
          }
        })
        if (pts.length > 1) {
          legPathsCoords.push(pts)
        }
      }
    })
  }

  let minLng = Infinity, maxLng = -Infinity, minLat = Infinity, maxLat = -Infinity
  for (const [lng, lat] of allPts) {
    if (lng < minLng) minLng = lng
    if (lng > maxLng) maxLng = lng
    if (lat < minLat) minLat = lat
    if (lat > maxLat) maxLat = lat
  }

  // 保护：如果只有一个点或所有点重合
  if (minLng === maxLng) { minLng -= 0.02; maxLng += 0.02 }
  if (minLat === maxLat) { minLat -= 0.015; maxLat += 0.015 }

  const midLat = (minLat + maxLat) / 2
  const cosLat = Math.cos((midLat * Math.PI) / 180)

  // 投影边界与画布 padding
  const paddingX = 48
  const paddingY = 44
  const innerW = props.width - paddingX * 2
  const innerH = props.height - paddingY * 2

  const spanLng = (maxLng - minLng) * cosLat
  const spanLat = maxLat - minLat

  // 保持宽高比
  const scale = Math.min(innerW / (spanLng || 0.001), innerH / (spanLat || 0.001))

  const project = (lng: number, lat: number): [number, number] => {
    const x = paddingX + (innerW - spanLng * scale) / 2 + (lng - minLng) * cosLat * scale
    const y = paddingY + (innerH - spanLat * scale) / 2 + (maxLat - lat) * scale
    return [Math.round(x * 10) / 10, Math.round(y * 10) / 10]
  }

  // 投影点
  const projectedStops = validStops.map(s => {
    const [x, y] = project(s.lng, s.lat)
    return {
      id: s.stop.id,
      title: s.stop.title,
      sequence: s.stop.sequence || (s.index + 1),
      displayIndex: s.index + 1,
      x,
      y,
      isFirst: s.index === 0,
      isLast: s.index === validStops.length - 1 && validStops.length > 1,
    }
  })

  // 投影路线
  const paths: string[] = []
  if (legPathsCoords.length > 0) {
    legPathsCoords.forEach(pts => {
      const projected = pts.map(p => project(p[0], p[1]))
      const d = projected.map((p, i) => (i === 0 ? `M ${p[0]} ${p[1]}` : `L ${p[0]} ${p[1]}`)).join(' ')
      paths.push(d)
    })
  } else {
    // 若无 geometry，直接连接相邻 stop
    for (let i = 0; i < projectedStops.length - 1; i++) {
      const p1 = projectedStops[i]
      const p2 = projectedStops[i + 1]
      // 优雅平滑连接
      const mx = (p1.x + p2.x) / 2
      const my = (p1.y + p2.y) / 2
      paths.push(`M ${p1.x} ${p1.y} Q ${mx} ${my - 6} ${p2.x} ${p2.y}`)
    }
  }

  return {
    stops: projectedStops,
    paths,
    totalCount: validStops.length,
  }
})
</script>

<template>
  <div class="route-topology" :class="[theme]">
    <svg
      :viewBox="`0 0 ${width} ${height}`"
      :width="width"
      :height="height"
      class="topology-svg"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <!-- 渐变色定义 -->
        <linearGradient id="routeGradientLight" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#00668b" />
          <stop offset="50%" stop-color="#006c4c" />
          <stop offset="100%" stop-color="#825500" />
        </linearGradient>
        <linearGradient id="routeGradientDark" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#7bd0ff" />
          <stop offset="50%" stop-color="#6cdba9" />
          <stop offset="100%" stop-color="#ffba38" />
        </linearGradient>
        <filter id="routeGlow" x="-20%" y="-20%" width="140%" height="140%">
          <feGaussianBlur stdDeviation="3" result="blur" />
          <feComposite in="SourceGraphic" in2="blur" operator="over" />
        </filter>
        <pattern id="gridPattern" width="28" height="28" patternUnits="userSpaceOnUse">
          <circle cx="2" cy="2" r="1" :fill="theme === 'dark' ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.05)'" />
        </pattern>
      </defs>

      <!-- 背景网格与边框 -->
      <rect width="100%" height="100%" rx="16" :fill="theme === 'dark' ? '#1b2024' : '#f0f4f8'" />
      <rect width="100%" height="100%" rx="16" fill="url(#gridPattern)" />

      <!-- 水印与方位标 -->
      <g transform="translate(24, 28)" class="north-mark" opacity="0.7">
        <circle cx="0" cy="0" r="11" fill="none" :stroke="theme === 'dark' ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.15)'" stroke-width="1.5" />
        <path d="M 0 -8 L 3 2 L 0 0 L -3 2 Z" :fill="theme === 'dark' ? '#7bd0ff' : '#00668b'" />
        <text x="0" y="7" text-anchor="middle" font-size="8" font-weight="700" :fill="theme === 'dark' ? '#94a3b8' : '#64748b'">N</text>
      </g>
      <text
        :x="width - 24"
        y="30"
        text-anchor="end"
        font-size="11"
        font-weight="600"
        letter-spacing="0.5"
        :fill="theme === 'dark' ? '#94a3b8' : '#64748b'"
      >
        ROUTE TOPOLOGY
      </text>

      <template v-if="projectedData">
        <!-- 轨迹路径 (发光层) -->
        <g class="path-glow-group">
          <path
            v-for="(p, i) in projectedData.paths"
            :key="`glow-${i}`"
            :d="p"
            fill="none"
            :stroke="theme === 'dark' ? 'url(#routeGradientDark)' : 'url(#routeGradientLight)'"
            stroke-width="7"
            stroke-linecap="round"
            stroke-linejoin="round"
            opacity="0.25"
          />
        </g>

        <!-- 轨迹路径 (实体主线) -->
        <g class="path-main-group">
          <path
            v-for="(p, i) in projectedData.paths"
            :key="`main-${i}`"
            :d="p"
            fill="none"
            :stroke="theme === 'dark' ? 'url(#routeGradientDark)' : 'url(#routeGradientLight)'"
            stroke-width="3"
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-dasharray="none"
          />
        </g>

        <!-- 站点 Marker 与 标签 -->
        <g v-for="s in projectedData.stops" :key="s.id" class="stop-node">
          <!-- 节点外光圈 (起终点加大) -->
          <circle
            :cx="s.x"
            :cy="s.y"
            :r="s.isFirst || s.isLast ? 13 : 10"
            :fill="s.isFirst ? (theme === 'dark' ? '#22c55e' : '#16a34a') : s.isLast ? (theme === 'dark' ? '#f59e0b' : '#d97706') : (theme === 'dark' ? '#334155' : '#e2e8f0')"
            :stroke="theme === 'dark' ? '#0f172a' : '#ffffff'"
            stroke-width="2.5"
          />

          <!-- 节点序号或文字 -->
          <text
            :x="s.x"
            :y="s.y + 3.5"
            text-anchor="middle"
            font-size="9"
            font-weight="700"
            :fill="s.isFirst || s.isLast ? '#ffffff' : (theme === 'dark' ? '#f1f5f9' : '#1e293b')"
          >
            {{ s.isFirst ? '起' : s.isLast ? '终' : s.displayIndex }}
          </text>

          <!-- 站点地名标签 -->
          <g :transform="`translate(${s.x}, ${s.y + 19})`">
            <rect
              :x="-(Math.min(s.title.length * 6 + 10, 60))"
              y="-10"
              :width="Math.min(s.title.length * 12 + 20, 120)"
              height="18"
              rx="9"
              :fill="theme === 'dark' ? 'rgba(15, 23, 42, 0.85)' : 'rgba(255, 255, 255, 0.92)'"
              :stroke="theme === 'dark' ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.08)'"
              stroke-width="1"
            />
            <text
              x="0"
              y="2.5"
              text-anchor="middle"
              font-size="10"
              font-weight="500"
              :fill="theme === 'dark' ? '#e2e8f0' : '#1e293b'"
            >
              {{ s.title.length > 7 ? s.title.slice(0, 6) + '…' : s.title }}
            </text>
          </g>
        </g>
      </template>

      <!-- 空数据状态 -->
      <g v-else transform="translate(280, 150)">
        <text text-anchor="middle" font-size="13" :fill="theme === 'dark' ? '#64748b' : '#94a3b8'">
          暂无可绘制的地点坐标
        </text>
      </g>
    </svg>
  </div>
</template>

<style scoped>
.route-topology {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
  border-radius: 16px;
  overflow: hidden;
}
.topology-svg {
  width: 100%;
  height: auto;
  display: block;
  user-select: none;
}
</style>
