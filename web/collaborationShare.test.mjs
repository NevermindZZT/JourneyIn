import assert from 'node:assert/strict'
import test from 'node:test'
import { collaborationAPIPath, collaborationTokenFromHash } from './src/collaborationShare.mjs'

const origin = 'https://journeyin.example'
const token = 'A'.repeat(43)

test('accepts only an opaque base64url collaboration token in the fragment', () => {
  assert.equal(collaborationTokenFromHash('#' + token), token)
  assert.equal(collaborationTokenFromHash(''), '')
  assert.equal(collaborationTokenFromHash('#short'), '')
  assert.equal(collaborationTokenFromHash('#' + token + '='), '')
  assert.equal(collaborationTokenFromHash('#' + 'x'.repeat(42)), '')
})

test('rewrites only the current Trip and provider operations to collaboration endpoints', () => {
  assert.equal(collaborationAPIPath('/api/v1/trips/trip-1/days/day-1/stops?day=all', 'trip-1', origin), '/api/v1/collaboration/trips/trip-1/days/day-1/stops?day=all')
  assert.equal(collaborationAPIPath('/api/v1/maps/pois/search?q=park', 'trip-1', origin), '/api/v1/collaboration/maps/pois/search?q=park')
  assert.equal(collaborationAPIPath('/api/v1/maps/reverse-geocode', 'trip-1', origin), '/api/v1/collaboration/maps/reverse-geocode')
})

test('refuses other Trips, owner APIs and cross-origin URLs', () => {
  assert.equal(collaborationAPIPath('/api/v1/trips/trip-2', 'trip-1', origin), null)
  assert.equal(collaborationAPIPath('/api/v1/trips', 'trip-1', origin), null)
  assert.equal(collaborationAPIPath('/api/v1/settings/map-keys', 'trip-1', origin), null)
  assert.equal(collaborationAPIPath('https://elsewhere.example/api/v1/trips/trip-1', 'trip-1', origin), null)
})
