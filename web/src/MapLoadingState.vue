<template>
  <div class="map-loading-container" :class="[mode || 'trip']" role="status" aria-live="polite">
    <!-- 背景经纬网格与柔和光晕 -->
    <div class="map-loading-backdrop">
      <div class="map-loading-grid"></div>
      <div class="map-loading-glow"></div>
    </div>

    <!-- 核心微动效区域：路线轨迹 + 脉冲雷达 + 悬浮图钉 -->
    <div class="map-loading-graphic">
      <svg
        class="map-loading-svg"
        viewBox="0 0 160 140"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <!-- 渐变与滤镜定义 -->
          <linearGradient :id="gradId + '_path'" x1="20" y1="90" x2="145" y2="45" gradientUnits="userSpaceOnUse">
            <stop stop-color="#24695c" stop-opacity="0.15" />
            <stop offset="0.5" stop-color="#24695c" stop-opacity="0.9" />
            <stop offset="1" stop-color="#0e7490" stop-opacity="1" />
          </linearGradient>

          <linearGradient :id="gradId + '_pin'" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#ff8c72" />
            <stop offset="100%" stop-color="#e56a4d" />
          </linearGradient>

          <linearGradient :id="gradId + '_atlasPin'" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#38bdf8" />
            <stop offset="100%" stop-color="#0284c7" />
          </linearGradient>

          <radialGradient :id="gradId + '_groundShadow'" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stop-color="#092722" stop-opacity="0.25" />
            <stop offset="100%" stop-color="#092722" stop-opacity="0" />
          </radialGradient>

          <filter :id="gradId + '_pinShadow'" x="60" y="32" width="40" height="58" filterUnits="userSpaceOnUse">
            <feDropShadow dx="0" dy="3.5" stdDeviation="2.5" flood-color="#092722" flood-opacity="0.28" />
          </filter>
        </defs>

        <!-- 底层雷达脉冲光环 (围绕落点 x: 80, y: 102) -->
        <circle class="radar-pulse radar-pulse-1" cx="80" cy="102" r="12" />
        <circle class="radar-pulse radar-pulse-2" cx="80" cy="102" r="12" />

        <!-- 地面投影（与图钉浮动反向微缩） -->
        <ellipse class="ground-shadow" cx="80" cy="102" rx="13" ry="4" :fill="'url(#' + gradId + '_groundShadow)'" />

        <!-- 底层固定拓扑底线 -->
        <path
          class="trail-path-base"
          d="M 18 96 C 40 102, 52 80, 72 86 C 90 92, 102 60, 122 66 C 134 70, 142 56, 148 48"
          stroke="color-mix(in srgb, var(--p, #24695c) 16%, transparent)"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-dasharray="3 3"
        />

        <!-- 动态流淌穿梭的路线轨迹 -->
        <path
          class="trail-path-flow"
          d="M 18 96 C 40 102, 52 80, 72 86 C 90 92, 102 60, 122 66 C 134 70, 142 56, 148 48"
          :stroke="'url(#' + gradId + '_path)'"
          stroke-width="2.8"
          stroke-linecap="round"
          stroke-dasharray="10 14"
        />

        <!-- 路线上的发光规划点 -->
        <circle class="path-node node-1" cx="34" cy="98" r="3" fill="var(--p, #24695c)" />
        <circle class="path-node node-2" cx="72" cy="86" r="3.2" fill="var(--p, #24695c)" />
        <circle class="path-node node-3" cx="122" cy="66" r="3" fill="#0e7490" />

        <!-- 悬浮的旅行图钉 (轻盈上下浮动) -->
        <g class="floating-pin" :filter="'url(#' + gradId + '_pinShadow)'">
          <path
            d="M 80 48 C 72.5 48 66.5 54 66.5 61.5 C 66.5 71.5 80 84 80 84 C 80 84 93.5 71.5 93.5 61.5 C 93.5 54 87.5 48 80 48 Z"
            :fill="mode === 'atlas' ? 'url(#' + gradId + '_atlasPin)' : 'url(#' + gradId + '_pin)'"
          />
          <circle cx="80" cy="61" r="4.2" fill="#ffffff" />
          <circle cx="80" cy="61" r="1.8" :fill="mode === 'atlas' ? '#0284c7' : '#e56a4d'" />
        </g>
      </svg>
    </div>

    <!-- 文本与阶段指示 -->
    <div class="map-loading-content">
      <h3 class="map-loading-title">{{ title || '正在绘制旅途地图…' }}</h3>
      <p class="map-loading-subtitle">{{ subtitle || '正在解析地点坐标与路线拓扑' }}</p>
      <div v-if="hint" class="map-loading-badge">
        <span class="badge-dot"></span>
        <span>{{ hint }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  subtitle?: string
  hint?: string
  mode?: 'trip' | 'atlas'
}>()

