import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDayWeatherBadge, iconForWeatherLabel, visibleWeatherLabelIDs } from './src/weatherMapLabel.ts'

const day = '2026-10-03'
const now = Date.parse('2026-10-02T00:00:00Z')

test('formats forecast condition and daily temperature range', () => {
  assert.equal(formatDayWeatherBadge({ available: true, local_date: day, condition: '晴', temp_min_c: 4, temp_max_c: 17 }, day, now), '晴 · 4～17°C')
  assert.equal(formatDayWeatherBadge({ available: true, local_date: day, temperature_c: -2 }, day, now), '-2°C')
})

test('hides unavailable, mismatched and empty snapshots', () => {
  assert.equal(formatDayWeatherBadge(undefined, day, now), '')
  assert.equal(formatDayWeatherBadge({ available: false, condition: '晴' }, day, now), '')
  assert.equal(formatDayWeatherBadge({ local_date: '2026-10-04', condition: '晴' }, day, now), '')
  assert.equal(formatDayWeatherBadge({ available: true }, day, now), '')
})

test('hides expired forecasts and avoids invalid temperatures', () => {
  const expired = { condition: '阴', expires_at: '2026-10-01T00:00:00Z' }
  assert.equal(formatDayWeatherBadge(expired, day, now), '')
  assert.equal(formatDayWeatherBadge({ condition: '阴', expires_at: '2026-10-02T00:00:00Z' }, day, Date.parse('2026-10-02T00:00:00Z')), '')
  assert.equal(formatDayWeatherBadge({ condition: '晴', temp_min_c: 'not a number', temp_max_c: 20 }, day, now), '晴')
  const overlay = { id: 'expired', x: 20, y: 20, hasWeather: Boolean(formatDayWeatherBadge(expired, day, now)), nameVisible: true }
  assert.deepEqual([...visibleWeatherLabelIDs([overlay], 'always')], [])
})

test('weather labels have their own condition icon', () => {
  assert.equal(iconForWeatherLabel('晴 · 4～17°C'), '☀')
  assert.equal(iconForWeatherLabel('雨 · 6°C'), '☂')
})

test('all and hidden modes apply to weather overlays separately', () => {
  const points = [
    { id: 'first', x: 10, y: 10, hasWeather: true, nameVisible: true },
    { id: 'second', x: 25, y: 10, hasWeather: true, nameVisible: false },
  ]
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'none')], [])
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'always')], ['first', 'second'])
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'auto')], ['first'])
})

test('auto mode hides weather first near another visible name', () => {
  const points = [
    { id: 'first', x: 10, y: 10, hasWeather: true, nameVisible: true },
    { id: 'second', x: 80, y: 10, hasWeather: true, nameVisible: true },
    { id: 'third', x: 300, y: 10, hasWeather: true, nameVisible: true },
  ]
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'auto')], ['third'])
  assert.equal(points.every(point => point.nameVisible), true)
})

test('auto weather hides near an obscured or clipped viewport edge', () => {
  const points = [
    { id: 'obscured', x: 20, y: 80, labelWidth: 120, hasWeather: true, nameVisible: true },
    { id: 'visible', x: 200, y: 80, labelWidth: 100, hasWeather: true, nameVisible: true },
  ]
  const viewport = { left: 50, top: 0, right: 320, bottom: 200 }
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'auto', viewport)], ['visible'])
  assert.deepEqual([...visibleWeatherLabelIDs(points, 'always', viewport)], ['obscured', 'visible'])
})

