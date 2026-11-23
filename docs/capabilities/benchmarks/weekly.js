import { metrics, value, change, assessment } from './daily-model.mjs';
const $ = id => document.getElementById(id);
const labels = { native: 'TypeRB Native', 'pure-go': 'Pure Go', c: 'C', cpp: 'C++', rust: 'Rust', java: 'Java' };
const node = (tag, text, className) => { const el = document.createElement(tag); el.textContent = text; if (className) el.className = className; return el; };
let state, workload = 'fannkuch-redux', metricKey = 'runtime';
const format = n => n === null || !Number.isFinite(n) || n <= 0 ? 'Unavailable' : metricKey === 'memory' ? `${(n / 1024).toFixed(1)} KiB` : `${n.toFixed(3)} s`;
function render() {
  const snapshot = state.latest; $('result-body').replaceChildren();
  if (!snapshot) return;
  const metric = metrics[metricKey], rows = snapshot.rows.filter(r => r.case === workload);
  const current = value(rows.find(r => r.role === 'native'), metric);
  $('metric-description').textContent = `${metric.description} Assessments describe Native relative to each language. Differences under 5% are screening-neutral.`;
  for (const row of rows) {
    const tr = document.createElement('tr'), title = node('th', labels[row.role]); title.scope = 'row';
    const number = node('td', row.status === 'pass' ? format(value(row, metric)) : `Failed: ${row.status}`); number.dataset.label = metric.label;
    if (metricKey === 'runtime' && row.runtime) number.append(node('small', `${format(row.runtime.wallMin)}–${format(row.runtime.wallMax)}`));
    const cell = document.createElement('td'); cell.dataset.label = 'Native assessment';
    if (row.role === 'native') cell.append(node('span', 'Current Native', 'verdict neutral'));
    else {
      const verdict = assessment(change(current, value(row, metric)), metricKey);
      cell.append(node('span', verdict.tone === 'missing' ? verdict.label : `Native: ${verdict.label.toLowerCase()}`, `verdict ${verdict.tone}`), node('small', verdict.detail));
    }
    tr.append(title, number, cell); $('result-body').append(tr);
  }
}
function buttons(id, entries, selected, change) {
  for (const [key, label] of entries) {
    const button = node('button', label); button.type = 'button'; button.setAttribute('aria-pressed', String(key === selected));
    button.addEventListener('click', () => { for (const b of $(id).children) b.setAttribute('aria-pressed', String(b === button)); change(key); render(); });
    $(id).append(button);
  }
}
async function main() {
  const response = await fetch('./weekly-state.json', { cache: 'no-store' });
  if (!response.ok) throw new Error('Could not load the weekly comparison.');
  state = await response.json();
  if (state.schemaVersion !== 1 || (state.latest && state.latest.profile !== 'weekly')) throw new Error('Unsupported weekly data.');
  const snapshot = state.latest;
  $('status').replaceChildren();
  if (snapshot) {
    $('status').append(node('strong', `Measured ${new Date(snapshot.at).toLocaleString()}`), node('div', `${snapshot.revision.slice(0, 8)} · ${state.pendingChanges ? 'New performance changes await the next weekly run.' : 'No unmeasured performance changes at the last update.'}`));
    $('context').textContent = `${snapshot.platform} · 1 logical CPU · ${snapshot.samples} fresh-process samples · Native and all other languages measured together.`;
    if (/^https:\/\/github\.com\/type-rb\/type-rb-native\/actions\/runs\/\d+$/.test(snapshot.runUrl)) {
      const anchor = node('a', 'Measurement and raw evidence ↗'); anchor.href = snapshot.runUrl; $('evidence').append(anchor);
    }
    for (const [role, identity] of Object.entries(snapshot.roles)) $('identities').append(node('li', `${labels[role]}: ${identity.version ?? identity.revision}`));
  } else { $('status').textContent = 'First weekly comparison pending. The dated detailed comparison remains available.'; $('results').hidden = true; }
  if (state.publicationWarning) { $('status').classList.add('warning'); $('status').append(node('div', state.publicationWarning)); }
  if (state.attempt && state.attempt.status !== 'measured') {
    $('status').classList.add('warning'); $('status').append(node('div', `Latest attempt: ${state.attempt.status}. Missing or failed observations are not passing results.`));
  }
  buttons('metric', ['runtime', 'memory'].map(k => [k, metrics[k].label]), metricKey, k => { metricKey = k; });
  buttons('case', [['fannkuch-redux', 'fannkuch-redux · 10'], ['n-body', 'n-body · 1,000,000'], ['spectral-norm', 'spectral-norm · 5,500']], workload, k => { workload = k; });
  render();
}
main().catch(error => { $('status').textContent = error.message; $('status').classList.add('warning'); });