// 独立 ID 保证 SVG defs 不冲突
const gradId = 'mlp_' + Math.random().toString(36).slice(2, 9)
</script>

<style scoped>
.map-loading-container {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: max(76px, env(safe-area-inset-top) + 64px) 24px 36px;
  overflow: hidden;
  user-select: none;
  pointer-events: none;
  animation: loadingFadeIn 0.3s ease-out;
}

@keyframes loadingFadeIn {
  from {
    opacity: 0;
    transform: scale(0.98);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.map-loading-backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background: radial-gradient(circle at 50% 46%, color-mix(in srgb, var(--p) 8%, transparent) 0%, transparent 60%),
              radial-gradient(circle at 50% 50%, color-mix(in srgb, var(--surface) 90%, var(--md-map)), color-mix(in srgb, var(--surface) 96%, var(--md-map)));
}

.map-loading-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(to right, color-mix(in srgb, var(--line) 40%, transparent) 1px, transparent 1px),
    linear-gradient(to bottom, color-mix(in srgb, var(--line) 40%, transparent) 1px, transparent 1px);
  background-size: 32px 32px;
  opacity: 0.32;
  mask-image: radial-gradient(circle at 50% 48%, black 25%, transparent 72%);
  -webkit-mask-image: radial-gradient(circle at 50% 48%, black 25%, transparent 72%);
}

.map-loading-graphic {
  position: relative;
  z-index: 1;
  width: 150px;
  height: 130px;
  margin-bottom: 12px;
}

.map-loading-svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}

/* 脉冲光环 */
.radar-pulse {
  fill: none;
  stroke: color-mix(in srgb, var(--p, #24695c) 40%, transparent);
  stroke-width: 1.5;
  transform-origin: 80px 102px;
  animation: radarPulse 2.4s cubic-bezier(0.15, 0.85, 0.25, 1) infinite;
}

.radar-pulse-2 {
  animation-delay: 1.2s;
}

@keyframes radarPulse {
  0% {
    r: 6px;
    opacity: 0.85;
    stroke-width: 2;
  }
  100% {
    r: 44px;
    opacity: 0;
    stroke-width: 0.5;
  }
}

/* 地面投影 */
.ground-shadow {
  transform-origin: 80px 102px;
  animation: shadowScale 2s ease-in-out infinite alternate;
}

@keyframes shadowScale {
  0% {
    transform: scale(1);
    opacity: 0.7;
  }
  100% {
    transform: scale(0.65);
    opacity: 0.25;
  }
}

/* 浮动图钉 */
.floating-pin {
  transform-origin: 80px 84px;
  animation: pinHover 2s cubic-bezier(0.45, 0, 0.55, 1) infinite alternate;
}

@keyframes pinHover {
  0% {
    transform: translateY(0);
  }
  100% {
    transform: translateY(-8px);
  }
}

/* 路线流动虚线 */
.trail-path-flow {
  animation: trailFlow 2s linear infinite;
}

@keyframes trailFlow {
  from {
    stroke-dashoffset: 48;
  }
  to {
    stroke-dashoffset: 0;
  }
}

/* 途径点发光点 */
.path-node {
  transform-origin: center;
  animation: nodePulse 2.4s ease-in-out infinite alternate;
}

.node-2 {
  animation-delay: 0.6s;
}

.node-3 {
  animation-delay: 1.2s;
}

@keyframes nodePulse {
  0% {
    opacity: 0.45;
    transform: scale(0.9);
  }
  100% {
    opacity: 1;
    transform: scale(1.2);
  }
}

/* 内容文字 */
.map-loading-content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  max-width: 320px;
}

.map-loading-title {
  margin: 0;
  color: var(--ink);
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: 1.35;
}

.map-loading-subtitle {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.45;
}

.map-loading-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--p) 24%, transparent);
  background: color-mix(in srgb, var(--p) 8%, var(--surface));
  color: var(--p);
  font-size: 11px;
  font-weight: 600;
  line-height: 16px;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--p) 10%, transparent);
}

.badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--p, #24695c);
  animation: dotPulse 1.8s ease-in-out infinite;
}

@keyframes dotPulse {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--p, #24695c) 60%, transparent);
  }
  70% {
    box-shadow: 0 0 0 5px transparent;
  }
  100% {
    box-shadow: 0 0 0 0 transparent;
  }
}

/* 桌面端避让左侧行程抽屉 */
@media (min-width: 901px) {
  .map-loading-container {
    padding: 40px 40px 40px min(430px, 35vw);
  }

  .atlas-workspace .map-loading-container {
    padding: 40px;
  }
}
</style>
