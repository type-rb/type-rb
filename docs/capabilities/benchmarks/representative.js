import { metrics, roles, positive, resultFor, verdict, groupResults, metricValue, invalidRss, sampleRange, rangeComparison, displayCases } from './representative-model.mjs';

const $ = id => document.getElementById(id);
const node = (tag, text, className) => {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  if (className) element.className = className;
  return element;
};
const link = (text, href) => {
  const element = node('a', text);
  if (/^https:\/\/github\.com\/type-rb\/type-rb-native\/actions\/runs\/\d+$/.test(href)) element.href = href;
  return element;
};
const date = value => new Date(value).toLocaleString();
let metricKey = 'wallSeconds', roleKey = 'pure-go', cases = [], state = { history: [] };

function format(value) {
  if (!positive(value)) return 'Unavailable';
  if (metrics[metricKey].unit === 'bytes') return `${(value / 1048576).toFixed(2)} MiB`;
  if (metrics[metricKey].unit === 'items/s') return `${Math.round(value).toLocaleString()} items/s`;
  return value < 1 ? `${(value * 1000).toFixed(3)} ms` : `${value.toFixed(3)} s`;
}

function choices(id, entries, current, update) {
  for (const [key, entry] of Object.entries(entries)) {
    const button = node('button', typeof entry === 'string' ? entry : entry.label);
    button.type = 'button';
    button.setAttribute('aria-pressed', String(key === current));
    button.onclick = () => {
      update(key);
      for (const item of $(id).children) item.setAttribute('aria-pressed', String(item === button));
      render();
    };
    $(id).append(button);
  }
}

function cell(text, label) {
  const result = node('td', text);
  result.dataset.label = label;
  return result;
}

function render() {
  const open = new Set([...$('families').children].filter(item => item.open).map(item => item.dataset.family));
  const initial = !$('families').children.length;
  const snapshot = state.latest ?? state.attempt?.partial;
  const groups = groupResults(cases, snapshot, roleKey, metricKey);
  $('families').replaceChildren();
  $('metric-description').textContent = metrics[metricKey].description + ' Assessments compare medians. Observed min–max ranges describe retained samples, not confidence intervals.';
  if (metricKey === 'peakRssBytes' && invalidRss(snapshot)) {
    $('metric-description').textContent += ' Earlier Linux RSS observations include launcher memory and are unavailable. Other metrics and the original raw observations are retained.';
  }
  const comparable = groups.reduce((sum, group) => sum + group.inputs.length - group.counts.missing, 0);
  $('coverage-summary').textContent = `${cases.length} registered cases across ${groups.length} areas · ${comparable} comparable · ${cases.length - comparable} unavailable. Open an area to see every input.`;
  for (const group of groups) {
    const section = node('details', undefined, 'workload-family');
    section.dataset.family = group.id;
    section.open = initial ? group.id === 'json-transform' : open.has(group.id);
    const summary = node('summary');
    summary.append(node('span', group.title, 'family-title'));
    const counts = node('div', undefined, 'family-counts');
    for (const [tone, label] of [['better', `${metrics[metricKey].better} median`], ['neutral', 'About the same'], ['worse', `${metrics[metricKey].worse} median`], ['missing', 'Unavailable']]) {
      counts.append(node('span', `${group.counts[tone]} ${label.toLowerCase()}`, `verdict ${tone}`));
    }
    summary.append(counts);
    section.append(summary, node('p', group.description, 'family-description'));
    const scroll = node('div', undefined, 'table-scroll'), table = node('table', undefined, 'daily-table');
    const head = node('thead'), header = node('tr'), body = node('tbody');
    for (const label of ['Input', 'Native', roles[roleKey], 'Median comparison']) {
      const heading = node('th', label); heading.scope = 'col'; header.append(heading);
    }
    head.append(header); table.append(head, body); scroll.append(table); section.append(scroll);
    for (const input of group.inputs) {
      const row = resultFor(snapshot, input), current = metricValue(snapshot, row, 'native', metricKey), control = metricValue(snapshot, row, roleKey, metricKey);
      const tr = node('tr'), name = node('th', `${input.size} · ${input.variant}`, 'input-name');
      name.scope = 'row';
      name.append(node('small', `${input.count.toLocaleString()} items × ${input.batches} batches`));
      name.append(node('small', input.id));
      tr.append(name, cell(format(current), 'Native'), cell(format(control), roles[roleKey]));
      for (const [index, role] of ['native', roleKey].entries()) {
        const range = sampleRange(snapshot, row, role, metricKey);
        tr.children[index + 1].append(node('small', range ? `Observed ${range.minimum === 0 ? '0 (below resolution)' : format(range.minimum)} – ${format(range.maximum)} · n=${range.count}` : 'Observed range unavailable'));
      }
      const assessment = verdict(current, control, metricKey), status = cell(undefined, 'Assessment');
      status.append(node('span', row ? assessment.label : 'Not measured', `verdict ${assessment.tone}`));
      if (assessment.detail) status.append(node('small', assessment.detail));
      if (row) status.append(node('small', rangeComparison(snapshot, row, roleKey, metricKey)));
      if (row) status.append(node('small', `${new Date(snapshot.at).toLocaleDateString()} · ${snapshot.revision.slice(0, 8)}`));
      const failure = state.attempt?.failedObservation;
      if (failure?.case === input.id) status.append(node('small', `Latest attempt failed: ${failure.role} · ${failure.status}`));
      tr.append(status); body.append(tr);
    }
    $('families').append(section);
  }
}

