<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { IonIcon } from '@ionic/vue'
import {
  addOutline,
  chevronDownOutline,
  closeOutline,
  compassOutline,
  ellipsisHorizontalOutline,
  layersOutline,
  mapOutline,
  moonOutline,
  navigateOutline,
  searchOutline,
  settingsOutline,
  sunnyOutline,
  timeOutline,
} from 'ionicons/icons'

type PrototypeScreen = 'list' | 'peek' | 'half' | 'stop' | 'desktop'

const validScreens: PrototypeScreen[] = ['list', 'peek', 'half', 'stop', 'desktop']
const hashScreen = window.location.hash.slice(1) as PrototypeScreen
const initialScreen = validScreens.includes(hashScreen)
  ? hashScreen
  : window.matchMedia('(max-width: 720px)').matches ? 'list' : 'desktop'

const screen = ref<PrototypeScreen>(initialScreen)
const toolsOpen = ref(false)
const darkMode = ref(false)
const activeStopID = ref('stop-1')
const activeChild = ref(false)
const desktopDetailOpen = ref(true)

if (!window.history.state?.prototype) {
  const initialURL = new URL(window.location.href)
  initialURL.searchParams.set('prototype', '1')
  initialURL.hash = initialScreen
  window.history.replaceState({ prototype: true, screen: initialScreen, depth: 0 }, '', initialURL.toString())
}

const screens: Array<{ id: PrototypeScreen; number: string; label: string; description: string }> = [
  { id: 'list', number: '01', label: '行程列表', description: '手机端全屏列表' },
  { id: 'peek', number: '02', label: '地图 Peek', description: '地图 + 收起 Sheet' },
  { id: 'half', number: '03', label: '行程 Half', description: '地图 + 时间线' },
  { id: 'stop', number: '04', label: '规划点详情', description: '单一 Sheet 详情' },
  { id: 'desktop', number: '05', label: '桌面工作区', description: 'Rail + 地图 + 详情' },
]

const stops = [
  { id: 'stop-1', number: '01', title: '西湖断桥', shortTitle: '断桥', time: '09:00–10:30', address: '杭州市西湖区北山街', type: '景点', duration: '1 小时 30 分', left: '28%', top: '38%' },
  { id: 'stop-2', number: '02', title: '苏堤春晓', shortTitle: '苏堤', time: '10:50–12:00', address: '西湖景区苏堤', type: '漫步', duration: '1 小时 10 分', left: '52%', top: '53%' },
  { id: 'stop-3', number: '03', title: '曲院风荷', shortTitle: '曲院', time: '14:00–15:30', address: '西湖区杨公堤', type: '景点', duration: '1 小时 30 分', left: '73%', top: '34%' },
  { id: 'stop-4', number: '04', title: '湖畔咖啡', shortTitle: '咖啡', time: '16:00–17:00', address: '孤山路 8 号', type: '餐饮', duration: '1 小时', left: '67%', top: '68%' },
]

const selectedStop = computed(() => stops.find(stop => stop.id === activeStopID.value) || stops[0])
const activeScreen = computed(() => screens.find(item => item.id === screen.value) || screens[0])

type PrototypeHistoryState = { prototype?: boolean; screen?: PrototypeScreen; depth?: number }

function updatePrototypeURL(next: PrototypeScreen, mode: 'push' | 'replace' = 'push') {
  const url = new URL(window.location.href)
  url.searchParams.set('prototype', '1')
  url.hash = next
  const current = window.history.state as PrototypeHistoryState | null
  const depth = Math.max(0, current?.depth || 0) + (mode === 'push' ? 1 : 0)
  const state: PrototypeHistoryState = { prototype: true, screen: next, depth }
  if (mode === 'push') window.history.pushState(state, '', url.toString())
  else window.history.replaceState(state, '', url.toString())
}

function setScreen(next: PrototypeScreen, mode: 'push' | 'replace' = 'push') {
  screen.value = next
  toolsOpen.value = false
  activeChild.value = false
  updatePrototypeURL(next, mode)
}

function applyHistoryState() {
  const state = window.history.state as PrototypeHistoryState | null
  if (state?.prototype && state.screen && validScreens.includes(state.screen)) {
    screen.value = state.screen
    toolsOpen.value = false
    activeChild.value = false
    return
  }
  const next = window.location.hash.slice(1) as PrototypeScreen
  if (validScreens.includes(next)) screen.value = next
}

function goBack() {
  const state = window.history.state as PrototypeHistoryState | null
  if (state?.prototype && (state.depth || 0) > 0) window.history.back()
  else if (screen.value !== 'list') setScreen('list', 'replace')
}

onMounted(() => window.addEventListener('popstate', applyHistoryState))
onUnmounted(() => window.removeEventListener('popstate', applyHistoryState))

function selectStop(id: string) {
  activeStopID.value = id
  if (screen.value !== 'desktop') setScreen('stop')
}

function toggleMapTools() {
  toolsOpen.value = !toolsOpen.value
}
</script>

