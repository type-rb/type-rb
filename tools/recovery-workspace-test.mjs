import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { cleanupWorkspace, ownerName, parseReceipt, readWorkspace } from './recovery-workspace.mjs';

const receipt = root => `trbn-recovery-workspace-v1\n${root}\n`;

test('workflow consumers validate the receipt and cleanup precedes evidence upload', () => {
  const workflow = fs.readFileSync(new URL('../.github/workflows/native-validation.yml', import.meta.url), 'utf8');
  assert.equal(workflow.includes('/tmp/type-rb-native-compiler-bootstrap'), false);
  assert.equal(workflow.match(/recovery_workspace=\$\(node tools\/recovery-workspace\.mjs read /g)?.length, 4);
  assert.equal(workflow.match(/"\$recovery_workspace\//g)?.length, 6);
  assert.match(workflow, /TYPE_RB_NATIVE_RECOVERY_RECEIPT: \$\{\{ runner.temp \}\}\/native-suite-evidence\/recovery-workspace.txt/);
  const cleanup = workflow.indexOf('- name: Clean only the invocation-owned recovery workspace');
  const upload = workflow.indexOf('- name: Upload joined suite logs, ownership receipt and cleanup outcome');
  assert.ok(cleanup > workflow.indexOf('- name: Verify the Go-free Native bootstrap harness'));
  assert.ok(upload > cleanup);
  assert.match(workflow.slice(cleanup, upload), /if: always\(\) && steps.native_suites.outcome != 'skipped'/);
  assert.match(workflow.slice(upload), /if: always\(\) && steps.native_suites.outcome != 'skipped'/);
  assert.match(workflow.slice(upload), /if-no-files-found: error/);
});
function fixture(t) {
  const evidence = fs.mkdtempSync(path.join(os.tmpdir(), 'trbn-receipt-control-'));
  const root = execFileSync('/usr/bin/mktemp', ['-d', '/tmp/trbn-recovery.XXXXXX'], { encoding: 'utf8' }).trimEnd();
  // Tests own exactly these freshly allocated paths, including deliberately
  // damaged negative controls that the production cleanup must refuse.
  t.after(() => { fs.rmSync(root, { recursive: true, force: true }); fs.rmSync(evidence, { recursive: true }); });
  const receiptPath = path.join(evidence, 'receipt');
  const reportPath = path.join(evidence, 'cleanup.json');
  fs.writeFileSync(receiptPath, receipt(root));
  fs.writeFileSync(path.join(root, ownerName), receipt(root));
  fs.writeFileSync(path.join(root, 'sentinel'), 'preserve');
  return { root, evidence, receiptPath, reportPath };
}

test('strict receipt grammar rejects broad, nested and extra paths', () => {
  assert.equal(parseReceipt(receipt('/tmp/trbn-recovery.aZ0199')), '/tmp/trbn-recovery.aZ0199');
  for (const value of ['', '/', '/tmp', '/tmp/trbn-recovery.12345', '/tmp/trbn-recovery.1234567',
    '/tmp/trbn-recovery.123/56', '/tmp/trbn-recovery.123456/child', '/tmp/trbn-recovery.12345.',
    '../trbn-recovery.123456']) assert.throws(() => parseReceipt(receipt(value)));
  for (const value of [receipt('/tmp/trbn-recovery.123456') + '\n',
    receipt('/tmp/trbn-recovery.123456').trimEnd(), 'v2\n/tmp/trbn-recovery.123456\n',
    receipt('/tmp/trbn-recovery.123456').replaceAll('\n', '\r\n')]) assert.throws(() => parseReceipt(value));
});

test('read and exact cleanup preserve receipt and publish the outcome', t => {
  const f = fixture(t);
  assert.equal(readWorkspace(f.receiptPath).workspace, f.root);
  assert.equal(cleanupWorkspace(f.receiptPath, f.reportPath).state, 'removed');
  assert.equal(fs.existsSync(f.root), false);
  assert.equal(fs.readFileSync(f.receiptPath, 'utf8'), receipt(f.root));
  assert.equal(JSON.parse(fs.readFileSync(f.reportPath)).state, 'removed');
});

test('CLI publishes only validated paths and refuses malformed input', t => {
  const f = fixture(t);
  const tool = fileURLToPath(new URL('./recovery-workspace.mjs', import.meta.url));
  const run = args => execFileSync(process.execPath, [tool, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  assert.equal(run(['read', f.receiptPath]), f.root + '\n');
  assert.throws(() => run(['read', f.receiptPath, 'unexpected']));
  fs.writeFileSync(f.receiptPath, receipt('/tmp'));
  assert.throws(() => run(['read', f.receiptPath]), error => error.status === 1 && error.stdout === '');
  assert.throws(() => run(['cleanup', f.receiptPath, f.reportPath]), error => error.status === 1);
  assert.equal(JSON.parse(fs.readFileSync(f.reportPath)).state, 'refused');
  assert.equal(fs.readFileSync(path.join(f.root, 'sentinel'), 'utf8'), 'preserve');
});

test('missing allocation receipt reports not-created without deleting anything', t => {
  const f = fixture(t);
  fs.unlinkSync(f.receiptPath);
  assert.equal(cleanupWorkspace(f.receiptPath, f.reportPath).state, 'not-created');
  assert.equal(fs.readFileSync(path.join(f.root, 'sentinel'), 'utf8'), 'preserve');
});

for (const kind of ['malformed', 'foreign-owner', 'missing-owner', 'receipt-symlink', 'owner-symlink', 'directory-receipt', 'fifo-receipt']) {
  test(`${kind} is refused and preserves the workspace`, t => {
    const f = fixture(t);
    const marker = path.join(f.root, ownerName);
    if (kind === 'malformed') fs.writeFileSync(f.receiptPath, receipt('/tmp'));
    if (kind === 'foreign-owner') fs.writeFileSync(marker, receipt('/tmp/trbn-recovery.ZZZZZZ'));
    if (kind === 'missing-owner') fs.unlinkSync(marker);
    if (kind === 'receipt-symlink') {
      fs.renameSync(f.receiptPath, f.receiptPath + '.original');
      fs.symlinkSync(f.receiptPath + '.original', f.receiptPath);
    }
    if (kind === 'owner-symlink') { fs.unlinkSync(marker); fs.symlinkSync(f.receiptPath, marker); }
    if (kind === 'directory-receipt' || kind === 'fifo-receipt') {
      fs.unlinkSync(f.receiptPath);
      if (kind === 'directory-receipt') fs.mkdirSync(f.receiptPath);
      else execFileSync('mkfifo', [f.receiptPath]);
    }
    assert.throws(() => readWorkspace(f.receiptPath));
    assert.throws(() => cleanupWorkspace(f.receiptPath, f.reportPath));
    assert.equal(fs.readFileSync(path.join(f.root, 'sentinel'), 'utf8'), 'preserve');
    assert.equal(JSON.parse(fs.readFileSync(f.reportPath)).state, 'refused');
  });
}

test('symlink workspace never deletes its target', t => {
  const f = fixture(t);
  const target = path.join(f.evidence, 'actual');
  fs.renameSync(f.root, target);
  fs.symlinkSync(target, f.root);
  assert.throws(() => cleanupWorkspace(f.receiptPath, f.reportPath));
  assert.equal(fs.readFileSync(path.join(target, 'sentinel'), 'utf8'), 'preserve');
});

test('cleanup cannot discard its evidence or overwrite an earlier report', t => {
  const f = fixture(t);
  fs.writeFileSync(f.reportPath, 'earlier');
  assert.throws(() => cleanupWorkspace(f.receiptPath, f.reportPath));
  assert.equal(fs.readFileSync(f.reportPath, 'utf8'), 'earlier');
  assert.throws(() => cleanupWorkspace(f.receiptPath, path.join(f.root, 'report')));
  assert.throws(() => cleanupWorkspace(f.receiptPath, f.receiptPath));
  assert.equal(fs.readFileSync(f.receiptPath, 'utf8'), receipt(f.root));
  assert.equal(fs.readFileSync(path.join(f.root, 'sentinel'), 'utf8'), 'preserve');
});

test('simultaneous invocations use distinct roots and cleanup cannot erase a peer', { timeout: 15000 }, async t => {
  const evidence = fs.mkdtempSync(path.join(os.tmpdir(), 'trbn-concurrent-control-'));
  const children = [];
  const roots = [];
  t.after(() => {
    for (const child of children) if (child.exitCode === null) child.kill('SIGKILL');
    for (const root of roots) fs.rmSync(root, { recursive: true, force: true });
    fs.rmSync(evidence, { recursive: true });
  });
  const source = `
    import fs from 'node:fs';
    import { execFileSync } from 'node:child_process';
    const root = execFileSync('/usr/bin/mktemp', ['-d', '/tmp/trbn-recovery.XXXXXX'], {encoding:'utf8'}).trimEnd();
    fs.writeFileSync(root + '/.trbn-recovery-owner', 'trbn-recovery-workspace-v1\\n' + root + '\\n');
    process.on('message', () => { fs.writeFileSync(root + '/sentinel', 'peer'); process.disconnect(); });
    process.send(root);
  `;
  const ready = [0, 1].map(() => {
    const child = spawn(process.execPath, ['--input-type=module', '-e', source], { stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    children.push(child);
    return once(child, 'message').then(([root]) => { parseReceipt(receipt(root)); roots.push(root); return root; });
  });
  const allocated = await Promise.all(ready);
  assert.notEqual(allocated[0], allocated[1]);
  const done = children.map(child => once(child, 'exit'));
  // Neither child can finish its test work until both allocations are ready.
  for (const child of children) child.send('both-ready');
  for (const [code, signal] of await Promise.all(done)) { assert.equal(code, 0); assert.equal(signal, null); }
  for (let index = 0; index < 2; index++) fs.writeFileSync(path.join(evidence, `receipt-${index}`), receipt(allocated[index]));
  cleanupWorkspace(path.join(evidence, 'receipt-0'), path.join(evidence, 'report-0'));
  assert.equal(fs.existsSync(allocated[0]), false);
  assert.equal(fs.readFileSync(path.join(allocated[1], 'sentinel'), 'utf8'), 'peer');
  cleanupWorkspace(path.join(evidence, 'receipt-1'), path.join(evidence, 'report-1'));
});
