export function collaborationTokenFromHash(hash) {
  const token = String(hash || '').replace(/^#/, '')
  return /^[A-Za-z0-9_-]{43}$/.test(token) ? token : ''
}

export function collaborationAPIPath(input, tripID, origin) {
  if (!tripID) return null
  const raw = typeof input === 'string'
    ? input
    : input instanceof URL
      ? input.toString()
      : typeof Request !== 'undefined' && input instanceof Request
        ? input.url
        : String(input || '')
  let url
  try { url = new URL(raw, origin) } catch { return null }
  if (url.origin !== origin) return null

  const tripPath = '/api/v1/trips/' + encodeURIComponent(tripID)
  if (url.pathname === tripPath || url.pathname.startsWith(tripPath + '/')) {
    return '/api/v1/collaboration/trips/' + encodeURIComponent(tripID) + url.pathname.slice(tripPath.length) + url.search
  }
  if (url.pathname === '/api/v1/maps/pois/search' || url.pathname === '/api/v1/maps/reverse-geocode') {
    return '/api/v1/collaboration/maps' + url.pathname.slice('/api/v1/maps'.length) + url.search
  }
  return null
}
