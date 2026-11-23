import { test } from 'node:test';
import assert from 'node:assert/strict';
import { assessment, change, value, metrics } from '../../docs/capabilities/benchmarks/daily-model.mjs';
import { groupResults, resultFor, verdict, metricValue } from '../../docs/capabilities/benchmarks/representative-model.mjs';
test('verdicts distinguish speed, memory, build time and size without inverting ratios', () => {
  assert.deepEqual(assessment(change(80, 100), 'runtime'), { tone: 'better', label: 'Faster', detail: '20.0% less time' });
  assert.equal(assessment(change(120, 100), 'runtime').label, 'Slower');
  assert.equal(assessment(change(80, 100), 'memory').label, 'Less memory');
  assert.equal(assessment(change(120, 100), 'size').label, 'Larger');
  assert.equal(assessment(change(80, 100), 'build').label, 'Faster build');
  for (const ratio of [.96, 1, 1.04]) assert.equal(assessment(change(ratio, 1), 'runtime').label, 'About the same');
});
test('missing, failed and below-resolution comparisons never look favorable', () => {
  for (const v of [null, undefined, 0, NaN, Infinity]) {
    assert.equal(assessment(change(v, 1), 'runtime').label, 'Not comparable');
    assert.equal(assessment(change(1, v), 'runtime').tone, 'missing');
  }
  assert.equal(value({ status: 'timeout', runtime: { wallSeconds: 1 } }, metrics.runtime), null);
});
test('representative coverage keeps unmeasured and changed inputs without borrowing old results', () => {
  const input = { id: 'json-small', workload: 'json-transform', count: 16, batches: 5, size: 'small', variant: 'valid' };
  const snapshot = { inputs: [input], rows: [{ case: input.id, summaries: { native: { wallSeconds: 2 }, 'pure-go': { wallSeconds: 1 } } }] };
  const pending = { ...input, id: 'json-large', count: 1024 };
  assert.equal(resultFor(snapshot, { ...input, count: 32 }), undefined);
  assert.deepEqual(groupResults([input, pending], snapshot, 'pure-go', 'wallSeconds')[0].counts, { better: 0, worse: 1, neutral: 0, missing: 1 });
  assert.equal(groupResults([input], undefined, 'pure-go', 'wallSeconds')[0].counts.missing, 1);
  assert.equal(groupResults([input], snapshot, 'typerb-go', 'wallSeconds')[0].counts.missing, 1);
});
test('representative throughput reverses time direction and zero CPU stays unknown', () => {
  assert.equal(verdict(2, 1, 'unitsPerSecond').label, 'Higher throughput');
  assert.equal(verdict(2, 1, 'wallSeconds').label, 'Slower');
  assert.equal(verdict(0, 1, 'cpuSeconds').label, 'Not comparable');
  assert.equal(verdict(1.01, 1, 'peakRssBytes').label, 'About the same');
});
test('legacy Linux RSS is unavailable without changing valid timing or source data', () => {
  const input = { id: 'json-small', workload: 'json-transform', count: 16, batches: 5, size: 'small', variant: 'valid' };
  const row = { case: input.id, summaries: { native: { wallSeconds: 1, peakRssBytes: 60000000 }, 'pure-go': { wallSeconds: 2, peakRssBytes: 60000000 } } };
  const snapshot = { platform: 'Linux', environment: {}, inputs: [input], rows: [row] };
  assert.equal(groupResults([input], snapshot, 'pure-go', 'peakRssBytes')[0].counts.missing, 1);
  assert.equal(metricValue(snapshot, row, 'native', 'wallSeconds'), 1);
  assert.equal(row.summaries.native.peakRssBytes, 60000000);
  snapshot.environment.peakRssMethod = 'gnu-time-child-v1';
  assert.equal(metricValue(snapshot, row, 'native', 'peakRssBytes'), 60000000);
  assert.equal(metricValue({ platform: 'macOS' }, row, 'native', 'peakRssBytes'), 60000000);
});
test('language areas use geometric means, keep the weakest workload and count missing values', async () => {
  const { areaSummary, familyCoverage } = await import('../../docs/capabilities/benchmarks/daily-model.mjs');
  const row = (name, area, wall, family) => ({ case: name, area, family, status: 'pass', runtime: { wallSeconds: wall } });
  const rows = [row('a', 'Strings', 2, 'strings'), row('b', 'Strings', .5, 'strings'), row('c', 'Hash', 3, 'hashes'), row('d', 'Hash', 1)];
  const controls = { a: 1, b: 1, c: 1 };
  const summary = areaSummary(rows, r => controls[r.case] ? { status: 'pass', runtime: { wallSeconds: controls[r.case] } } : undefined, metrics.runtime);
  const strings = summary.areas.find(area => area.area === 'Strings');
  assert.equal(strings.ratio, 1);
  assert.deepEqual(strings.weakest, { case: 'a', ratio: 2 });
  const hash = summary.areas.find(area => area.area === 'Hash');
  assert.ok(Math.abs(hash.ratio - 3) < 1e-12);
  assert.deepEqual([hash.compared, hash.missing, hash.family], [1, 1, 'hashes']);
  assert.equal(summary.overall.compared, 3);
  assert.ok(Math.abs(summary.overall.ratio - Math.cbrt(3)) < 1e-12);
  assert.equal(areaSummary([row('e', 'Empty', 1)], () => undefined, metrics.runtime).areas[0].ratio, null);
  const language = { features: [{ id: 'strings', title: 'Strings and UTF-8', cases: ['x', 'y'], pending: ['p'] }],
    cases: [{ id: 'x', gaps: [] }, { id: 'y', gaps: ['repl'] }] };
  assert.deepEqual(familyCoverage(language, 'strings'), { title: 'Strings and UTF-8', probes: 2, differing: 1, pending: 1 });
  assert.equal(familyCoverage(language, 'missing'), null);
});

