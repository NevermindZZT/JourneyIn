function datePartsInTimezone(date, timezone) {
  const parts = new Intl.DateTimeFormat('en-US', { calendar: 'gregory', numberingSystem: 'latn', timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(date)
  const values = {}
  for (const part of parts) if (part.type === 'year' || part.type === 'month' || part.type === 'day') values[part.type] = part.value
  if (!values.year || !values.month || !values.day) return ''
  return values.year + '-' + values.month + '-' + values.day
}

export function localDateInTimezone(instant, timezone) {
  if (!timezone || instant === undefined || instant === null || instant === '') return ''
  const date = instant instanceof Date ? instant : new Date(instant)
  if (!Number.isFinite(date.getTime())) return ''
  try { return datePartsInTimezone(date, timezone) } catch { return '' }
}

function calendarDateEpoch(date) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return null
  const [year, month, day] = date.split('-').map(Number)
  const epoch = Date.UTC(year, month - 1, day)
  return new Date(epoch).toISOString().slice(0, 10) === date ? epoch : null
}

export function isCalendarDateWithinDays(left, right, toleranceDays = 1) {
  const leftEpoch = calendarDateEpoch(left)
  const rightEpoch = calendarDateEpoch(right)
  if (leftEpoch === null || rightEpoch === null || !Number.isFinite(toleranceDays) || toleranceDays < 0) return false
  return Math.abs(leftEpoch - rightEpoch) <= Math.floor(toleranceDays) * 86400000
}

export function isPastTripDate(dayDate, timezone, now = Date.now()) {
  if (!dayDate || dayDate.length !== 10 || !timezone) return false
  const today = localDateInTimezone(now, timezone)
  return Boolean(today && dayDate < today)
}