function context() {
  const snapshot = state.latest ?? state.attempt?.partial;
  $('status').classList.toggle('warning', Boolean(state.pendingChanges || state.publicationWarning || !state.latest));
  $('status').replaceChildren(node('strong', snapshot ? `${state.latest ? 'Measured' : 'Partial attempt'} ${date(snapshot.at)}` : 'Awaiting the first published measurement'));
  if (snapshot) {
    $('status').append(node('span', `${snapshot.revision.slice(0, 8)} · ${state.pendingChanges ? 'New performance changes await measurement.' : 'Up to date with performance inputs.'}`));
    $('context').textContent = `${snapshot.platform} · ${snapshot.machine} · profile ${snapshot.inputProfile ?? 'registered'} · ${snapshot.samples} retained fresh processes after ${snapshot.warmups} first process per role · ${snapshot.environment.cpuPinning}. First-process observations remain separate in the evidence.`;
    $('identity').textContent = `Native ${snapshot.revision} · TypeRB Go ${snapshot.referenceRevision} · ${snapshot.tools.go.version} · series ${snapshot.series.slice(0, 12)}. Tool and source hashes are available in the compact results.`;
    $('evidence').replaceChildren(link('Measurement run and raw evidence ↗', snapshot.runUrl));
  }
  if (state.publicationWarning) $('status').append(node('span', state.publicationWarning));
  const attempt = state.attempt;
  if (attempt && attempt.status !== 'measured') {
    $('attempt').hidden = false;
    $('attempt').append(node('span', `Latest attempt: ${attempt.status} · ${date(attempt.at)} · ${attempt.revision.slice(0, 8)}. ${state.latest ? 'The table retains the previous complete measurement.' : 'Unfinished cases remain unmeasured.'}`), link('Inspect attempt ↗', attempt.runUrl));
  }
  for (const item of [...state.history].reverse()) {
    const tr = node('tr');
    tr.append(cell(date(item.at), 'Measured'), cell(item.revision.slice(0, 8), 'Native revision'), cell(`${item.platform} · ${item.machine} · ${item.inputProfile ?? 'registered'}`, 'Environment'));
    const evidence = cell(undefined, 'Evidence'); evidence.append(link('Run ↗', item.runUrl)); tr.append(evidence); $('history').append(tr);
  }
}

choices('metric', metrics, metricKey, value => { metricKey = value; });
choices('comparison', roles, roleKey, value => { roleKey = value; });
const fetchJson = async path => {
  const response = await fetch(path, { cache: 'no-store' });
  if (!response.ok) throw new Error(`Unavailable ${response.status}`);
  return response.json();
};
try {
  const catalog = await fetchJson('./representative-suite.json');
  try {
    const candidate = await fetchJson('./representative-state.json');
    if (candidate.profile !== 'representative' || candidate.schemaVersion !== 1 || !Array.isArray(candidate.history)) throw new Error('Unsupported state');
    state = candidate;
  } catch {
    state = { history: [], publicationWarning: 'Measurement data is unavailable. The complete registered coverage is shown below.' };
  }
  cases = displayCases(catalog, state.latest ?? state.attempt?.partial);
  context(); render();
} catch {
  $('status').textContent = 'The workload catalog could not be loaded. Reload the page to try again.';
  $('status').classList.add('warning');
}