test('every registered representative input stays visible before a measurement', async () => {
  const { readFile } = await import('node:fs/promises');
  const suite = JSON.parse(await readFile(new URL('../../docs/capabilities/benchmarks/representative-suite.json', import.meta.url), 'utf8'));
  const groups = groupResults(suite.cases, undefined, 'pure-go', 'wallSeconds');
  assert.deepEqual(groups.flatMap(group => group.inputs.map(input => input.id)).sort(), suite.cases.map(input => input.id).sort());
  assert.equal(groups.reduce((total, group) => total + group.counts.missing, 0), suite.cases.length);
});

test('observed ranges expose overlap, sparse and zero samples without changing median screening', async () => {
  const { rangeComparison, sampleRange } = await import('../../docs/capabilities/benchmarks/representative-model.mjs');
  const range = (minimum, maximum, count = 3) => ({ minimum, maximum, count });
  const row = { sampleRanges: { native: { cpuSeconds: range(1, 4) }, 'pure-go': { cpuSeconds: range(3, 6) } } };
  assert.match(rangeComparison({}, row, 'pure-go', 'cpuSeconds'), /overlap;.*uncertain/);
  assert.equal(verdict(2, 5, 'cpuSeconds').tone, 'better');
  row.sampleRanges.native.cpuSeconds.maximum = 2;
  assert.match(rangeComparison({}, row, 'pure-go', 'cpuSeconds'), /not overlap;.*not a confidence/);
  row.sampleRanges.native.cpuSeconds.minimum = 0;
  assert.match(rangeComparison({}, row, 'pure-go', 'cpuSeconds'), /zero observations/);
  row.sampleRanges.native.cpuSeconds.count = 1;
  assert.match(rangeComparison({}, row, 'pure-go', 'cpuSeconds'), /Range unavailable/);
  assert.match(rangeComparison({}, {}, 'pure-go', 'cpuSeconds'), /Range unavailable/);
  row.sampleRanges.native.peakRssBytes = range(1, 4);
  assert.equal(sampleRange({ platform: 'Linux', environment: {} }, row, 'native', 'peakRssBytes'), undefined);
});

test('fixed profile display preserves historical inputs and lists new unmeasured cases', async () => {
  const { displayCases, profileCases } = await import('../../docs/capabilities/benchmarks/representative-model.mjs');
  const input = { id: 'json-small', workload: 'json-transform', count: 16, batches: 5, size: 'small', variant: 'valid' };
  const catalog = { cases: [input, { ...input, id: 'new-case' }], inputProfiles: {
    registered: { batchMultipliers: { small: 1 } },
    'sustained-v1': { batchMultipliers: { small: 200 }, caseMultipliers: { 'new-case': 40 } },
  } };
  assert.deepEqual(profileCases(catalog), catalog.cases);
  const longer = profileCases(catalog, 'sustained-v1');
  assert.equal(longer[0].batches, 1000);
  assert.equal(longer[1].batches, 200);
  const snapshot = { inputProfile: 'sustained-v1', inputs: [longer[0]], rows: [] };
  assert.deepEqual(displayCases(catalog, snapshot), longer);
  assert.deepEqual(displayCases(catalog, { inputs: [input] }), catalog.cases);
  const historical = { ...input, batches: 7 };
  assert.equal(displayCases(catalog, { inputs: [historical] })[0].batches, 7);
  assert.equal(resultFor(snapshot, input), undefined);
});
