import { benchmarkData } from './data.js?v=2026-09-10-8980b597';

const caseLabels = {
  'fannkuch-redux': 'fannkuch-redux',
  'n-body': 'n-body',
  'spectral-norm': 'spectral-norm',
};

const laneLabels = { 'one-core': 'One core', 'four-core': 'Four cores' };
const runtimeMetrics = {
  wallSeconds: { label: 'Wall time', short: 'Wall', format: (value) => `${formatNumber(value)} s` },
  cpuSeconds: { label: 'CPU time', short: 'CPU', format: (value) => `${formatNumber(value)} s` },
  memoryBytes: { label: 'Peak RSS', short: 'RSS', format: formatBytes },
};
const buildMetrics = {
  wallSeconds: runtimeMetrics.wallSeconds,
  cpuSeconds: runtimeMetrics.cpuSeconds,
  memoryBytes: runtimeMetrics.memoryBytes,
  artifactBytes: { label: 'Raw application', short: 'Size', format: formatBytes },
};

const state = {
  runtimeCase: 'fannkuch-redux',
  runtimeLane: 'one-core',
  runtimeMetric: 'wallSeconds',
  buildMetric: 'wallSeconds',
};

function formatNumber(value) {
  if (value >= 100) return value.toFixed(1);
  if (value >= 10) return value.toFixed(2);
  return value.toFixed(3);
}

