export function isPastTripDate(dayDate, timezone, now = Date.now()) {
  if (!dayDate || dayDate.length !== 10 || !timezone) return false
  try {
    const parts = new Intl.DateTimeFormat('en-US', { calendar: 'gregory', numberingSystem: 'latn', timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(now))
    const values = {}
    for (const part of parts) if (part.type === 'year' || part.type === 'month' || part.type === 'day') values[part.type] = part.value
    if (!values.year || !values.month || !values.day) return false
    const today = values.year + '-' + values.month + '-' + values.day
    return dayDate < today
  } catch {
    return false
  }
}