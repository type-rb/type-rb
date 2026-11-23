import { metrics, comparisons, value, valid, change, assessment, areaSummary, familyCoverage } from './daily-model.mjs';
import { ordinaryLanguage } from '../ordinary-language.js';
const $ = id => document.getElementById(id);
const node = (tag, text, className) => {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  if (className) element.className = className;
  return element;
};
const link = (label, url) => {
  const anchor = node('a', label);
  if (typeof url === 'string' && /^https:\/\/github\.com\/type-rb\/type-rb-native\/actions\/runs\/\d+$/.test(url)) anchor.href = url;
  return anchor;
};
const format = (number, metric) => !valid(number) ? 'Unavailable' : metric.unit === 'bytes' ?
  `${(number / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} KiB` :
  number < .1 ? `${(number * 1000).toFixed(2)} ms` : `${number.toFixed(3)} s`;
const rowFor = (snapshot, caseName, role) => snapshot.rows.find(row => row.case === caseName && row.role === role);
let state;
let metricKey = 'runtime', comparisonKey = 'pure-go', caseName = 'fannkuch-redux';
const reference = (snapshot, name) => comparisonKey === 'previous' && snapshot.roles.previous.revision === snapshot.roles.native.revision ?
  undefined : rowFor(snapshot, name, comparisonKey);

