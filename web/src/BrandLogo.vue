<template>
  <div class="brand-logo" :class="[variant, shape, themeClass]" :style="rootStyle">
    <!-- 图形 Mark 部分 -->
    <div v-if="variant !== 'text'" class="logo-mark" :style="markStyle">
      <svg
        v-if="shape === 'squircle'"
        viewBox="0 0 128 128"
        width="100%"
        height="100%"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <linearGradient :id="gradId + '_bg'" x1="16" y1="16" x2="112" y2="112" gradientUnits="userSpaceOnUse">
            <stop stop-color="var(--jin-logo-bg-start, #24695C)" />
            <stop offset="1" stop-color="var(--jin-logo-bg-end, #16463D)" />
          </linearGradient>
          <linearGradient :id="gradId + '_path'" x1="36" y1="52" x2="84" y2="88" gradientUnits="userSpaceOnUse">
            <stop stop-color="#A7E8D2" />
            <stop offset="1" stop-color="#56C9A3" />
          </linearGradient>
          <linearGradient :id="gradId + '_pin'" x1="68" y1="20" x2="104" y2="58" gradientUnits="userSpaceOnUse">
            <stop stop-color="#FF7F62" />
            <stop offset="1" stop-color="#E56A4D" />
          </linearGradient>
          <filter :id="gradId + '_shadow'" x="62" y="18" width="48" height="56" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
            <feDropShadow dx="0" dy="2.5" stdDeviation="3" flood-opacity="0.28" flood-color="#0A221E"/>
          </filter>
        </defs>
        <!-- 背景圆角矩形 -->
        <rect width="128" height="128" rx="36" :fill="'url(#' + gradId + '_bg)'" />
        <!-- J 路线轨迹 -->
        <path
          d="M84 46 C84 74 76 90 55 90 C41 90 33 81 33 70 C33 60 40 54 48 54 C54 54 58 58 58 64"
          :stroke="'url(#' + gradId + '_path)'"
          stroke-width="11"
          stroke-linecap="round"
          stroke-linejoin="round"
          fill="none"
        />
        <!-- 途经点 -->
        <circle cx="48" cy="64" r="3.5" fill="#FFFFFF" />
        <!-- 目的地定位图钉 -->
        <g :filter="'url(#' + gradId + '_shadow)'">
          <path
            d="M86 21 C76.06 21 68 29.06 68 39 C68 51.5 86 67 86 67 C86 67 104 51.5 104 39 C104 29.06 95.94 21 86 21 Z"
            :fill="'url(#' + gradId + '_pin)'"
          />
          <circle cx="86" cy="38" r="6" fill="#FFFFFF" />
        </g>
      </svg>

      <!-- 纯路径版本 (适用于无需背景底板的场景) -->
      <svg
        v-else
        viewBox="0 0 100 100"
        width="100%"
        height="100%"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <linearGradient :id="gradId + '_plain_path'" x1="20" y1="40" x2="70" y2="85" gradientUnits="userSpaceOnUse">
            <stop stop-color="var(--p, #24695c)" />
            <stop offset="1" stop-color="var(--accent, #e56a4d)" />
          </linearGradient>
        </defs>
        <!-- 路线轨迹 -->
        <path
          d="M70 42 C70 66 64 80 44 80 C32 80 25 72 25 62 C25 53 31 48 38 48 C43 48 46 51 46 56"
          stroke="currentColor"
          stroke-width="9"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
        <circle cx="38" cy="56" r="3" fill="var(--accent, #e56a4d)" />
        <!-- 目的地定位图钉 -->
        <path
          d="M72 16 C63.16 16 56 23.16 56 32 C56 43 72 58 72 58 C72 58 88 43 88 32 C88 23.16 80.84 16 72 16 Z"
          fill="var(--accent, #e56a4d)"
        />
        <circle cx="72" cy="31" r="5" fill="#FFFFFF" />
      </svg>
    </div>

    <!-- 字标部分 -->
    <div v-if="variant === 'full' || variant === 'text'" class="logo-text">
      <span class="text-journey">Journey</span><span class="text-in">In</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: number | string
    variant?: 'mark' | 'full' | 'text'
    shape?: 'squircle' | 'plain'
    themeClass?: string
  }>(),
  {
    size: 32,
    variant: 'mark',
    shape: 'squircle',
    themeClass: ''
  }
)

// 生成独立 ID 防止同页面多个渐变滤镜 ID 碰撞
const gradId = 'jin_' + Math.random().toString(36).slice(2, 9)

const parsedSize = computed(() => {
  return typeof props.size === 'number' ? `${props.size}px` : props.size
})

const rootStyle = computed(() => {
  if (props.variant === 'mark') {
    return {
      width: parsedSize.value,
      height: parsedSize.value
    }
  }
  return {
    '--logo-height': parsedSize.value
  }
})

const markStyle = computed(() => {
  return {
    width: parsedSize.value,
    height: parsedSize.value
  }
})
</script>

<style scoped>
.brand-logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  vertical-align: middle;
  line-height: 1;
  user-select: none;
}

.logo-mark {
  flex-shrink: 0;
  display: grid;
  place-items: center;
}

.logo-mark svg {
  display: block;
}

.logo-text {
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: calc(var(--logo-height, 32px) * 0.65);
  font-weight: 800;
  letter-spacing: -0.03em;
  white-space: nowrap;
}

.text-journey {
  color: var(--ink, #1d2b2a);
}

.text-in {
  color: var(--accent, #e56a4d);
  margin-left: 0.5px;
}

/* 适配暗色主题微调 */
:root[data-theme="dark"] .brand-logo {
  --jin-logo-bg-start: #24695C;
  --jin-logo-bg-end: #12332D;
}
</style>