<template>
  <main class="prototype-shell" :class="{ 'prototype-dark': darkMode }">
    <header class="prototype-toolbar">
      <div class="prototype-brand" aria-label="JourneyIn 原型预览">
        <span class="prototype-brand-mark">✦</span>
        <strong>JourneyIn</strong>
        <span class="prototype-caption">Trailglass UI Prototype</span>
      </div>

      <nav class="prototype-tabs" aria-label="原型页面切换" role="tablist">
        <button
          v-for="item in screens"
          :key="item.id"
          type="button"
          role="tab"
          :aria-selected="screen === item.id"
          :class="{ active: screen === item.id }"
          @click="setScreen(item.id)"
        >
          <span>{{ item.number }}</span>{{ item.label }}
        </button>
      </nav>

      <div class="prototype-toolbar-actions">
        <span class="prototype-review-note">{{ activeScreen.description }}</span>
        <button type="button" class="prototype-icon-button" aria-label="切换原型深色模式" @click="darkMode = !darkMode">
          <IonIcon :icon="moonOutline" />
        </button>
        <a class="prototype-exit" href="./" aria-label="退出原型预览">退出预览</a>
      </div>
    </header>

    <section class="prototype-stage">
      <div v-if="screen === 'desktop'" class="desktop-prototype" aria-label="桌面端工作区原型">
        <aside class="desktop-rail">
          <div class="rail-logo" aria-label="JourneyIn">✦</div>
          <button class="rail-item selected" type="button"><IonIcon :icon="compassOutline" /><span>行程</span></button>
          <button class="rail-item" type="button"><IonIcon :icon="mapOutline" /><span>地图</span></button>
          <button class="rail-item" type="button"><IonIcon :icon="settingsOutline" /><span>设置</span></button>
          <div class="rail-spacer"></div>
          <button class="rail-item rail-muted" type="button"><IonIcon :icon="moonOutline" /><span>主题</span></button>
        </aside>

        <aside class="desktop-itinerary">
          <header class="desktop-panel-header">
            <div>
              <span class="section-kicker">当前行程</span>
              <h1>杭州周末行</h1>
              <p>4 月 18 日 — 4 月 19 日 · 2 天</p>
            </div>
            <button class="round-icon" type="button" aria-label="行程更多操作"><IonIcon :icon="ellipsisHorizontalOutline" /></button>
          </header>

          <div class="desktop-panel-actions">
            <button class="secondary-button" type="button"><IonIcon :icon="searchOutline" /> 添加地点</button>
            <button class="primary-button" type="button"><IonIcon :icon="addOutline" /> 新建</button>
          </div>

          <div class="desktop-day-tabs" role="tablist" aria-label="日期筛选">
            <button class="active" type="button">全程</button>
            <button type="button">D1 · 4/18</button>
            <button type="button">D2 · 4/19</button>
          </div>

          <div class="desktop-route-card">
            <div>
              <span>全程路线</span>
              <strong>6.2 km <i>·</i> 2 小时 10 分</strong>
            </div>
            <small><IonIcon :icon="navigateOutline" /> 高德 · 步行</small>
          </div>

          <div class="desktop-timeline" aria-label="规划点时间线">
            <button
              v-for="stop in stops"
              :key="stop.id"
              type="button"
              class="desktop-stop-row"
              :class="{ selected: activeStopID === stop.id }"
              @click="activeStopID = stop.id; desktopDetailOpen = true"
            >
              <span class="timeline-dot">{{ stop.number }}</span>
              <span class="timeline-copy">
                <strong>{{ stop.title }}</strong>
                <small>{{ stop.time }} · {{ stop.address }}</small>
              </span>
              <span class="row-chevron">›</span>
            </button>
          </div>

          <button class="desktop-add-row" type="button"><IonIcon :icon="addOutline" /> 搜索并添加规划点</button>
        </aside>

        <section class="desktop-map-pane">
          <div class="desktop-map-topbar">
            <div class="desktop-map-title"><span class="live-dot"></span><span>地图工作区</span><small>地图是主内容 · 规划点与路线已同步</small></div>
            <div class="desktop-map-tools">
              <button type="button" class="map-tool-button active"><IonIcon :icon="layersOutline" /> 高德</button>
              <button type="button" class="map-tool-button"><IonIcon :icon="sunnyOutline" /> 标准图</button>
              <button type="button" class="map-tool-button"><IonIcon :icon="ellipsisHorizontalOutline" /> 更多</button>
            </div>
          </div>

          <div class="mock-map desktop-map" aria-label="模拟西湖地图">
            <div class="map-water water-main"></div>
            <div class="map-water water-small"></div>
            <div class="map-block block-a"></div>
            <div class="map-block block-b"></div>
            <div class="map-block block-c"></div>
            <div class="map-road road-one"></div>
            <div class="map-road road-two"></div>
            <div class="map-road road-three"></div>
            <div class="map-road road-four"></div>
            <svg class="route-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
              <polyline points="28,38 52,53 73,34 67,68" />
              <polyline class="route-shadow" points="28,38 52,53 73,34 67,68" />
            </svg>
            <button
              v-for="stop in stops"
              :key="stop.id"
              type="button"
              class="map-pin"
              :class="{ selected: activeStopID === stop.id }"
              :style="{ left: stop.left, top: stop.top }"
              :aria-label="'查看 ' + stop.title"
              @click="activeStopID = stop.id; desktopDetailOpen = true"
            >
              <span>{{ stop.number }}</span>
            </button>
            <div class="map-compass"><IonIcon :icon="compassOutline" /></div>
            <div class="map-scale">500 m</div>
            <div class="desktop-map-status"><span class="status-mark">✓</span><span>高德 · 4 个规划点 · 路线已缓存</span><button type="button">查看路线设置</button></div>
          </div>

          <aside v-if="desktopDetailOpen" class="desktop-detail-drawer" aria-label="规划点详情">
            <div class="drawer-header">
              <div>
                <span class="section-kicker">规划点 {{ selectedStop.number }}</span>
                <h2>{{ selectedStop.title }}</h2>
              </div>
              <button class="round-icon" type="button" aria-label="关闭详情" @click="desktopDetailOpen = false"><IonIcon :icon="closeOutline" /></button>
            </div>
            <p class="drawer-address">{{ selectedStop.address }}</p>
            <div class="drawer-meta"><span><IonIcon :icon="timeOutline" /> {{ selectedStop.time }}</span><span><IonIcon :icon="sunnyOutline" /> 晴 · 22°C</span></div>
            <div class="drawer-actions"><button class="primary-button" type="button"><IonIcon :icon="navigateOutline" /> 开始导航</button><button class="secondary-button" type="button">编辑</button></div>
            <section class="drawer-section"><h3>地点说明</h3><p>适合清晨拍照，沿北山街慢慢走到湖边。这里保留足够的地图上下文，不需要离开当前工作区。</p></section>
            <section class="drawer-section"><h3>子规划点 <span>2 个</span></h3><button class="child-row" type="button"><b>断桥残雪观景位</b><small>09:20 · 观景</small><span>›</span></button><button class="child-row" type="button"><b>湖畔咖啡</b><small>10:00 · 餐饮</small><span>›</span></button></section>
          </aside>
        </section>
      </div>

      <div v-else-if="screen === 'list'" class="mobile-viewport list-viewport" aria-label="手机端行程列表原型">
        <div class="mobile-list-screen">
          <header class="mobile-list-header">
            <div class="mobile-inline-brand"><span class="prototype-brand-mark">✦</span><span>JourneyIn</span><span class="preview-chip">原型</span></div>
            <div class="list-heading-row"><div><span class="section-kicker">旅行工作台</span><h1>你的行程</h1><p>把下一段旅程放在地图上</p></div><div class="list-header-actions"><button type="button" aria-label="搜索行程"><IonIcon :icon="searchOutline" /></button><button type="button" aria-label="打开设置"><IonIcon :icon="settingsOutline" /></button></div></div>
          </header>
          <div class="list-filter-row"><span>全部行程 <b>3</b></span><button type="button">最近更新 <IonIcon :icon="chevronDownOutline" /></button></div>
          <div class="mobile-trip-cards">
            <button class="mobile-trip-card" type="button" @click="setScreen('peek')">
              <span class="prototype-trip-visual prototype-trip-visual-0"><span>杭</span></span><span class="trip-card-content"><strong>杭州周末行</strong><span class="trip-dates">4 月 18 日 — 4 月 19 日</span><span class="trip-stats"><b>2</b> 天 <i>·</i> <b>6</b> 个规划点 <i>·</i> 2 个路线快照</span></span><span class="trip-card-arrow">›</span>
            </button>
            <button class="mobile-trip-card" type="button" @click="setScreen('peek')">
              <span class="prototype-trip-visual prototype-trip-visual-1"><span>甘</span></span><span class="trip-card-content"><strong>甘南自驾</strong><span class="trip-dates">5 月 1 日 — 5 月 6 日</span><span class="trip-stats"><b>6</b> 天 <i>·</i> <b>18</b> 个规划点</span></span><span class="trip-card-arrow">›</span>
            </button>
            <button class="mobile-trip-card" type="button" @click="setScreen('peek')">
              <span class="prototype-trip-visual prototype-trip-visual-2"><span>厦</span></span><span class="trip-card-content"><strong>厦门慢游</strong><span class="trip-dates">6 月 12 日 — 6 月 15 日</span><span class="trip-stats"><b>4</b> 天 <i>·</i> <b>11</b> 个规划点</span></span><span class="trip-card-arrow">›</span>
            </button>
          </div>
          <p class="mobile-list-hint"><span>↗</span> 点击整张卡片进入地图工作区；更多操作集中在卡片菜单中。</p>
          <button class="mobile-fab" type="button" @click="setScreen('peek')"><IonIcon :icon="addOutline" /><span>新建行程</span></button>
        </div>
      </div>

      <div v-else class="mobile-viewport map-viewport" :aria-label="activeScreen.description">
        <div class="mock-map mobile-map">
          <div class="map-water water-main"></div>
          <div class="map-water water-small"></div>
          <div class="map-block block-a"></div>
          <div class="map-block block-b"></div>
          <div class="map-block block-c"></div>
          <div class="map-road road-one"></div>
          <div class="map-road road-two"></div>
          <div class="map-road road-three"></div>
          <div class="map-road road-four"></div>
          <svg class="route-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            <polyline points="28,38 52,53 73,34 67,68" />
            <polyline class="route-shadow" points="28,38 52,53 73,34 67,68" />
          </svg>
          <button
            v-for="stop in stops"
            :key="stop.id"
            type="button"
            class="map-pin"
            :class="{ selected: activeStopID === stop.id }"
            :style="{ left: stop.left, top: stop.top }"
            :aria-label="'查看 ' + stop.title"
            @click="selectStop(stop.id)"
          ><span>{{ stop.number }}</span></button>
          <div class="map-label label-west">西湖</div><div class="map-label label-north">北山街</div><div class="map-label label-south">苏堤</div>

          <header class="mobile-map-context">
            <button class="mobile-back-button" type="button" aria-label="返回行程列表" @click="goBack"><span>‹</span><small>行程</small></button>
            <div class="mobile-context-copy"><strong>杭州周末行</strong><span>4 月 18 日 · D1 西湖周边</span></div>
            <button class="mobile-map-menu" type="button" aria-label="打开地图选项" :aria-expanded="toolsOpen" @click="toggleMapTools"><IonIcon :icon="ellipsisHorizontalOutline" /></button>
          </header>

          <div class="mobile-map-top-status"><span class="status-mark">✓</span><span>高德 · 4 个点</span></div>
          <div class="map-compass"><IonIcon :icon="compassOutline" /></div>

          <div v-if="toolsOpen" class="mobile-map-options" role="dialog" aria-label="地图选项">
            <div class="options-heading"><strong>地图选项</strong><button type="button" aria-label="关闭地图选项" @click="toolsOpen = false"><IonIcon :icon="closeOutline" /></button></div>
            <div class="options-row"><span>底图</span><div class="option-segment"><button class="active" type="button">高德</button><button type="button">百度</button></div></div>
            <div class="options-row"><span>图层</span><div class="option-segment"><button class="active" type="button">标准图</button><button type="button">卫星图</button></div></div>
            <button class="option-action" type="button"><IonIcon :icon="mapOutline" /> 地图选点</button>
          </div>
        </div>

        <article v-if="screen === 'peek'" class="mobile-sheet peek-sheet">
          <button class="sheet-handle" type="button" aria-label="展开行程" @click="setScreen('half', 'replace')"><span></span></button>
          <div class="sheet-peek-header"><div><span class="sheet-eyebrow">D1 · 西湖周边</span><h1>4 个规划点</h1></div><button class="sheet-expand-button" type="button" @click="setScreen('half', 'replace')">查看行程 <span>↑</span></button></div>
          <div class="sheet-metrics"><span><b>6.2 km</b><small>路线距离</small></span><span><b>2h 10m</b><small>预计用时</small></span><span><b>晴 · 22°</b><small>天气快照</small></span></div>
          <p class="sheet-peek-hint">上拉查看时间线 · 地图仍可操作</p>
        </article>

        <article v-else-if="screen === 'half'" class="mobile-sheet half-sheet">
          <button class="sheet-handle" type="button" aria-label="收起行程" @click="setScreen('peek', 'replace')"><span></span></button>
          <div class="sheet-title-row"><div><span class="sheet-eyebrow">杭州周末行</span><h1>D1 · 西湖周边</h1></div><button class="sheet-collapse-button" type="button" aria-label="收起行程" @click="setScreen('peek', 'replace')"><IonIcon :icon="chevronDownOutline" /></button></div>
          <div class="mobile-day-tabs"><button class="active" type="button">D1 <small>4/18</small></button><button type="button">D2 <small>4/19</small></button><button type="button">全程</button></div>
          <div class="mobile-route-summary"><span><b>6.2 km</b> · 2h 10m</span><small><IonIcon :icon="navigateOutline" /> 高德 · 步行</small></div>
          <div class="mobile-timeline">
            <button v-for="stop in stops" :key="stop.id" type="button" class="mobile-stop-row" :class="{ selected: activeStopID === stop.id }" @click="selectStop(stop.id)"><span class="stop-number">{{ stop.number }}</span><span class="stop-copy"><strong>{{ stop.title }}</strong><small>{{ stop.time }} · {{ stop.address }}</small></span><span class="stop-chevron">›</span></button>
          </div>
          <button class="mobile-add-place" type="button"><IonIcon :icon="searchOutline" /> 搜索并添加规划点</button>
        </article>

        <article v-else class="mobile-sheet stop-sheet">
          <button class="sheet-handle" type="button" aria-label="返回行程时间线" @click="goBack"><span></span></button>
          <div class="stop-detail-header"><button type="button" class="inline-back" @click="goBack"><span>‹</span> D1 行程</button><button type="button" class="sheet-more-button" aria-label="规划点更多操作"><IonIcon :icon="ellipsisHorizontalOutline" /></button></div>
          <div class="stop-detail-kicker"><span>规划点 {{ selectedStop.number }}</span><span>{{ selectedStop.type }}</span></div>
          <h1>{{ activeChild ? '断桥残雪观景位' : selectedStop.title }}</h1>
          <p class="stop-address">{{ activeChild ? '西湖断桥北侧观景平台' : selectedStop.address }}</p>
          <div class="stop-detail-meta"><span><IonIcon :icon="timeOutline" /> {{ activeChild ? '09:20–09:50' : selectedStop.time }}</span><span><IonIcon :icon="sunnyOutline" /> 晴 · 22°C</span></div>
          <div class="stop-primary-actions"><button class="primary-button" type="button"><IonIcon :icon="navigateOutline" /> 开始导航</button><button class="secondary-button" type="button">编辑</button></div>
          <section class="mobile-detail-section weather-section"><div class="section-icon"><IonIcon :icon="sunnyOutline" /></div><div><strong>晴朗 · 22°C</strong><small>天气快照更新于今天 08:00</small></div><button type="button">刷新</button></section>
          <section class="mobile-detail-section"><div class="detail-section-heading"><h2>地点说明</h2><button type="button">编辑</button></div><p>适合清晨拍照，沿北山街慢慢走到湖边。把说明、天气和导航集中在同一个 Sheet 中，返回时不会丢失行程上下文。</p></section>
          <section v-if="!activeChild" class="mobile-detail-section child-section"><div class="detail-section-heading"><h2>子规划点 <span>2</span></h2><button type="button">添加</button></div><button class="child-row" type="button" @click="activeChild = true"><b>断桥残雪观景位</b><small>09:20 · 观景</small><span>›</span></button><button class="child-row" type="button"><b>湖畔咖啡</b><small>10:00 · 餐饮</small><span>›</span></button></section>
          <button v-else class="return-parent-button" type="button" @click="activeChild = false">‹ 返回主规划点：西湖断桥</button>
        </article>
      </div>
    </section>

    <footer class="prototype-footer"><span>视觉与交互原型 · 模拟地图数据 · 不产生真实保存操作</span><span>建议验证：360×800 / 390×844 / 1280×800</span></footer>
  </main>
