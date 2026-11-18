import test from 'node:test'
import assert from 'node:assert/strict'
import { change, localInput, summarize, validatePeriods } from '../app/utils/comparison.ts'

const now = Date.UTC(2026, 10, 18, 12), before = { from: now - 120000, to: now - 60000 }, after = { from: now - 60000, to: now }
const point = (cpu, count, extra = {}) => ({ time: new Date(before.from).toISOString(), cpu, memory: null, disk: null, rx: null, tx: null, count, ...extra })

test('weighted mean respects bucket sample counts and preserves peaks', () => {
  const s = summarize([point(10, 10, { cpu_max: 40 }), point(50, 30, { cpu_max: 99 }), point(null, 10)], 'cpu', before)
  assert.equal(s.mean, 40); assert.equal(s.max, 99); assert.equal(s.samples, 40); assert.equal(s.coverage, 40 / 60)
})
test('missing data stays unavailable and zero is a valid measurement', () => {
  assert.equal(summarize([point(null, 20)], 'cpu', before).mean, null)
  assert.equal(summarize([point(0, 20)], 'cpu', before).mean, 0)
  assert.equal(summarize([], 'cpu', before).coverage, 0)
})
test('zero baseline does not produce Infinity or a fictitious relative percentage', () => {
  assert.deepEqual(change(0, 10), { absolute: 10, relative: null })
  assert.deepEqual(change(0, 0), { absolute: 0, relative: 0 })
  assert.deepEqual(change(null, 10), { absolute: null, relative: null })
  assert.deepEqual(change(20, 30), { absolute: 10, relative: 50 })
})
test('equal consecutive periods are valid; future, expired, reversed and unequal ranges fail', () => {
  assert.equal(validatePeriods(before, after, now), '')
  for (const [a, b] of [
    [{ from: NaN, to: now }, after], [{ from: now, to: now - 1 }, after],
    [before, { ...after, to: now + 60000 }], [{ from: now - 90000000, to: now - 89940000 }, after],
    [before, { ...after, from: after.from - 60000 }], [after, before]
  ]) assert.notEqual(validatePeriods(a, b, now), '')
})
test('local datetime values round-trip without an implicit UTC conversion', () => {
  const local = localInput(now)
  assert.equal(new Date(local).getTime(), now)
})
