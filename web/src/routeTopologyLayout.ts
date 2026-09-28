export type PosterPoint = { id: string; title: string; displayIndex: number; x: number; y: number; isFirst: boolean; isLast: boolean; dayBoundary?: boolean }
export type PosterNode = PosterPoint & { count: number }
export type PosterLabel = { id: string; text: string; x: number; y: number; width: number; height: number; nodeID: string }
type Rect = { x: number; y: number; width: number; height: number }

export function clusterPosterPoints(points: PosterPoint[], distance = 13): PosterNode[] {
  const clusters: Array<{ anchor: PosterPoint; members: PosterPoint[] }> = []
  for (const point of points) {
    const cluster = clusters.find(item => Math.hypot(item.anchor.x - point.x, item.anchor.y - point.y) < distance)
    if (cluster) cluster.members.push(point)
    else clusters.push({ anchor: point, members: [point] })
  }
  return clusters.map(({ members }) => {
    const representative = members.find(point => point.isFirst) || members.find(point => point.isLast) || members.find(point => point.dayBoundary) || members[0]
    return { ...representative, count: members.length, isFirst: members.some(point => point.isFirst), isLast: members.some(point => point.isLast), dayBoundary: members.some(point => point.dayBoundary) }
  })
}

function posterTitle(title: string): string {
  const chars = Array.from(title.trim())
  const wide = chars.some(char => /[^\u0000-\u007f]/.test(char))
  const max = wide ? 8 : 16
  return chars.length > max ? chars.slice(0, max - 1).join('') + '…' : title.trim()
}

function overlaps(a: Rect, b: Rect, padding = 4): boolean {
  return a.x < b.x + b.width + padding && a.x + a.width + padding > b.x && a.y < b.y + b.height + padding && a.y + a.height + padding > b.y
}

export function layoutPosterLabels(nodes: PosterNode[], width: number, height: number, density: 'simple' | 'detailed', routePixels: Array<[number, number]> = []): PosterLabel[] {
  const cap = density === 'detailed' ? 12 : 6
  const occupied: Rect[] = []
  const labels: PosterLabel[] = []
  const prioritized = nodes.filter(node => node.isFirst || node.isLast)
  const remaining = nodes.filter(node => !prioritized.includes(node))
  while (remaining.length) {
    const dayStarts = remaining.filter(node => node.dayBoundary)
    const pool = dayStarts.length ? dayStarts : remaining
    const distance = (node: PosterNode) => prioritized.length ? Math.min(...prioritized.map(item => Math.abs(item.displayIndex - node.displayIndex))) : 0
    const next = [...pool].sort((a, b) => distance(b) - distance(a) || a.displayIndex - b.displayIndex)[0]
    prioritized.push(next)
    remaining.splice(remaining.indexOf(next), 1)
  }
  for (const node of prioritized) {
    if (labels.length >= cap) break
    const text = posterTitle(node.title)
    if (!text) continue
    const labelWidth = Math.min(130, Math.max(42, Array.from(text).reduce((sum, ch) => sum + (/[^\u0000-\u007f]/.test(ch) ? 10 : 6), 0) + 18))
    const labelHeight = 19
    const options: Rect[] = [
      { x: node.x + 17, y: node.y - 10, width: labelWidth, height: labelHeight },
      { x: node.x - 17 - labelWidth, y: node.y - 10, width: labelWidth, height: labelHeight },
      { x: node.x - labelWidth / 2, y: node.y - 35, width: labelWidth, height: labelHeight },
      { x: node.x - labelWidth / 2, y: node.y + 17, width: labelWidth, height: labelHeight },
    ]
    const candidates = options.filter(box => box.x >= 9 && box.x + box.width <= width - 9 && box.y >= 39 && box.y + box.height <= height - 24 && !occupied.some(other => overlaps(box, other)) && !nodes.some(other => other.id !== node.id && other.x >= box.x - 9 && other.x <= box.x + box.width + 9 && other.y >= box.y - 9 && other.y <= box.y + box.height + 9))
    if (!candidates.length) continue
    const routeOverlap = (box: Rect) => routePixels.reduce((count, [x, y]) => count + Number(x >= box.x - 3 && x <= box.x + box.width + 3 && y >= box.y - 3 && y <= box.y + box.height + 3), 0)
    candidates.sort((a, b) => routeOverlap(a) - routeOverlap(b))
    const chosen = candidates[0]
    occupied.push(chosen)
    labels.push({ ...chosen, id: node.id, nodeID: node.id, text })
  }
  return labels
}

type RouteGeometry = Array<[number, number] | { lng: number; lat: number; crs?: string }>
export type PosterRouteLeg = { snapshots?: Array<{ provider?: string; coordinate_system?: string; mode?: string; geometry?: RouteGeometry }> }
export function selectPosterRoutes(legs: PosterRouteLeg[], preferredProvider = '', preferredMode = ''): { provider: string; crs: string; paths: Array<Array<[number, number]>> } {
  let provider = preferredProvider
  let crs = ''
  const paths: Array<Array<[number, number]>> = []
  for (const leg of legs) {
    const snapshot = leg.snapshots?.find(snap => {
      if (!snap.provider || !['wgs84', 'gcj02', 'bd09ll'].includes(snap.coordinate_system || '')) return false
      if (provider && snap.provider !== provider) return false
      if (preferredMode && snap.mode && snap.mode !== preferredMode) return false
      return Array.isArray(snap.geometry) && snap.geometry.length > 1 && (!crs || snap.coordinate_system === crs)
    })
    if (!snapshot?.geometry) continue
    const selectedCRS = snapshot.coordinate_system!
    const coordinates: Array<[number, number]> = []
    for (const point of snapshot.geometry) {
      const lng = Array.isArray(point) ? point[0] : point.lng
      const lat = Array.isArray(point) ? point[1] : point.lat
      if (!Number.isFinite(lng) || !Number.isFinite(lat) || (!Array.isArray(point) && point.crs && point.crs !== selectedCRS)) { coordinates.length = 0; break }
      coordinates.push([lng, lat])
    }
    if (coordinates.length < 2) continue
    provider ||= snapshot.provider!
    crs ||= selectedCRS
    paths.push(coordinates)
  }
  return { provider, crs, paths }
}