</template>

<style scoped>
:global(html),
:global(body),
:global(#app) { min-height: 100%; }

.prototype-shell {
  --proto-bg: #f6f3ed;
  --proto-surface: #fffdf8;
  --proto-surface-alt: #ebe8df;
  --proto-surface-raised: #ffffff;
  --proto-ink: #1d2b2a;
  --proto-muted: #64716f;
  --proto-line: #d8d6cc;
  --proto-primary: #24695c;
  --proto-primary-dark: #164a41;
  --proto-on-primary: #ffffff;
  --proto-accent: #e56a4d;
  --proto-sky: #4f7fa3;
  --proto-route: #3d73d8;
  --proto-warning: #b77918;
  --proto-map: #dbe8e2;
  --proto-water: #9ccfd0;
  --proto-shadow: 0 22px 70px rgba(34, 46, 43, .18);
  min-height: 100dvh;
  color: var(--proto-ink);
  background: var(--proto-bg);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans SC", sans-serif;
  transition: background .25s ease, color .25s ease;
}

.prototype-dark {
  --proto-bg: #101817;
  --proto-surface: #18211f;
  --proto-surface-alt: #22302c;
  --proto-surface-raised: #1d2926;
  --proto-ink: #f4f0e8;
  --proto-muted: #b5c1bc;
  --proto-line: #53625e;
  --proto-primary: #8bcdb5;
  --proto-primary-dark: #62ad96;
  --proto-on-primary: #10201c;
  --proto-accent: #ff9a78;
  --proto-sky: #8bb5ff;
  --proto-route: #94baff;
  --proto-map: #243c3a;
  --proto-water: #326567;
  --proto-shadow: 0 22px 70px rgba(0, 0, 0, .38);
}

* { box-sizing: border-box; }
button, a { font: inherit; }
button { cursor: pointer; }
button:focus-visible, a:focus-visible { outline: 3px solid color-mix(in srgb, var(--proto-accent) 72%, transparent); outline-offset: 3px; }

.prototype-toolbar {
  position: sticky;
  z-index: 100;
  top: 0;
  display: flex;
  min-height: 68px;
  align-items: center;
  gap: 22px;
  padding: 10px clamp(16px, 3vw, 40px);
  border-bottom: 1px solid color-mix(in srgb, var(--proto-line) 72%, transparent);
  background: color-mix(in srgb, var(--proto-bg) 88%, transparent);
  backdrop-filter: blur(20px);
}

.prototype-brand { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 8px; white-space: nowrap; }
.prototype-brand-mark { display: inline-grid; width: 28px; height: 28px; place-items: center; border-radius: 10px; color: var(--proto-on-primary); background: var(--proto-primary); font-size: 16px; }
.prototype-brand strong { letter-spacing: -.03em; }
.prototype-caption, .prototype-review-note { color: var(--proto-muted); font-size: 11px; }
.prototype-caption { padding-left: 6px; }
.prototype-tabs { display: flex; min-width: 0; flex: 1 1 auto; gap: 4px; overflow-x: auto; scrollbar-width: none; }
.prototype-tabs::-webkit-scrollbar { display: none; }
.prototype-tabs button { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 6px; min-height: 38px; padding: 7px 11px; border: 1px solid transparent; border-radius: 12px; color: var(--proto-muted); background: transparent; font-size: 12px; font-weight: 700; white-space: nowrap; }
.prototype-tabs button span { color: color-mix(in srgb, var(--proto-muted) 72%, transparent); font-size: 10px; }
.prototype-tabs button:hover { color: var(--proto-ink); background: var(--proto-surface-alt); }
.prototype-tabs button.active { border-color: color-mix(in srgb, var(--proto-primary) 40%, var(--proto-line)); color: var(--proto-primary-dark); background: color-mix(in srgb, var(--proto-primary) 12%, var(--proto-surface)); }
.prototype-tabs button.active span { color: var(--proto-primary); }
.prototype-toolbar-actions { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 10px; }
.prototype-icon-button, .round-icon { display: inline-grid; place-items: center; width: 38px; height: 38px; border: 1px solid var(--proto-line); border-radius: 12px; color: var(--proto-ink); background: var(--proto-surface); }
.prototype-icon-button ion-icon, .round-icon ion-icon { font-size: 18px; }
.prototype-exit { color: var(--proto-muted); font-size: 11px; text-decoration: none; }
.prototype-exit:hover { color: var(--proto-primary); }

.prototype-stage { display: grid; min-height: calc(100dvh - 108px); place-items: center; padding: 24px clamp(14px, 3vw, 48px); }
.prototype-footer { display: flex; justify-content: space-between; gap: 16px; padding: 12px clamp(16px, 3vw, 40px) 18px; color: var(--proto-muted); font-size: 10px; }

/* Shared map prototype */
.mock-map { position: relative; overflow: hidden; background: var(--proto-map); isolation: isolate; }
.mock-map::before { position: absolute; z-index: 0; inset: 0; background-image: linear-gradient(115deg, transparent 0 17%, color-mix(in srgb, var(--proto-surface) 48%, transparent) 17.2% 17.7%, transparent 17.9% 100%), linear-gradient(25deg, transparent 0 68%, color-mix(in srgb, var(--proto-surface) 54%, transparent) 68.2% 68.8%, transparent 69% 100%), linear-gradient(90deg, color-mix(in srgb, var(--proto-ink) 3%, transparent) 1px, transparent 1px), linear-gradient(0deg, color-mix(in srgb, var(--proto-ink) 3%, transparent) 1px, transparent 1px); background-position: center; background-size: 100% 100%, 100% 100%, 56px 56px, 56px 56px; content: ""; opacity: .8; }
.map-water { position: absolute; z-index: 1; border: 6px solid color-mix(in srgb, var(--proto-water) 65%, transparent); background: color-mix(in srgb, var(--proto-water) 64%, transparent); opacity: .94; }
.water-main { top: -14%; left: 38%; width: 40%; height: 122%; border-radius: 48% 58% 40% 66%; transform: rotate(17deg); }
.water-small { right: -10%; bottom: -8%; width: 45%; height: 28%; border-radius: 54% 0 0 0; transform: rotate(-8deg); }
.map-block { position: absolute; z-index: 2; border: 1px solid color-mix(in srgb, var(--proto-ink) 8%, transparent); border-radius: 6px; background: color-mix(in srgb, var(--proto-surface) 55%, transparent); box-shadow: inset 0 0 0 5px color-mix(in srgb, var(--proto-surface) 20%, transparent); opacity: .72; }
.block-a { top: 14%; left: 6%; width: 26%; height: 22%; transform: rotate(-9deg); }
.block-b { top: 55%; left: 5%; width: 30%; height: 25%; transform: rotate(11deg); }
.block-c { top: 11%; right: 7%; width: 24%; height: 29%; transform: rotate(8deg); }
.map-road { position: absolute; z-index: 3; height: 3px; border-radius: 99px; background: color-mix(in srgb, var(--proto-ink) 18%, var(--proto-surface)); box-shadow: 0 0 0 2px color-mix(in srgb, var(--proto-surface) 22%, transparent); opacity: .72; }
.road-one { top: 27%; left: -5%; width: 54%; transform: rotate(12deg); }
.road-two { top: 62%; left: -8%; width: 66%; transform: rotate(-17deg); }
.road-three { top: 47%; right: -5%; width: 65%; transform: rotate(22deg); }
.road-four { top: 75%; right: 2%; width: 48%; transform: rotate(-28deg); }
.route-lines { position: absolute; z-index: 5; inset: 0; width: 100%; height: 100%; overflow: visible; }
.route-lines polyline { fill: none; stroke: var(--proto-route); stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.2; vector-effect: non-scaling-stroke; }
.route-lines .route-shadow { stroke: color-mix(in srgb, var(--proto-surface) 86%, transparent); stroke-width: 5; opacity: .92; }
.map-pin { position: absolute; z-index: 7; display: grid; width: 32px; height: 32px; place-items: center; padding: 0; border: 3px solid var(--proto-surface-raised); border-radius: 50% 50% 50% 3px; color: var(--proto-on-primary); background: var(--proto-primary); box-shadow: 0 8px 18px rgba(25, 46, 41, .24); transform: translate(-50%, -92%) rotate(-45deg); transition: transform .18s ease, background .18s ease, scale .18s ease; }
.map-pin span { transform: rotate(45deg); font-size: 11px; font-weight: 800; }
.map-pin.selected { z-index: 9; background: var(--proto-accent); scale: 1.16; }
.map-label { position: absolute; z-index: 6; color: color-mix(in srgb, var(--proto-ink) 68%, transparent); font-size: 12px; font-weight: 800; letter-spacing: .08em; }
.label-west { top: 42%; left: 56%; writing-mode: vertical-rl; }
.label-north { top: 22%; left: 18%; }
.label-south { bottom: 20%; left: 27%; }
.map-compass { position: absolute; z-index: 8; top: 18px; right: 18px; display: grid; width: 38px; height: 38px; place-items: center; border: 1px solid color-mix(in srgb, var(--proto-line) 74%, transparent); border-radius: 50%; color: var(--proto-primary); background: color-mix(in srgb, var(--proto-surface) 84%, transparent); box-shadow: 0 8px 20px rgba(27, 45, 40, .12); backdrop-filter: blur(10px); }
.map-compass ion-icon { font-size: 18px; }

/* Desktop prototype */
.desktop-prototype { display: grid; width: min(1440px, 100%); min-height: min(790px, calc(100dvh - 155px)); grid-template-columns: 78px 350px minmax(480px, 1fr); overflow: hidden; border: 1px solid color-mix(in srgb, var(--proto-line) 70%, transparent); border-radius: 28px; background: var(--proto-surface); box-shadow: var(--proto-shadow); }
.desktop-rail { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 22px 10px; border-right: 1px solid var(--proto-line); background: var(--proto-surface-alt); }
.rail-logo { display: grid; width: 38px; height: 38px; margin-bottom: 14px; place-items: center; border-radius: 13px; color: var(--proto-on-primary); background: var(--proto-primary); font-size: 20px; }
.rail-item { display: grid; width: 56px; min-height: 56px; place-items: center; gap: 3px; padding: 7px 3px; border: 0; border-radius: 16px; color: var(--proto-muted); background: transparent; font-size: 10px; font-weight: 700; }
.rail-item ion-icon { font-size: 20px; }
.rail-item:hover, .rail-item.selected { color: var(--proto-primary-dark); background: color-mix(in srgb, var(--proto-primary) 15%, var(--proto-surface)); }
.rail-spacer { flex: 1; }
.rail-muted { opacity: .72; }
.desktop-itinerary { min-width: 0; overflow: auto; padding: 26px 20px 24px; border-right: 1px solid var(--proto-line); background: var(--proto-surface); }
.desktop-panel-header, .drawer-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.section-kicker { display: block; margin-bottom: 7px; color: var(--proto-primary); font-size: 10px; font-weight: 800; letter-spacing: .14em; text-transform: uppercase; }
.desktop-panel-header h1 { margin: 0; color: var(--proto-ink); font-size: 25px; letter-spacing: -.04em; }
.desktop-panel-header p { margin: 7px 0 0; color: var(--proto-muted); font-size: 12px; }
.desktop-panel-actions { display: flex; gap: 8px; margin-top: 20px; }
.primary-button, .secondary-button { display: inline-flex; min-height: 40px; align-items: center; justify-content: center; gap: 6px; padding: 8px 13px; border: 1px solid var(--proto-line); border-radius: 12px; font-size: 12px; font-weight: 800; white-space: nowrap; }
.primary-button { border-color: var(--proto-primary); color: var(--proto-on-primary); background: var(--proto-primary); }
.secondary-button { color: var(--proto-primary-dark); background: var(--proto-surface); }
.primary-button:hover { background: var(--proto-primary-dark); }
.secondary-button:hover { border-color: var(--proto-primary); background: color-mix(in srgb, var(--proto-primary) 9%, var(--proto-surface)); }
.primary-button ion-icon, .secondary-button ion-icon { font-size: 16px; }
.desktop-day-tabs { display: flex; gap: 5px; margin-top: 22px; padding-bottom: 4px; border-bottom: 1px solid var(--proto-line); }
.desktop-day-tabs button { min-height: 34px; padding: 6px 9px; border: 0; border-radius: 9px 9px 0 0; color: var(--proto-muted); background: transparent; font-size: 11px; font-weight: 800; }
.desktop-day-tabs button.active { color: var(--proto-primary-dark); box-shadow: inset 0 -2px 0 var(--proto-primary); }
.desktop-route-card { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 16px 0; padding: 12px; border: 1px solid color-mix(in srgb, var(--proto-primary) 24%, var(--proto-line)); border-radius: 14px; background: color-mix(in srgb, var(--proto-primary) 7%, var(--proto-surface)); }
.desktop-route-card div { display: grid; gap: 3px; }
.desktop-route-card span { color: var(--proto-muted); font-size: 10px; font-weight: 700; }
.desktop-route-card strong { color: var(--proto-primary-dark); font-size: 14px; }
.desktop-route-card i { padding: 0 3px; font-style: normal; color: var(--proto-muted); }
.desktop-route-card small { display: inline-flex; align-items: center; gap: 3px; color: var(--proto-muted); font-size: 10px; white-space: nowrap; }
.desktop-timeline { position: relative; display: grid; gap: 4px; }
.desktop-timeline::before { position: absolute; top: 27px; bottom: 27px; left: 20px; width: 1px; background: color-mix(in srgb, var(--proto-primary) 32%, var(--proto-line)); content: ""; }
.desktop-stop-row { position: relative; z-index: 1; display: flex; width: 100%; min-width: 0; align-items: center; gap: 10px; padding: 9px 7px; border: 1px solid transparent; border-radius: 14px; text-align: left; color: var(--proto-ink); background: transparent; }
.desktop-stop-row:hover, .desktop-stop-row.selected { border-color: color-mix(in srgb, var(--proto-primary) 35%, var(--proto-line)); background: color-mix(in srgb, var(--proto-primary) 10%, var(--proto-surface)); }
.timeline-dot { display: grid; flex: 0 0 28px; width: 28px; height: 28px; place-items: center; border: 3px solid var(--proto-surface); border-radius: 50%; color: var(--proto-on-primary); background: var(--proto-primary); box-shadow: 0 0 0 1px color-mix(in srgb, var(--proto-primary) 40%, var(--proto-line)); font-size: 10px; font-weight: 800; }
.desktop-stop-row.selected .timeline-dot { background: var(--proto-accent); }
.timeline-copy { display: grid; min-width: 0; flex: 1; gap: 3px; }
.timeline-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.timeline-copy small { overflow: hidden; color: var(--proto-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.row-chevron { color: var(--proto-muted); font-size: 21px; }
.desktop-add-row { display: inline-flex; width: 100%; min-height: 40px; align-items: center; justify-content: center; gap: 7px; margin-top: 13px; border: 1px dashed var(--proto-line); border-radius: 12px; color: var(--proto-primary); background: transparent; font-size: 11px; font-weight: 800; }
.desktop-map-pane { position: relative; min-width: 0; overflow: hidden; background: var(--proto-map); }
.desktop-map-topbar { position: absolute; z-index: 20; top: 18px; right: 18px; left: 18px; display: flex; align-items: center; justify-content: space-between; gap: 12px; pointer-events: none; }
.desktop-map-title, .desktop-map-tools { pointer-events: auto; }
.desktop-map-title { display: grid; grid-template-columns: auto 1fr; align-items: center; column-gap: 7px; padding: 10px 13px; border: 1px solid color-mix(in srgb, var(--proto-line) 78%, transparent); border-radius: 14px; background: color-mix(in srgb, var(--proto-surface) 87%, transparent); box-shadow: 0 8px 24px rgba(29, 48, 43, .12); backdrop-filter: blur(14px); }
.desktop-map-title > span:not(.live-dot) { font-size: 12px; font-weight: 800; }
.desktop-map-title small { grid-column: 2; color: var(--proto-muted); font-size: 10px; }
.live-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--proto-primary); box-shadow: 0 0 0 4px color-mix(in srgb, var(--proto-primary) 18%, transparent); }
.desktop-map-tools { display: flex; gap: 6px; }
.map-tool-button { display: inline-flex; min-height: 38px; align-items: center; gap: 5px; padding: 7px 11px; border: 1px solid color-mix(in srgb, var(--proto-line) 78%, transparent); border-radius: 11px; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 88%, transparent); box-shadow: 0 8px 24px rgba(29, 48, 43, .12); backdrop-filter: blur(14px); font-size: 11px; font-weight: 800; }
.map-tool-button.active { border-color: color-mix(in srgb, var(--proto-primary) 45%, var(--proto-line)); color: var(--proto-primary-dark); }
.desktop-map { position: absolute; inset: 0; }
.desktop-map .map-pin { width: 38px; height: 38px; }
.desktop-map .map-label { font-size: 16px; }
.desktop-map .map-compass { top: 92px; right: 22px; }
.map-scale { position: absolute; z-index: 8; right: 24px; bottom: 20px; padding: 5px 8px; border-bottom: 2px solid color-mix(in srgb, var(--proto-ink) 55%, transparent); color: var(--proto-muted); font-size: 10px; }
.desktop-map-status { position: absolute; z-index: 18; right: 22px; bottom: 20px; left: 22px; display: flex; align-items: center; gap: 8px; min-height: 42px; padding: 8px 12px; border: 1px solid color-mix(in srgb, var(--proto-line) 70%, transparent); border-radius: 13px; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 90%, transparent); box-shadow: 0 8px 24px rgba(29, 48, 43, .14); backdrop-filter: blur(14px); font-size: 11px; }
.status-mark { display: inline-grid; flex: 0 0 19px; width: 19px; height: 19px; place-items: center; border-radius: 50%; color: var(--proto-on-primary); background: var(--proto-primary); font-size: 11px; font-weight: 900; }
.desktop-map-status button { margin-left: auto; border: 0; color: var(--proto-primary-dark); background: transparent; font-size: 10px; font-weight: 800; }
.desktop-detail-drawer { position: absolute; z-index: 24; top: 18px; right: 18px; bottom: 76px; width: min(330px, 35%); overflow: auto; padding: 24px 20px; border: 1px solid color-mix(in srgb, var(--proto-line) 78%, transparent); border-radius: 22px; background: color-mix(in srgb, var(--proto-surface) 94%, transparent); box-shadow: 0 18px 58px rgba(25, 43, 39, .2); backdrop-filter: blur(22px); }
.desktop-detail-drawer h2 { margin: 0; color: var(--proto-ink); font-size: 24px; letter-spacing: -.04em; }
.drawer-address { margin: 8px 0 0; color: var(--proto-muted); font-size: 12px; line-height: 1.5; }
.drawer-meta, .stop-detail-meta { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 16px; }
.drawer-meta span, .stop-detail-meta span { display: inline-flex; align-items: center; gap: 4px; padding: 6px 8px; border-radius: 8px; color: var(--proto-muted); background: var(--proto-surface-alt); font-size: 10px; }
.drawer-meta ion-icon, .stop-detail-meta ion-icon { color: var(--proto-primary); font-size: 14px; }
.drawer-actions { display: flex; gap: 7px; margin-top: 18px; }
.drawer-actions .primary-button { flex: 1; }
.drawer-section { margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--proto-line); }
.drawer-section h3, .detail-section-heading h2 { display: flex; align-items: center; justify-content: space-between; margin: 0 0 9px; color: var(--proto-ink); font-size: 14px; }
.drawer-section h3 span, .detail-section-heading h2 span { color: var(--proto-muted); font-size: 11px; font-weight: 700; }
.drawer-section p { margin: 0; color: var(--proto-muted); font-size: 12px; line-height: 1.65; }
.child-row { display: grid; width: 100%; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 8px; padding: 10px 0; border: 0; border-bottom: 1px solid color-mix(in srgb, var(--proto-line) 62%, transparent); text-align: left; color: var(--proto-ink); background: transparent; }
.child-row b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.child-row small { color: var(--proto-muted); font-size: 10px; }
.child-row > span { grid-row: 1 / 3; grid-column: 2; align-self: center; color: var(--proto-muted); font-size: 20px; }

