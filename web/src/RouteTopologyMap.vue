<script setup lang="ts">
import { computed } from 'vue'
import { clusterPosterPoints, layoutPosterLabels, selectPosterRoutes, type PosterPoint, type PosterRouteLeg } from './routeTopologyLayout'

type Coord = { lat: number; lng: number; crs?: string }
type LocationData = { coordinates?: Record<string, Coord> }
type Stop = { id: string; sequence: number; title: string; dayBoundary?: boolean; location?: LocationData }
type Leg = PosterRouteLeg & { id: string }

const props = withDefaults(defineProps<{
  stops: Stop[]
  legs?: Leg[]
  theme?: 'light' | 'dark'
  labelDensity?: 'simple' | 'detailed'
  routeProvider?: string
  routeMode?: string
  width?: number
  height?: number
}>(), {
  theme: 'light', labelDensity: 'simple', routeProvider: '', routeMode: '', width: 560, height: 300, legs: () => [],
})

const coordinateSystems = ['gcj02', 'bd09ll', 'wgs84'] as const
function getStopCoord(stop: Stop, crs: string): [number, number] | null {
  const point = stop.location?.coordinates?.[crs]
  if (!point || point.crs && point.crs !== crs || !Number.isFinite(point.lng) || !Number.isFinite(point.lat) || Math.abs(point.lng) > 180 || Math.abs(point.lat) > 90) return null
  return [point.lng, point.lat]
}

const projectedData = computed(() => {
  const route = selectPosterRoutes(props.legs, props.routeProvider, props.routeMode)
  const stopCRS = coordinateSystems.reduce<string>((best, candidate) =>
    props.stops.filter(stop => getStopCoord(stop, candidate)).length > props.stops.filter(stop => getStopCoord(stop, best)).length ? candidate : best, coordinateSystems[0])
  const routeStopCount = route.crs ? props.stops.filter(stop => getStopCoord(stop, route.crs)).length : 0
  const stopCount = props.stops.filter(stop => getStopCoord(stop, stopCRS)).length
  const crs = route.crs && routeStopCount >= stopCount ? route.crs : stopCRS
  const routeCoords = route.crs === crs ? route.paths : []
  const validStops = props.stops.flatMap((stop, index) => {
    const coord = getStopCoord(stop, crs)
    return coord ? [{ stop, index, coord }] : []
  })
  if (!validStops.length) return null
  const allCoords: Array<[number, number]> = [...validStops.map(item => item.coord), ...routeCoords.flat()]
  let minLng = Infinity, maxLng = -Infinity, minLat = Infinity, maxLat = -Infinity
  for (const [lng, lat] of allCoords) { minLng = Math.min(minLng, lng); maxLng = Math.max(maxLng, lng); minLat = Math.min(minLat, lat); maxLat = Math.max(maxLat, lat) }
  if (minLng === maxLng) { minLng -= .02; maxLng += .02 }
  if (minLat === maxLat) { minLat -= .015; maxLat += .015 }
  const cosLat = Math.max(.01, Math.cos((minLat + maxLat) * Math.PI / 360))
  const innerW = Math.max(1, props.width - 96), innerH = Math.max(1, props.height - 96)
  const spanLng = (maxLng - minLng) * cosLat, spanLat = maxLat - minLat
  const scale = Math.min(innerW / spanLng, innerH / spanLat)
  const project = ([lng, lat]: [number, number]): [number, number] => [
    Math.round((48 + (innerW - spanLng * scale) / 2 + (lng - minLng) * cosLat * scale) * 10) / 10,
    Math.round((44 + (innerH - spanLat * scale) / 2 + (maxLat - lat) * scale) * 10) / 10,
  ]
  const points: PosterPoint[] = validStops.map(({ stop, index, coord }, visibleIndex) => {
    const [x, y] = project(coord)
    return { id: stop.id, title: stop.title, displayIndex: index + 1, x, y, isFirst: visibleIndex === 0, isLast: visibleIndex === validStops.length - 1 && validStops.length > 1, dayBoundary: stop.dayBoundary }
  })
  const paths = routeCoords.map(coords => coords.map(project))
  const routePixels = paths.flatMap(path => path.filter((_, i) => i % Math.max(1, Math.ceil(path.length / 120)) === 0))
  const nodes = clusterPosterPoints(points)
  const labels = layoutPosterLabels(nodes, props.width, props.height, props.labelDensity, routePixels)
  const routeMessage = !paths.length ? '路线未生成 · 仅显示规划点' : paths.length < props.legs.length ? '仅显示已生成的路线段' : ''
  return { nodes, labels, paths: paths.map(path => path.map(([x, y], i) => (i ? 'L' : 'M') + ' ' + x + ' ' + y).join(' ')), routeMessage, omittedCount: props.stops.length - points.length, totalCount: points.length, coordinateSystem: crs }
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

        <!-- 真实坐标聚合节点：密集区域合并编号，路线几何保持原样 -->
        <g v-for="node in projectedData.nodes" :key="node.id" class="stop-node">
          <circle :cx="node.x" :cy="node.y" :r="node.isFirst || node.isLast ? 12 : 10"
            :fill="node.isFirst ? (theme === 'dark' ? '#22c55e' : '#16a34a') : node.isLast ? (theme === 'dark' ? '#f59e0b' : '#d97706') : (theme === 'dark' ? '#334155' : '#e2e8f0')"
            :stroke="theme === 'dark' ? '#0f172a' : '#ffffff'" stroke-width="2.5" />
          <text :x="node.x" :y="node.y + 3.5" text-anchor="middle" font-size="9" font-weight="700"
            :fill="node.isFirst || node.isLast ? '#ffffff' : (theme === 'dark' ? '#f1f5f9' : '#1e293b')">
            {{ node.count > 1 ? '+' + node.count : node.isFirst ? '起' : node.isLast ? '终' : node.displayIndex }}
          </text>
        </g>

        <!-- 仅显示有空间的地名，优先起终点和每天首点 -->
        <g v-for="label in projectedData.labels" :key="'label-' + label.id" class="topology-name-label">
          <rect :x="label.x" :y="label.y" :width="label.width" :height="label.height" rx="9"
            :fill="theme === 'dark' ? 'rgba(15, 23, 42, 0.88)' : 'rgba(255, 255, 255, 0.94)'"
            :stroke="theme === 'dark' ? 'rgba(255, 255, 255, 0.17)' : 'rgba(0, 0, 0, 0.12)'" stroke-width="1" />
          <text :x="label.x + label.width / 2" :y="label.y + 13" text-anchor="middle" font-size="10" font-weight="600"
            :fill="theme === 'dark' ? '#e2e8f0' : '#1e293b'">{{ label.text }}</text>
        </g>
        <text v-if="projectedData.routeMessage" x="24" :y="height - 14" font-size="10" font-weight="700"
          :fill="theme === 'dark' ? '#cbd5e1' : '#475569'">{{ projectedData.routeMessage }}</text>
        <text v-if="projectedData.omittedCount" :x="width - 24" :y="height - 14" text-anchor="end" font-size="10"
          :fill="theme === 'dark' ? '#cbd5e1' : '#475569'">{{ projectedData.omittedCount }} 点无同坐标系坐标</text>
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