function formatBytes(value) {
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(value >= 10 * 1024 ** 2 ? 1 : 2)} MiB`;
  return `${(value / 1024).toFixed(value >= 100 * 1024 ? 0 : 1)} KiB`;
}

function runtimeComparison(value, best, metric) {
  const ratio = value / best;
  if (metric === 'memoryBytes') {
    return ratio === 1 ? 'lowest RSS (1.00×)' : `${ratio.toFixed(2)}× the lowest RSS`;
  }
  return ratio === 1 ? 'best time (1.00×)' : `${ratio.toFixed(2)}× the best time`;
}

const element = (name, className, text) => {
  const node = document.createElement(name);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
};

const button = (label, active, onClick) => {
  const node = element('button', '', label);
  node.type = 'button';
  node.setAttribute('aria-pressed', String(active));
  node.addEventListener('click', onClick);
  return node;
};

const evidenceLink = (url, label = 'Source') => {
  const link = element('a', 'row-evidence', label);
  link.href = url;
  link.target = '_blank';
  link.rel = 'noreferrer';
  return link;
};

function renderSummary() {
  const runtime = benchmarkData.runtime.records;
  const build = benchmarkData.build.records;
  const nativePureGoRatios = [];
  for (const caseName of Object.keys(caseLabels)) {
    for (const lane of Object.keys(laneLabels)) {
      const rows = runtime.filter((row) => row.case === caseName && row.lane === lane);
      const native = rows.find((row) => row.candidate === 'typerb-native');
      const pureGo = rows.find((row) => row.candidate === 'go');
      nativePureGoRatios.push(native.wallSeconds / pureGo.wallSeconds);
    }
  }
  const reductions = Object.keys(caseLabels).map((caseName) => {
    const rows = build.filter((row) => row.case === caseName);
    const native = rows.find((row) => row.candidate === 'typerb-native');
    const go = rows.find((row) => row.candidate === 'typerb-go');
    return (1 - native.artifactBytes / go.artifactBytes) * 100;
  });
  const cards = [
    [`${benchmarkData.runtime.retained + benchmarkData.build.retained}`, 'retained samples', 'All passed', 'total'],
    [`${Object.keys(caseLabels).length}`, 'exact programs', 'No composite score', ''],
    [`${Math.min(...nativePureGoRatios).toFixed(2)}–${Math.max(...nativePureGoRatios).toFixed(2)}×`, 'Native / Pure Go runtime', 'Parity or better is the target', 'caution'],
    [`${Math.min(...reductions).toFixed(2)}%`, 'minimum raw size reduction', 'Native applications', 'positive'],
  ];
  document.querySelector('#benchmark-summary').replaceChildren(...cards.map(([value, label, note, kind]) => {
    const card = element('article', kind ? `summary-${kind}` : '');
    card.append(element('strong', '', value), element('span', '', label), element('small', '', note));
    return card;
  }));
}

function renderControls() {
  document.querySelector('#runtime-cases').replaceChildren(...Object.entries(caseLabels).map(([id, label]) => button(label, state.runtimeCase === id, () => {
    state.runtimeCase = id;
    renderRuntime();
    renderControls();
  })));
  document.querySelector('#runtime-lanes').replaceChildren(...Object.entries(laneLabels).map(([id, label]) => button(label, state.runtimeLane === id, () => {
    state.runtimeLane = id;
    renderRuntime();
    renderControls();
  })));
  document.querySelector('#runtime-metrics').replaceChildren(...Object.entries(runtimeMetrics).map(([id, metric]) => button(metric.short, state.runtimeMetric === id, () => {
    state.runtimeMetric = id;
    renderRuntime();
    renderControls();
  })));
  document.querySelector('#build-metrics').replaceChildren(...Object.entries(buildMetrics).map(([id, metric]) => button(metric.short, state.buildMetric === id, () => {
    state.buildMetric = id;
    renderBuild();
    renderControls();
  })));
}

function renderRuntime() {
  const metric = runtimeMetrics[state.runtimeMetric];
  const rows = benchmarkData.runtime.records
    .filter((row) => row.case === state.runtimeCase && row.lane === state.runtimeLane)
    .sort((left, right) => left[state.runtimeMetric] - right[state.runtimeMetric]);
  const fastest = rows[0][state.runtimeMetric];
  const maximum = Math.max(...rows.map((row) => row[state.runtimeMetric]));
  document.querySelector('#runtime-title').textContent = `${caseLabels[state.runtimeCase]} · ${metric.label}`;
  document.querySelector('#runtime-context').textContent = laneLabels[state.runtimeLane];

  document.querySelector('#runtime-chart').replaceChildren(...rows.map((row) => {
    const value = row[state.runtimeMetric];
    const comparison = runtimeComparison(value, fastest, state.runtimeMetric);
    const item = element('div', `comparison-row candidate-${row.candidate}`);
    const label = element('div', 'candidate-label');
    label.append(element('strong', '', row.label), element('small', '', comparison));
    const track = element('div', 'bar-track');
    const fill = element('span', 'bar-fill');
    fill.style.width = `${Math.max(1.5, (value / maximum) * 100)}%`;
    track.append(fill);
    item.append(label, track, element('strong', 'metric-value', metric.format(value)));
    return item;
  }));

  document.querySelector('#runtime-table-body').replaceChildren(...rows.map((row) => {
    const tableRow = element('tr', `candidate-${row.candidate}`);
    const label = element('th', '', row.label);
    label.scope = 'row';
    const wall = element('td', '', runtimeMetrics.wallSeconds.format(row.wallSeconds));
    const cpu = element('td', '', runtimeMetrics.cpuSeconds.format(row.cpuSeconds));
    const memory = element('td', '', runtimeMetrics.memoryBytes.format(row.memoryBytes));
    const samples = element('td', '', `${row.passed}/${row.retained}`);
    const source = element('td');
    source.append(evidenceLink(row.evidence));
    tableRow.append(label, wall, cpu, memory, samples, source);
    return tableRow;
  }));
}

function buildOutcome(native, go, field) {
  if (field === 'wallSeconds') return `${(go[field] / native[field]).toFixed(2)}× faster`;
  if (field === 'cpuSeconds') return `${(native[field] / go[field]).toFixed(2)}× Go CPU time`;
  return `${((1 - native[field] / go[field]) * 100).toFixed(2)}% less`;
}

function renderBuild() {
  const metric = buildMetrics[state.buildMetric];
  const cards = Object.keys(caseLabels).map((caseName) => {
    const rows = benchmarkData.build.records.filter((row) => row.case === caseName);
    const native = rows.find((row) => row.candidate === 'typerb-native');
    const go = rows.find((row) => row.candidate === 'typerb-go');
    const maximum = Math.max(native[state.buildMetric], go[state.buildMetric]);
    const card = element('article', 'build-card');
    const heading = element('div', 'build-heading');
    heading.append(element('h3', '', caseLabels[caseName]), element('strong', 'build-outcome', buildOutcome(native, go, state.buildMetric)));
    const rowsNode = element('div', 'build-comparison');
    for (const row of [native, go]) {
      const value = row[state.buildMetric];
      const item = element('div', `build-row candidate-${row.candidate}`);
      const top = element('div', 'build-row-label');
      top.append(element('span', '', row.label), element('strong', '', metric.format(value)));
      const track = element('div', 'bar-track');
      const fill = element('span', 'bar-fill');
      fill.style.width = `${Math.max(1.5, (value / maximum) * 100)}%`;
      track.append(fill);
      item.append(top, track);
      rowsNode.append(item);
    }
    const footer = element('div', 'build-footer');
    footer.append(element('span', '', `${native.passed + go.passed}/${native.retained + go.retained} samples passed`), evidenceLink(native.evidence));
    card.append(heading, rowsNode, footer);
    return card;
  });
  document.querySelector('#build-grid').replaceChildren(...cards);
}

document.querySelector('#snapshot-date').textContent = `Evidence snapshot · ${benchmarkData.snapshot}`;
document.querySelector('#platform-note').textContent = benchmarkData.platform;
document.querySelector('#integrity-title').textContent = `All ${benchmarkData.runtime.retained + benchmarkData.build.retained} retained samples passed`;
document.querySelector('#integrity-detail').textContent = `${benchmarkData.runtime.retained} runtime · ${benchmarkData.build.retained} build`;
document.querySelector('#runtime-result-link').href = benchmarkData.runtime.result;
document.querySelector('#build-result-link').href = benchmarkData.build.result;
document.querySelector('#methodology-link').href = benchmarkData.methodology;
for (const link of document.querySelectorAll('.evidence-link, #methodology-link')) {
  link.target = '_blank';
  link.rel = 'noreferrer';
}

renderSummary();
renderControls();
renderRuntime();
renderBuild();