/* Mobile prototypes */
.mobile-viewport { position: relative; width: min(430px, 100%); height: min(820px, calc(100dvh - 155px)); min-height: 660px; overflow: hidden; border: 1px solid color-mix(in srgb, var(--proto-line) 76%, transparent); border-radius: 32px; background: var(--proto-surface); box-shadow: var(--proto-shadow); }
.mobile-list-screen { position: relative; height: 100%; overflow: auto; padding: 28px 20px 98px; background: radial-gradient(circle at 86% 0%, color-mix(in srgb, var(--proto-accent) 13%, transparent), transparent 26%), var(--proto-bg); }
.mobile-list-header { padding: 5px 2px 22px; }
.mobile-inline-brand { display: inline-flex; align-items: center; gap: 7px; color: var(--proto-muted); font-size: 12px; font-weight: 800; letter-spacing: .02em; }
.mobile-inline-brand .prototype-brand-mark { width: 23px; height: 23px; border-radius: 8px; font-size: 12px; }
.preview-chip { padding: 4px 7px; border-radius: 999px; color: var(--proto-primary-dark); background: color-mix(in srgb, var(--proto-primary) 13%, var(--proto-surface)); font-size: 9px; }
.list-heading-row { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; margin-top: 46px; }
.list-heading-row h1 { margin: 0; color: var(--proto-ink); font-size: 32px; letter-spacing: -.06em; }
.list-heading-row p { margin: 8px 0 0; color: var(--proto-muted); font-size: 12px; }
.list-header-actions { display: flex; gap: 5px; }
.list-header-actions button { display: grid; width: 38px; height: 38px; place-items: center; border: 1px solid var(--proto-line); border-radius: 12px; color: var(--proto-muted); background: color-mix(in srgb, var(--proto-surface) 86%, transparent); }
.list-header-actions ion-icon { font-size: 17px; }
.list-filter-row { display: flex; align-items: center; justify-content: space-between; padding: 13px 0; border-top: 1px solid var(--proto-line); border-bottom: 1px solid var(--proto-line); color: var(--proto-muted); font-size: 11px; font-weight: 800; }
.list-filter-row b { color: var(--proto-primary); }
.list-filter-row button { display: inline-flex; align-items: center; gap: 3px; border: 0; color: var(--proto-muted); background: transparent; font-size: 11px; }
.list-filter-row ion-icon { font-size: 13px; }
.mobile-trip-cards { display: grid; gap: 10px; margin-top: 16px; }
.mobile-trip-card { position: relative; display: flex; width: 100%; min-height: 88px; align-items: center; gap: 12px; padding: 12px; border: 1px solid var(--proto-line); border-radius: 18px; text-align: left; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 92%, transparent); box-shadow: 0 7px 22px rgba(43, 54, 49, .05); }
.mobile-trip-card:hover { border-color: color-mix(in srgb, var(--proto-primary) 48%, var(--proto-line)); transform: translateY(-1px); }
.prototype-trip-visual { position: relative; display: grid; flex: 0 0 64px; width: 64px; height: 64px; place-items: end start; overflow: hidden; padding: 8px; border-radius: 16px; color: #ffffff; background: linear-gradient(145deg, #24695c, #9bc9a8 58%, #eb8b6d); }
.prototype-trip-visual::before, .prototype-trip-visual::after { position: absolute; border: 1px solid #ffffff66; border-radius: 50%; content: ""; }
.prototype-trip-visual::before { top: -25px; right: -28px; width: 82px; height: 82px; }
.prototype-trip-visual::after { right: -5px; bottom: -28px; width: 64px; height: 64px; background: #ffffff1f; }
.prototype-trip-visual > span { position: relative; z-index: 1; font-size: 18px; font-weight: 900; text-shadow: 0 1px 4px #153c36aa; }
.prototype-trip-visual-1 { background: linear-gradient(145deg, #4f7fa3, #b8d7d0 55%, #eb8b70); }
.prototype-trip-visual-2 { background: linear-gradient(145deg, #715c88, #d8a9ad 55%, #f1bd83); }
.trip-card-content { display: grid; min-width: 0; flex: 1; gap: 5px; padding: 14px 4px 13px; }
.trip-card-content strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 16px; }
.trip-dates, .trip-stats { color: var(--proto-muted); font-size: 11px; }
.trip-stats b { color: var(--proto-primary-dark); }
.trip-stats i { padding: 0 3px; font-style: normal; }
.trip-card-arrow { align-self: center; padding-right: 8px; color: var(--proto-muted); font-size: 24px; }
.mobile-list-hint { display: flex; gap: 7px; margin: 17px 3px 0; color: var(--proto-muted); font-size: 10px; line-height: 1.5; }
.mobile-list-hint span { color: var(--proto-primary); font-size: 16px; }
.mobile-fab { position: absolute; right: 18px; bottom: 20px; display: inline-flex; min-height: 47px; align-items: center; gap: 6px; padding: 9px 15px 9px 12px; border: 0; border-radius: 15px; color: var(--proto-on-primary); background: var(--proto-accent); box-shadow: 0 12px 28px color-mix(in srgb, var(--proto-accent) 38%, transparent); font-size: 12px; font-weight: 900; }
.mobile-fab ion-icon { font-size: 18px; }

.map-viewport { background: var(--proto-map); }
.mobile-map { position: absolute; inset: 0; }
.mobile-map-context { position: absolute; z-index: 18; top: max(12px, env(safe-area-inset-top)); right: 13px; left: 13px; display: flex; align-items: center; gap: 10px; pointer-events: none; }
.mobile-map-context > * { pointer-events: auto; }
.mobile-back-button, .mobile-map-menu { display: grid; flex: 0 0 auto; width: 44px; height: 44px; place-items: center; border: 1px solid color-mix(in srgb, var(--proto-line) 72%, transparent); border-radius: 14px; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 88%, transparent); box-shadow: 0 8px 22px rgba(29, 48, 43, .12); backdrop-filter: blur(14px); }
.mobile-back-button { align-content: center; gap: 0; }
.mobile-back-button span { height: 20px; font-size: 26px; line-height: 17px; }
.mobile-back-button small { color: var(--proto-muted); font-size: 8px; font-weight: 800; }
.mobile-map-menu ion-icon { font-size: 18px; }
.mobile-context-copy { display: grid; min-width: 0; flex: 1; gap: 3px; padding: 9px 12px; border: 1px solid color-mix(in srgb, var(--proto-line) 72%, transparent); border-radius: 14px; background: color-mix(in srgb, var(--proto-surface) 86%, transparent); box-shadow: 0 8px 22px rgba(29, 48, 43, .12); backdrop-filter: blur(14px); }
.mobile-context-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.mobile-context-copy span { overflow: hidden; color: var(--proto-muted); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.mobile-map-top-status { position: absolute; z-index: 10; top: 74px; left: 14px; display: inline-flex; align-items: center; gap: 5px; min-height: 30px; padding: 5px 9px; border: 1px solid color-mix(in srgb, var(--proto-line) 72%, transparent); border-radius: 999px; color: var(--proto-muted); background: color-mix(in srgb, var(--proto-surface) 82%, transparent); box-shadow: 0 8px 20px rgba(29, 48, 43, .1); backdrop-filter: blur(12px); font-size: 9px; font-weight: 800; }
.mobile-map-top-status .status-mark { width: 14px; height: 14px; font-size: 8px; }
.mobile-map .map-compass { top: 76px; right: 14px; width: 32px; height: 32px; }
.mobile-map .map-compass ion-icon { font-size: 15px; }
.mobile-map-options { position: absolute; z-index: 30; top: 66px; right: 13px; width: min(276px, calc(100% - 26px)); padding: 14px; border: 1px solid var(--proto-line); border-radius: 18px; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 96%, transparent); box-shadow: 0 18px 50px rgba(27, 45, 40, .2); backdrop-filter: blur(22px); }
.options-heading { display: flex; align-items: center; justify-content: space-between; padding-bottom: 11px; border-bottom: 1px solid var(--proto-line); font-size: 13px; }
.options-heading button { display: grid; width: 28px; height: 28px; place-items: center; border: 0; border-radius: 8px; color: var(--proto-muted); background: transparent; }
.options-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 12px 0; border-bottom: 1px solid color-mix(in srgb, var(--proto-line) 60%, transparent); }
.options-row > span { color: var(--proto-muted); font-size: 11px; }
.option-segment { display: inline-flex; gap: 3px; padding: 3px; border-radius: 10px; background: var(--proto-surface-alt); }
.option-segment button { min-height: 29px; padding: 5px 8px; border: 0; border-radius: 8px; color: var(--proto-muted); background: transparent; font-size: 10px; font-weight: 800; }
.option-segment button.active { color: var(--proto-on-primary); background: var(--proto-primary); }
.option-action { display: inline-flex; width: 100%; min-height: 37px; align-items: center; justify-content: center; gap: 6px; margin-top: 12px; border: 1px dashed var(--proto-line); border-radius: 11px; color: var(--proto-primary-dark); background: transparent; font-size: 11px; font-weight: 800; }
.mobile-sheet { position: absolute; z-index: 20; right: 7px; bottom: max(7px, env(safe-area-inset-bottom)); left: 7px; overflow: hidden; border: 1px solid color-mix(in srgb, var(--proto-line) 74%, transparent); border-radius: 26px; color: var(--proto-ink); background: color-mix(in srgb, var(--proto-surface) 96%, transparent); box-shadow: 0 18px 60px rgba(26, 42, 37, .25); backdrop-filter: blur(22px); }
.peek-sheet { padding: 0 15px 15px; }
.half-sheet { max-height: 55%; padding: 0 15px 14px; }
.stop-sheet { top: 68px; max-height: calc(100% - 75px); overflow: auto; padding: 0 17px 18px; }
.sheet-handle { display: grid; width: 100%; height: 27px; place-items: center; border: 0; background: transparent; }
.sheet-handle span { display: block; width: 42px; height: 4px; border-radius: 999px; background: color-mix(in srgb, var(--proto-muted) 45%, transparent); }
.sheet-peek-header, .sheet-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.sheet-eyebrow, .stop-detail-kicker { display: block; color: var(--proto-primary); font-size: 10px; font-weight: 900; letter-spacing: .12em; text-transform: uppercase; }
.sheet-peek-header h1, .sheet-title-row h1 { margin: 4px 0 0; color: var(--proto-ink); font-size: 19px; letter-spacing: -.04em; }
.sheet-expand-button, .sheet-collapse-button, .sheet-more-button { display: inline-flex; align-items: center; gap: 4px; min-height: 34px; padding: 6px 9px; border: 1px solid color-mix(in srgb, var(--proto-primary) 38%, var(--proto-line)); border-radius: 10px; color: var(--proto-primary-dark); background: color-mix(in srgb, var(--proto-primary) 8%, var(--proto-surface)); font-size: 10px; font-weight: 900; white-space: nowrap; }
.sheet-expand-button span { font-size: 15px; }
.sheet-collapse-button, .sheet-more-button { display: grid; width: 34px; padding: 0; place-items: center; border-color: var(--proto-line); color: var(--proto-muted); background: transparent; }
.sheet-collapse-button ion-icon, .sheet-more-button ion-icon { font-size: 17px; }
.sheet-metrics { display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--proto-line); }
.sheet-metrics span { display: grid; gap: 3px; }
.sheet-metrics b { color: var(--proto-ink); font-size: 12px; }
.sheet-metrics small { color: var(--proto-muted); font-size: 9px; }
.sheet-peek-hint { margin: 13px 0 0; color: var(--proto-muted); font-size: 10px; }
.mobile-day-tabs { display: flex; gap: 5px; overflow-x: auto; margin-top: 14px; padding-bottom: 2px; scrollbar-width: none; }
.mobile-day-tabs::-webkit-scrollbar { display: none; }
.mobile-day-tabs button { display: inline-flex; flex: 0 0 auto; min-height: 35px; align-items: center; gap: 4px; padding: 6px 11px; border: 1px solid var(--proto-line); border-radius: 10px; color: var(--proto-muted); background: transparent; font-size: 11px; font-weight: 900; }
.mobile-day-tabs button small { font-size: 9px; font-weight: 700; }
.mobile-day-tabs button.active { border-color: var(--proto-primary); color: var(--proto-on-primary); background: var(--proto-primary); }
.mobile-route-summary { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 10px; padding: 9px 10px; border-radius: 11px; color: var(--proto-primary-dark); background: color-mix(in srgb, var(--proto-primary) 9%, var(--proto-surface)); font-size: 11px; }
.mobile-route-summary small { display: inline-flex; align-items: center; gap: 3px; color: var(--proto-muted); font-size: 9px; white-space: nowrap; }
.mobile-timeline { position: relative; display: grid; gap: 3px; margin-top: 9px; }
.mobile-timeline::before { position: absolute; top: 22px; bottom: 22px; left: 17px; width: 1px; background: color-mix(in srgb, var(--proto-primary) 32%, var(--proto-line)); content: ""; }
.mobile-stop-row { position: relative; z-index: 1; display: flex; width: 100%; min-width: 0; align-items: center; gap: 9px; padding: 7px 4px; border: 1px solid transparent; border-radius: 11px; text-align: left; color: var(--proto-ink); background: transparent; }
.mobile-stop-row:hover, .mobile-stop-row.selected { border-color: color-mix(in srgb, var(--proto-primary) 36%, var(--proto-line)); background: color-mix(in srgb, var(--proto-primary) 9%, var(--proto-surface)); }
.stop-number { display: grid; flex: 0 0 27px; width: 27px; height: 27px; place-items: center; border: 3px solid var(--proto-surface); border-radius: 50%; color: var(--proto-on-primary); background: var(--proto-primary); box-shadow: 0 0 0 1px color-mix(in srgb, var(--proto-primary) 35%, var(--proto-line)); font-size: 10px; font-weight: 900; }
.mobile-stop-row.selected .stop-number { background: var(--proto-accent); }
.stop-copy { display: grid; min-width: 0; flex: 1; gap: 3px; }
.stop-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.stop-copy small { overflow: hidden; color: var(--proto-muted); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.stop-chevron { color: var(--proto-muted); font-size: 20px; }
.mobile-add-place { display: inline-flex; width: 100%; min-height: 38px; align-items: center; justify-content: center; gap: 5px; margin-top: 9px; border: 1px dashed var(--proto-line); border-radius: 11px; color: var(--proto-primary-dark); background: transparent; font-size: 10px; font-weight: 900; }
.stop-detail-header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.inline-back { display: inline-flex; align-items: center; gap: 3px; min-height: 34px; padding: 4px 0; border: 0; color: var(--proto-primary-dark); background: transparent; font-size: 11px; font-weight: 900; }
.inline-back span { font-size: 22px; line-height: 16px; }
.stop-detail-kicker { display: flex; justify-content: space-between; margin-top: 13px; letter-spacing: .08em; }
.stop-sheet h1 { margin: 6px 0 0; color: var(--proto-ink); font-size: 27px; letter-spacing: -.06em; }
.stop-address { margin: 5px 0 0; color: var(--proto-muted); font-size: 11px; line-height: 1.5; }
.stop-primary-actions { display: flex; gap: 7px; margin-top: 16px; }
.stop-primary-actions .primary-button { flex: 1; }
.mobile-detail-section { margin-top: 19px; padding-top: 16px; border-top: 1px solid var(--proto-line); }
.weather-section { display: flex; align-items: center; gap: 9px; margin-top: 17px; padding: 11px; border: 0; border-radius: 14px; background: color-mix(in srgb, var(--proto-sky) 11%, var(--proto-surface)); }
.section-icon { display: grid; flex: 0 0 30px; width: 30px; height: 30px; place-items: center; border-radius: 10px; color: var(--proto-sky); background: color-mix(in srgb, var(--proto-sky) 15%, var(--proto-surface)); }
.section-icon ion-icon { font-size: 17px; }
.weather-section > div:nth-child(2) { display: grid; flex: 1; gap: 3px; }
.weather-section strong { font-size: 12px; }
.weather-section small { color: var(--proto-muted); font-size: 9px; }
.weather-section button, .detail-section-heading button { border: 0; color: var(--proto-primary-dark); background: transparent; font-size: 10px; font-weight: 900; }
.detail-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.mobile-detail-section p { margin: 0; color: var(--proto-muted); font-size: 11px; line-height: 1.7; }
.child-section .child-row:last-child { border-bottom: 0; }
.return-parent-button { display: inline-flex; width: 100%; min-height: 40px; align-items: center; margin-top: 18px; padding: 8px 10px; border: 1px dashed var(--proto-line); border-radius: 11px; color: var(--proto-primary-dark); background: transparent; font-size: 10px; font-weight: 900; }

@media (max-width: 1120px) {
  .desktop-prototype { grid-template-columns: 70px 310px minmax(420px, 1fr); }
  .desktop-detail-drawer { width: min(300px, 42%); }
  .desktop-map-title small { display: none; }
}

@media (max-width: 820px) {
  .prototype-toolbar { gap: 10px; }
  .prototype-caption, .prototype-review-note, .prototype-exit { display: none; }
  .prototype-brand { margin-right: auto; }
  .desktop-prototype { min-width: 920px; }
  .prototype-stage { justify-content: start; overflow-x: auto; }
}

@media (max-width: 720px) {
  /* The preview switcher wraps to a second row on narrow browsers. */
  .prototype-toolbar { min-height: 0; padding: 7px 10px; row-gap: 10px; }
  .prototype-brand strong { font-size: 12px; }
  .prototype-brand-mark { width: 25px; height: 25px; border-radius: 8px; font-size: 13px; }
  .prototype-tabs { order: 3; flex-basis: 100%; }
  .prototype-toolbar { flex-wrap: wrap; }
  .prototype-toolbar-actions { margin-left: 0; }
  .prototype-icon-button { width: 34px; height: 34px; }
  .prototype-stage { display: block; min-height: calc(100dvh - 100px); padding: 0; }
  .prototype-footer { display: none; }
  .mobile-viewport { width: 100%; height: calc(100dvh - 100px); min-height: 560px; border: 0; border-radius: 0; box-shadow: none; }
  .mobile-list-screen { padding-top: max(20px, env(safe-area-inset-top)); padding-bottom: calc(88px + env(safe-area-inset-bottom)); }
  .mobile-sheet { bottom: max(6px, env(safe-area-inset-bottom)); }
  .stop-sheet { top: max(63px, calc(56px + env(safe-area-inset-top))); max-height: calc(100% - 70px); }
}

@media (max-height: 720px) and (max-width: 720px) {
  .list-heading-row { margin-top: 24px; }
  .mobile-trip-card { min-height: 78px; }
  .mobile-list-hint { display: none; }
  .half-sheet { max-height: 60%; }
}
</style>
