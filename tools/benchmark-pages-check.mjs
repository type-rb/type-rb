import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { benchmarkData } from '../docs/capabilities/benchmarks/data.js';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const failures = [];
const fail = (message) => failures.push(message);

if (benchmarkData.schemaVersion !== 1) fail('unsupported benchmark data schema');
if (benchmarkData.runtime.records.length !== 42) fail('expected 42 runtime median rows');
if (benchmarkData.build.records.length !== 6) fail('expected 6 build median rows');
if (benchmarkData.runtime.retained !== 462) fail('expected 462 retained runtime samples');
if (benchmarkData.build.retained !== 66) fail('expected 66 retained build samples');

for (const row of [...benchmarkData.runtime.records, ...benchmarkData.build.records]) {
  if (row.status !== 'pass') fail(`${row.case}/${row.candidate} does not pass`);
  if (row.retained !== row.passed) fail(`${row.case}/${row.candidate} has missing samples`);
  for (const field of ['wallSeconds', 'cpuSeconds', 'memoryBytes']) {
    if (!Number.isFinite(row[field]) || row[field] <= 0) fail(`${row.case}/${row.candidate} has invalid ${field}`);
  }
  if (!row.evidence.startsWith('https://github.com/type-rb/type-rb-native/blob/main/results/')) {
    fail(`${row.case}/${row.candidate} has a non-public evidence URL`);
  }
}

const html = await readFile(resolve(root, 'docs/capabilities/benchmarks/comparison.html'), 'utf8');
for (const required of ['TypeRB Native Benchmarks', 'id="runtime-chart"', 'id="build-grid"', './app.js']) {
  if (!html.includes(required)) fail(`benchmark page is missing ${required}`);
}
if (!html.includes('Shorter bars are better')) fail('benchmark page does not explain bar direction');
if (/\d+\.\d+[×x] to \d+\.\d+[×x] slower than Pure Go/.test(html)) {
  fail('benchmark introduction duplicates a generated runtime range');
}

const app = await readFile(resolve(root, 'docs/capabilities/benchmarks/app.js'), 'utf8');
for (const required of ['× the best time', '× the lowest RSS']) {
  if (!app.includes(required)) fail(`benchmark comparison copy is missing ${required}`);
}
if (app.includes('× fastest')) fail('benchmark comparison uses the ambiguous “× fastest” wording');
if (!app.includes('cpuSeconds: runtimeMetrics.cpuSeconds')) fail('build comparison omits CPU time');

if (failures.length > 0) {
  failures.forEach((message) => console.error(`FAIL ${message}`));
  process.exitCode = 1;
} else {
  console.log('PASS benchmark Pages structure and evidence');
}

for (const page of ['index.html', 'trends.html']) {
  const daily = await readFile(resolve(root, 'docs/capabilities/benchmarks', page), 'utf8');
  for (const required of ['id="status"', 'id="metric"', './daily.js', 'comparison.html', 'Current performance']) {
    if (!daily.includes(required)) throw new Error(`${page} is missing ${required}`);
  }
}

const weekly = await readFile(resolve(root, 'docs/capabilities/benchmarks/weekly.html'), 'utf8');
for (const required of ['id="status"', 'id="metric"', 'id="case"', './weekly.js', 'Current performance']) {
  if (!weekly.includes(required)) throw new Error(`weekly.html is missing ${required}`);
}
for (const name of ['index.html', 'trends.html', 'weekly.html']) {
  const page = await readFile(resolve(root, 'docs/capabilities/benchmarks', name), 'utf8');
  if (page.includes('<select')) throw new Error(`${name} must use visible comparison buttons`);
}

const representative = await readFile(resolve(root, 'docs/capabilities/benchmarks/representative.html'), 'utf8');
for (const required of ['Application workloads', 'id="families"', 'id="attempt"', './representative.js', 'on demand', 'Remaining gaps']) {
  if (!representative.includes(required)) throw new Error(`representative.html is missing ${required}`);
}
for (const name of ['index.html', 'trends.html', 'weekly.html']) {
  const page = await readFile(resolve(root, 'docs/capabilities/benchmarks', name), 'utf8');
  if (!page.includes('representative.html')) throw new Error(`${name} must link the application coverage`);
}
