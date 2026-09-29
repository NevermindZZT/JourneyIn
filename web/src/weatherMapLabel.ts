export function formatDayWeatherBadge(weather: Record<string, unknown> | undefined, dayDate: string, now = Date.now()): string {
  if (!dayDate || !weather || weather.available === false) return ''
  if (typeof weather.local_date === 'string' && weather.local_date.slice(0, 10) !== dayDate) return ''
  if (typeof weather.expires_at === 'string') {
    const expiresAt = Date.parse(weather.expires_at)
    if (Number.isFinite(expiresAt) && expiresAt <= now) return ''
  }
  const description = [weather.condition, weather.text_day, weather.text, weather.current_condition]
    .find(value => typeof value === 'string' && value.trim())
  const condition = typeof description === 'string' ? description.trim().slice(0, 12) : ''
  const number = (value: unknown): number | null => {
    if (typeof value !== 'number' && (typeof value !== 'string' || !value.trim())) return null
    const parsed = Number(value)
    return Number.isFinite(parsed) ? Math.round(parsed) : null
  }
  const low = number(weather.temp_min_c ?? weather.low)
  const high = number(weather.temp_max_c ?? weather.high)
  const average = number(weather.temperature_c ?? weather.temp)
  const range = low !== null && high !== null ? low + '～' + high + '°C' : average !== null ? average + '°C' : ''
  return [condition, range].filter(Boolean).join(' · ')
}

export function iconForWeatherLabel(text: string): string {
  const condition = text.toLowerCase()
  if (/雷|thunder/.test(condition)) return '⚡'
  if (/雪|snow/.test(condition)) return '❄'
  if (/雨|rain|shower/.test(condition)) return '☂'
  if (/雾|霾|fog|haze/.test(condition)) return '≋'
  if (/晴|sunny|clear/.test(condition)) return '☀'
  return '☁'
}

export type WeatherLabelCandidate = { id: string; x: number | null; y: number | null; hasWeather: boolean; nameVisible: boolean; labelWidth?: number }
export type WeatherLabelViewport = { left: number; top: number; right: number; bottom: number }

// Names are never removed here. In auto mode, only a weather overlay may be
// hidden when nearby name labels or higher-priority weather overlays need room.
export function visibleWeatherLabelIDs(candidates: WeatherLabelCandidate[], mode: 'auto' | 'always' | 'none', viewport?: WeatherLabelViewport | null): Set<string> {
  if (mode === 'none') return new Set()
  if (mode === 'always') return new Set(candidates.filter(point => point.hasWeather).map(point => point.id))
  const names = candidates.filter(point => point.nameVisible && point.x !== null && point.y !== null)
  const visible = new Set<string>()
  const weatherPoints: WeatherLabelCandidate[] = []
  for (const point of candidates) {
    if (!point.hasWeather || !point.nameVisible || point.x === null || point.y === null) continue
    if (viewport && (point.x + 14 < viewport.left || point.x + 14 + (point.labelWidth ?? 150) > viewport.right || point.y + 18 < viewport.top || point.y + 41 > viewport.bottom)) continue
    if (names.some(name => name.id !== point.id && Math.hypot(point.x! - name.x!, point.y! - name.y!) < 110)) continue
    if (weatherPoints.some(other => Math.hypot(point.x! - other.x!, point.y! - other.y!) < 140)) continue
    visible.add(point.id)
    weatherPoints.push(point)
  }
  return visible
}