function choices(id, items, selected, onSelect) {
  const group = $(id);
  for (const [key, label] of items) {
    const button = node('button', label);
    button.type = 'button'; button.setAttribute('aria-pressed', String(key === selected));
    button.addEventListener('click', () => {
      for (const sibling of group.children) sibling.setAttribute('aria-pressed', String(sibling === button));
      onSelect(key);
    });
    group.append(button);
  }
}
function unavailable(snapshot, row, name) {
  if (row) return row.status === 'pass' ? 'Below resolution' : `Failed: ${row.status}`;
  if (comparisonKey === 'previous') return 'No previous measurement in this series';
  if (comparisonKey === 'pure-go') return snapshot.roles['pure-go'] ? 'No hand-written Pure Go program for this workload' : 'Pure Go measurement pending';
  if (comparisonKey === 'baseline' && snapshot.baselineCases && !snapshot.baselineCases.includes(name)) return 'Uses features newer than the frozen baseline';
  return 'Not measured';
}
function verdictCell(current, control, metric, missing) {
  const verdict = assessment(change(current, control), metricKey);
  const cell = node('td'); cell.dataset.label = 'Assessment';
  cell.append(node('span', verdict.label, `verdict ${verdict.tone}`));
  cell.append(node('small', verdict.detail || missing));
  return cell;
}
function numericCell(row, metric, label) {
  const cell = node('td', row?.status === 'pass' ? format(value(row, metric), metric) : row ? `Failed: ${row.status}` : '—');
  cell.dataset.label = label;
  if (metricKey === 'runtime' && row?.status === 'pass' && row.runtime) {
    cell.append(node('small', `${format(row.runtime.wallMin, metric)}–${format(row.runtime.wallMax, metric)}`));
  }
  return cell;
}
function renderCurrent() {
  const metric = metrics[metricKey], snapshot = state.latest;
  $('result-body').replaceChildren(); $('verdict-summary').replaceChildren();
  $('reference-heading').textContent = comparisons[comparisonKey];
  if (!snapshot) return;
  const counts = { better: 0, neutral: 0, worse: 0, missing: 0 };
  for (const row of snapshot.rows.filter(row => row.role === 'native')) {
    const tr = node('tr');
    const title = node('th', row.case); title.scope = 'row';
    title.append(node('small', row.area));
    if (row.coverage) title.append(node('small', row.coverage));
    tr.append(title, numericCell(row, metric, 'Native'));
    const ref = reference(snapshot, row.case), current = value(row, metric), control = value(ref, metric);
    tr.append(numericCell(ref, metric, comparisons[comparisonKey]));
    const verdict = assessment(change(current, control), metricKey);
    counts[verdict.tone]++;
    tr.append(verdictCell(current, control, metric, row.status === 'pass' ? unavailable(snapshot, ref, row.case) : `Native failed: ${row.status}`));
    $('result-body').append(tr);
  }
  const compared = counts.better + counts.neutral + counts.worse;
  const intro = node('div', undefined, 'summary-intro');
  intro.append(node('span', `${metric.label} vs ${comparisons[comparisonKey]}`, 'control-label'),
    node('strong', `${compared} workloads compared`), node('small', `${counts.missing} without a comparable value · lower is better`));
  $('verdict-summary').append(intro);
  for (const tone of ['better', 'neutral', 'worse']) {
    const card = node('div', undefined, `summary-count ${tone}`);
    card.append(node('strong', String(counts[tone])), node('span', assessment(tone === 'better' ? -.1 : tone === 'worse' ? .1 : 0, metricKey).label));
    $('verdict-summary').append(card);
  }
  renderAreas(snapshot, metric);
}
function renderAreas(snapshot, metric) {
  const summary = areaSummary(snapshot.rows.filter(row => row.role === 'native'), row => reference(snapshot, row.case), metric);
  $('area-body').replaceChildren();
  const overall = assessment(summary.overall.ratio === null ? null : summary.overall.ratio - 1, metricKey);
  $('area-overall').textContent = summary.overall.compared ?
    `All ${summary.overall.compared} compared workloads: ${overall.label}${overall.detail ? ` (${overall.detail}, geometric mean)` : ''}.` :
    'No workload has a comparable value for this selection.';
  for (const area of summary.areas) {
    const tr = node('tr');
    const title = node('th', area.area); title.scope = 'row';
    const verdict = assessment(area.ratio === null ? null : area.ratio - 1, metricKey);
    const verdictCellNode = node('td'); verdictCellNode.dataset.label = 'Assessment';
    verdictCellNode.append(node('span', verdict.label, `verdict ${verdict.tone}`), node('small', verdict.detail));
    const compared = node('td', `${area.compared} of ${area.compared + area.missing}`); compared.dataset.label = 'Compared';
    const weakest = node('td', area.weakest && area.compared > 1 ? `${area.weakest.case} (${assessment(area.weakest.ratio - 1, metricKey).detail})` : '—');
    weakest.dataset.label = 'Weakest workload';
    const coverage = node('td'); coverage.dataset.label = 'Language coverage';
    const family = area.family && familyCoverage(ordinaryLanguage, area.family);
    if (family) {
      const anchor = node('a', `${family.differing} of ${family.probes} probes differ`);
      anchor.href = `../?family=${encodeURIComponent(area.family)}#ordinary-language`;
      coverage.append(anchor);
      if (family.pending) coverage.append(node('small', `${family.pending} uncovered contracts`));
    } else coverage.textContent = '—';
    tr.append(title, verdictCellNode, compared, weakest, coverage);
    $('area-body').append(tr);
  }
}
function renderCompilerSelf() {
  const current = state.latest?.compilerSelf;
  if (!current || !$('compiler-self')) return;
  $('compiler-self').hidden = false;
  const summary = $('compiler-self-summary'); summary.replaceChildren();
  const history = $('compiler-self-history'); history.replaceChildren();
  const intro = node('div', undefined, 'summary-intro');
  intro.append(node('span', 'Current compiler', 'control-label'),
    node('strong', current.status === 'pass' ? 'Self-compilation measured' : `Measurement failed: ${current.status}`),
    node('small', `${state.latest.revision.slice(0, 8)} · same-revision compiler source`));
  summary.append(intro);
  if (current.status === 'pass') {
    for (const [label, result] of [
      ['Self build', format(current.build.wallSeconds, { unit: 'seconds' })],
      ['QBE size', `${(current.ir.bytes / 1048576).toFixed(2)} MiB`],
      ['Compiler binary', `${(current.binaryBytes / 1048576).toFixed(2)} MiB`],
    ]) {
      const card = node('div', undefined, 'summary-count neutral');
      card.append(node('strong', result), node('span', label)); summary.append(card);
    }
  }
  for (const snapshot of state.history.filter(item => item.series === state.latest.series && item.compilerSelf).slice(-10).reverse()) {
    const self = snapshot.compilerSelf, tr = node('tr');
    for (const [label, text] of [
      ['Measured', new Date(snapshot.at).toLocaleDateString()],
      ['Revision', snapshot.revision.slice(0, 8)],
      ['Self build', self.build ? format(self.build.wallSeconds, { unit: 'seconds' }) : `Failed: ${self.status}`],
      ['QBE size', self.ir ? `${(self.ir.bytes / 1048576).toFixed(2)} MiB` : '—'],
      ['QBE emission', self.ir ? format(self.ir.wallSeconds, { unit: 'seconds' }) : '—'],
    ]) {
      const cell = node('td', text); cell.dataset.label = label; tr.append(cell);
    }
    history.append(tr);
  }
}
function renderHistory() {
  const metric = metrics[metricKey];
  const history = state.history.filter(snapshot => snapshot.series === state.latest?.series);
  const points = [];
  $('history-body').replaceChildren();
  for (const [index, snapshot] of history.entries()) {
    const native = rowFor(snapshot, caseName, 'native'), ref = reference(snapshot, caseName);
    const current = value(native, metric), control = value(ref, metric);
    const ratio = change(current, control);
    const tr = node('tr');
    for (const [label, text] of [['Measured', new Date(snapshot.at).toLocaleString()], ['Revision', snapshot.revision.slice(0, 8)]]) {
      const cell = node('td', text); cell.dataset.label = label; tr.append(cell);
    }
    tr.append(numericCell(native, metric, 'Native'), verdictCell(current, control, metric, unavailable(snapshot, ref, caseName)));
    const evidence = node('td'); evidence.dataset.label = 'Evidence'; evidence.append(link('Measurement ↗', snapshot.runUrl)); tr.append(evidence);
    $('history-body').prepend(tr);
    points.push(ratio === null ? null : { index, ratio: ratio * 100 });
  }
  const svg = $('trend-chart'); svg.replaceChildren();
  const measured = points.filter(Boolean);
  $('trend-context').textContent = `${caseName} · ${metric.label} vs ${comparisons[comparisonKey]}. Negative means less ${metric.quantity}. Only the current series is shown.`;
  svg.hidden = !measured.length;
  if (!measured.length) return;
  const low = Math.min(0, ...measured.map(p => p.ratio)) - 5, high = Math.max(0, ...measured.map(p => p.ratio)) + 5;
  const x = index => history.length === 1 ? 365 : 65 + index * 605 / (history.length - 1);
  const y = ratio => 215 - (ratio - low) * 185 / (high - low);
  const svgNode = (name, attrs, text) => {
    const item = document.createElementNS('http://www.w3.org/2000/svg', name);
    for (const [key, value] of Object.entries(attrs)) item.setAttribute(key, value);
    if (text) item.textContent = text; svg.append(item); return item;
  };
  svgNode('line', { x1: 65, x2: 670, y1: y(0), y2: y(0), stroke: '#87978f', 'stroke-dasharray': '4 4' });
  svgNode('text', { x: 5, y: y(0) + 4 }, '0%');
  let previous;
  for (const point of points) {
    if (point && previous) svgNode('line', { x1: x(previous.index), y1: y(previous.ratio), x2: x(point.index), y2: y(point.ratio), stroke: '#1b7758', 'stroke-width': 3 });
    if (point) svgNode('circle', { cx: x(point.index), cy: y(point.ratio), r: 4, fill: '#1b7758' });
    previous = point;
  }
  svgNode('text', { x: 65, y: 248 }, history.length === 1 ? 'First snapshot — trends need another measurement' : 'Oldest → newest · one position per snapshot');
}
function render() {
  $('metric-description').textContent = metrics[metricKey].description + ' “About the same” means a difference below 5%; it is not a statistical conclusion.';
  if (document.body.dataset.view === 'history') renderHistory(); else { renderCurrent(); renderCompilerSelf(); }
}
async function main() {
  const response = await fetch('./daily-state.json', { cache: 'no-store' });
  if (!response.ok) throw new Error('Could not load the last daily measurement.');
  state = await response.json();
  if (state.schemaVersion !== 1 || !Array.isArray(state.history)) throw new Error('Unsupported measurement data.');
  const snapshot = state.latest;
  $('status').replaceChildren();
  if (snapshot) {
    $('status').append(node('strong', `Measured ${new Date(snapshot.at).toLocaleString()}`));
    $('status').append(node('div', `${snapshot.revision.slice(0, 8)} · ${state.pendingChanges ? 'New performance changes are awaiting measurement.' : 'Up to date with performance changes.'}`));
    $('evidence').replaceChildren(link('Measurement run and raw evidence ↗', snapshot.runUrl));
    $('context').textContent = `${snapshot.platform} · 1 logical CPU · ${snapshot.samples} runtime samples · ${snapshot.buildSamples} build samples. Ranges show minimum–maximum.`;
    $('identity').textContent = `Frozen Native: ${snapshot.roles.baseline.revision.slice(0, 8)} · Previous Native: ${snapshot.roles.previous.revision.slice(0, 8)} · TypeRB Go: ${snapshot.roles['typerb-go'].revision.slice(0, 8)} · Pure Go: ${snapshot.roles['pure-go']?.version ?? 'not yet measured'}`;
  } else {
    $('status').append(node('strong', 'First daily measurement pending'), node('div', 'Daily values appear after a completed measurement. The detailed comparison remains available.'));
    $('results').hidden = true;
  }
  const attempt = state.attempt;
  if (attempt && ['infrastructure-failure', 'measured-with-failures', 'running'].includes(attempt.status)) {
    $('status').classList.add('warning');
    $('status').append(node('div', `Latest attempt: ${attempt.status} · ${attempt.revision.slice(0, 8)}. Failed or missing measurements are not passing results.`), link('Inspect latest attempt', attempt.runUrl));
  }
  choices('metric', Object.entries(metrics).map(([key, m]) => [key, m.label]), metricKey, key => { metricKey = key; render(); });
  choices('comparison', Object.entries(comparisons), comparisonKey, key => { comparisonKey = key; render(); });
  if (document.body.dataset.view === 'history') {
    const cases = snapshot?.rows.filter(row => row.role === 'native').map(row => [row.case, row.case]) ?? [];
    caseName = cases[0]?.[0] ?? caseName;
    choices('case', cases, caseName, key => { caseName = key; render(); });
  }
  render();
}
main().catch(error => { $('status').textContent = error.message; $('status').classList.add('warning'); });

// A weekly-data failure must never hide or block the daily results.
if ($('weekly-summary')) fetch('./weekly-state.json', { cache: 'no-store' }).then(r => {
  if (!r.ok) throw new Error('Weekly summary unavailable');
  return r.json();
}).then(weekly => {
  if (weekly.latest?.profile === 'weekly') {
    const snapshot = weekly.latest;
    $('weekly-summary').append(node('span', ` · Measured ${new Date(snapshot.at).toLocaleDateString()} · ${snapshot.revision.slice(0, 8)} · ${snapshot.status}${weekly.pendingChanges ? ' · newer changes pending' : ''}${weekly.publicationWarning ? ' · weekly data retrieval delayed' : ''}`));
  }
}).catch(() => {});
