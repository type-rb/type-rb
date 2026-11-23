import { catalog } from '../docs/capabilities/catalog.js';
import { ordinaryLanguage } from '../docs/capabilities/ordinary-language.js';
import { access, readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';

const statuses = new Set(['verified', 'partial', 'open', 'unassessed']);
const scopes = new Set(['parity', 'production', 'ecosystem']);
const areaIds = new Set();
const titles = new Set();
const failures = [];
const evidencePaths = [];

if (catalog.schemaVersion !== 1) failures.push('catalog.schemaVersion must be 1');
if (!/^\d{4}-\d{2}-\d{2}$/.test(catalog.updatedAt)) failures.push('catalog.updatedAt must use YYYY-MM-DD');

const registryBytes = await readFile(new URL('./native-language-cases.json', import.meta.url));
const registry = JSON.parse(registryBytes);
const referenceRevision = (await readFile(new URL('../TYPE_RB_REVISION', import.meta.url), 'utf8')).trim();
if (ordinaryLanguage.schemaVersion !== 1
    || ordinaryLanguage.registrySha256 !== createHash('sha256').update(registryBytes).digest('hex')
    || ordinaryLanguage.referenceRevision !== referenceRevision) {
  failures.push('ordinary language data is stale; regenerate from the reviewed case registry');
}
if (JSON.stringify(ordinaryLanguage.features) !== JSON.stringify(registry.features)
    || JSON.stringify(ordinaryLanguage.cases.map((item) => item.id)) !== JSON.stringify(registry.cases.map((item) => item.id))) {
  failures.push('ordinary language view omits or changes registered cases or families');
}
if (!ordinaryLanguage.cases.some((item) => item.id === 'utf8-string')) {
  failures.push('ordinary language highlight requires the UTF-8 String case');
}

for (const area of catalog.areas) {
  if (areaIds.has(area.id)) failures.push(`duplicate area id: ${area.id}`);
  areaIds.add(area.id);
  if (!area.title || !area.description || area.items.length === 0) failures.push(`incomplete area: ${area.id}`);

  for (const item of area.items) {
    if (titles.has(item.title)) failures.push(`duplicate capability title: ${item.title}`);
    titles.add(item.title);
    if (!statuses.has(item.status)) failures.push(`invalid status for ${item.title}: ${item.status}`);
    if (!item.description) failures.push(`missing description: ${item.title}`);
    if (item.scopes.length === 0 || item.scopes.some((scope) => !scopes.has(scope))) {
      failures.push(`invalid scopes: ${item.title}`);
    }
    if (item.evidence && item.status !== 'verified' && item.status !== 'partial') {
      failures.push(`evidence requires verified or partial status: ${item.title}`);
    }
    if (item.status === 'verified' && !item.evidence) {
      failures.push(`verified capability requires public evidence: ${item.title}`);
    }
    if (item.evidence && !item.evidence.url.startsWith('https://github.com/type-rb/type-rb-native/')) {
      failures.push(`non-public evidence URL: ${item.title}`);
    }
    if (item.evidence) {
      const relativePath = new URL(item.evidence.url).pathname.split('/blob/main/')[1];
      if (!relativePath) failures.push(`invalid evidence URL: ${item.title}`);
      else evidencePaths.push([item.title, relativePath]);
    }
  }
}

for (const [title, relativePath] of evidencePaths) {
  try {
    await access(new URL(`../${relativePath}`, import.meta.url));
  } catch {
    failures.push(`missing evidence path for ${title}: ${relativePath}`);
  }
}

if (failures.length > 0) {
  failures.forEach((failure) => console.error(`ERROR ${failure}`));
  process.exit(1);
}

console.log(`PASS ${catalog.areas.length} areas, ${titles.size} capabilities and ${ordinaryLanguage.cases.length} ordinary language cases`);
