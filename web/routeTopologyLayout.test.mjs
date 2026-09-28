import test from 'node:test'
import assert from 'node:assert/strict'
import { clusterPosterPoints, layoutPosterLabels, selectPosterRoutes } from './src/routeTopologyLayout.ts'

test('dense nodes aggregate without moving route geometry or original coordinates', () => {
  const nodes = clusterPosterPoints([
    { id: 'a', title: '起点', displayIndex: 1, x: 200, y: 100, isFirst: true, isLast: false },
    { id: 'b', title: '附近点', displayIndex: 2, x: 206, y: 104, isFirst: false, isLast: false },
    { id: 'c', title: '终点', displayIndex: 3, x: 390, y: 170, isFirst: false, isLast: true },
  ])
  assert.equal(nodes.length, 2)
  assert.equal(nodes[0].count, 2)
  assert.deepEqual([nodes[0].x, nodes[0].y], [200, 100])
  assert.equal(nodes[1].isLast, true)
})

test('simple labels have priority and never overlap or escape poster canvas', () => {
  const nodes = Array.from({ length: 32 }, (_, i) => ({ id: 's' + i, title: '规划地点' + i, displayIndex: i + 1, x: 55 + (i % 8) * 56, y: 72 + Math.floor(i / 8) * 62, isFirst: i === 0, isLast: i === 31, count: 1, dayBoundary: i % 8 === 0 }))
  const labels = layoutPosterLabels(nodes, 512, 340, 'simple')
  assert.ok(labels.length <= 6)
  assert.ok(labels.some(label => label.id === 's0'))
  assert.ok(labels.some(label => label.id === 's31'))
  for (const label of labels) {
    assert.ok(label.x >= 9 && label.x + label.width <= 503)
    assert.ok(label.y >= 39 && label.y + label.height <= 316)
    for (const other of labels) {
      if (other.id === label.id) continue
      assert.ok(label.x + label.width + 4 <= other.x || other.x + other.width + 4 <= label.x || label.y + label.height + 4 <= other.y || other.y + other.height + 4 <= label.y)
    }
  }
  assert.ok(layoutPosterLabels(nodes, 512, 340, 'detailed').length >= labels.length)
})

test('route rendering uses real matching provider and CRS snapshots only', () => {
  const valid = { provider: 'amap', coordinate_system: 'gcj02', mode: 'walking', geometry: [[120, 30], [120.1, 30.1]] }
  const mismatched = { provider: 'baidu', coordinate_system: 'bd09ll', geometry: [[116, 39], [116.1, 39.1]] }
  const legs = [{ snapshots: [mismatched, valid] }, { snapshots: [{ provider: 'amap', coordinate_system: 'bd09ll', geometry: [[121, 31], [121.1, 31.1]] }] }, { snapshots: [] }]
  const route = selectPosterRoutes(legs, 'amap', 'walking')
  assert.equal(route.provider, 'amap')
  assert.equal(route.crs, 'gcj02')
  assert.deepEqual(route.paths, [[[120, 30], [120.1, 30.1]]])
  assert.deepEqual(selectPosterRoutes([{ snapshots: [] }]).paths, [])
})
