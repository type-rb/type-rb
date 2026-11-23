import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { test } from 'node:test';
import { runSuites } from './ci-run-suites.mjs';

const moduleUrl = new URL('./ci-run-suites.mjs', import.meta.url);
function workspace(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'native-ci-suites-'));
  t.after(() => fs.rmSync(dir, { recursive: true }));
  return dir;
}
const readStatus = dir => JSON.parse(fs.readFileSync(path.join(dir, 'status.json')));

test('runs both suites concurrently and retains independent complete logs', async t => {
  const dir = workspace(t);
  const code = `const fs=require('fs'); const [own,other]=process.argv.slice(1);
    fs.writeFileSync(own,'ready'); let attempts=0;
    const timer=setInterval(()=>{if(fs.existsSync(other)){clearInterval(timer);console.log('passed');console.error('diagnostic');}
    else if(++attempts===200){clearInterval(timer);process.exitCode=3;}},10);`;
  const result = await runSuites({ executable: process.execPath, evidence: dir, suites: [
    { name: 'root', args: ['-e', code, dir + '/root.ready', dir + '/compiler.ready'] },
    { name: 'compiler', args: ['-e', code, dir + '/compiler.ready', dir + '/root.ready'] },
  ] });
  assert.equal(result, 0);
  const status = readStatus(dir);
  assert.equal(status.cancellation, null);
  for (const suite of status.suites) {
    assert.equal(suite.code, 0);
    assert.ok(suite.elapsedSeconds > 0);
    assert.equal(fs.readFileSync(`${dir}/${suite.name}.stdout`, 'utf8'), 'passed\n');
    assert.equal(fs.readFileSync(`${dir}/${suite.name}.stderr`, 'utf8'), 'diagnostic\n');
  }
});

for (const codes of [[7, 0], [7, 9]]) {
  test(`retains both results when exit codes are ${codes}`, async t => {
    const dir = workspace(t);
    const result = await runSuites({ executable: process.execPath, evidence: dir, suites:
      ['root', 'compiler'].map((name, i) => ({ name, args: ['-e', `console.log('${name}');process.exitCode=${codes[i]}`] })) });
    assert.equal(result, 1);
    assert.deepEqual(readStatus(dir).suites.map(s=>s.code), codes);
    assert.equal(fs.readFileSync(dir + '/compiler.stdout', 'utf8'), 'compiler\n');
  });
}

test('records launch errors instead of treating missing children as success', async t => {
  const dir = workspace(t);
  assert.equal(await runSuites({ executable: dir + '/absent', evidence: dir, suites: [
    { name: 'root', args: [] }, { name: 'compiler', args: [] },
  ] }), 1);
  assert.ok(readStatus(dir).suites.every(s=>s.state==='completed' && s.launchError.includes('ENOENT')));
});

test('CLI rejects missing recovery variables before launching any suite', t => {
  const dir = workspace(t);
  const run = spawnSync(process.execPath, [moduleUrl.pathname, process.execPath, dir], { env: {}, encoding: 'utf8' });
  assert.equal(run.status, 1);
  assert.match(run.stderr, /recovery\/QBE environment variables/);
  assert.equal(fs.existsSync(dir + '/status.json'), false);
});

test('a log setup failure occurs before either suite starts', async t => {
  const dir = workspace(t);
  fs.writeFileSync(dir + '/compiler.stdout', 'preserved');
  const args = ['-e', `require('fs').writeFileSync(${JSON.stringify(dir + '/started')},'bad')`];
  await assert.rejects(runSuites({ executable: process.execPath, evidence: dir, suites: [
    { name: 'root', args }, { name: 'compiler', args },
  ] }), { code: 'EEXIST' });
  assert.equal(fs.existsSync(dir + '/started'), false);
  assert.equal(fs.readFileSync(dir + '/compiler.stdout', 'utf8'), 'preserved');
});

test('cancellation terminates owned descendants even after their parent exits', { timeout: 10000 }, async t => {
  const dir = workspace(t);
  const descendant = `const fs=require('fs');process.on('SIGTERM',()=>{});fs.writeFileSync(process.argv[1],String(process.pid));setInterval(()=>{},1000);`;
  const parent = `const fs=require('fs');const {spawn}=require('child_process');
    fs.writeFileSync(process.argv[1],String(process.pid));
    spawn(process.execPath,['-e',${JSON.stringify(descendant)},process.argv[1]+'.desc'],{stdio:'ignore'});setInterval(()=>{},1000);`;
  const suites = ['root', 'compiler'].map(name=>({name,args:['-e',parent,dir+'/'+name+'.pid']}));
  const script = `import {runSuites} from ${JSON.stringify(moduleUrl.href)};process.exitCode=await runSuites({executable:process.execPath,evidence:${JSON.stringify(dir)},suites:${JSON.stringify(suites)},graceMs:200});`;
  const controller = spawn(process.execPath, ['--input-type=module', '-e', script], { stdio: 'ignore' });
  const done = once(controller, 'close');
  t.after(() => { if (controller.exitCode === null) controller.kill('SIGTERM'); });
  const pidFiles = ['root.pid', 'root.pid.desc', 'compiler.pid', 'compiler.pid.desc'];
  for (let attempt = 0; !pidFiles.every(p=>fs.existsSync(path.join(dir,p))); attempt++) {
    assert.ok(attempt < 200, 'children should start');
    await delay(10);
  }
  const pids = pidFiles.map(p=>Number(fs.readFileSync(path.join(dir,p))));
  controller.kill('SIGTERM');
  assert.deepEqual(await done, [143, null]);
  assert.equal(readStatus(dir).cancellation, 'SIGTERM');
  const alive = pid => {
    try { process.kill(pid, 0); return true; }
    catch (error) { if (error.code === 'ESRCH') return false; throw error; }
  };
  for (let attempt=0; pids.some(alive) && attempt<100; attempt++) await delay(10);
  assert.ok(pids.every(pid=>!alive(pid)), 'no owned child or descendant should remain');
});

test('an unwritable status path fails before launching children', async t => {
  const dir = workspace(t);
  fs.mkdirSync(dir + '/status.json');
  const args = ['-e', `require('fs').writeFileSync(${JSON.stringify(dir + '/started')},'bad')`];
  await assert.rejects(runSuites({ executable: process.execPath, evidence: dir, suites: [
    { name: 'root', args }, { name: 'compiler', args },
  ] }), { code: 'EISDIR' });
  assert.equal(fs.existsSync(dir + '/started'), false);
});

test('a normally exiting parent cannot leave a descendant for later CI steps', async t => {
  const dir = workspace(t);
  const pidFile = dir + '/descendant.pid';
  const descendant = `require('fs').writeFileSync(${JSON.stringify(pidFile)},String(process.pid));process.on('SIGTERM',()=>{});setInterval(()=>{},1000);`;
  const parent = `const fs=require('fs');require('child_process').spawn(process.execPath,['-e',${JSON.stringify(descendant)}],{stdio:'ignore'}).unref();setInterval(()=>{if(fs.existsSync(${JSON.stringify(pidFile)}))process.exit(0);},10);`;
  assert.equal(await runSuites({ executable: process.execPath, evidence: dir, graceMs: 200, suites: [
    { name: 'root', args: ['-e', parent] }, { name: 'compiler', args: ['-e', ''] },
  ] }), 1);
  assert.equal(readStatus(dir).suites[0].orphanedDescendants, true);
  const pid = Number(fs.readFileSync(pidFile));
  await delay(100);
  assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
});

test('successful suite exit cannot replace missing or incomplete recovery stage evidence', async t => {
  const dir = workspace(t);
  const root = new URL('../', import.meta.url).pathname;
  const record = new URL('./recovery-stage.py', import.meta.url).pathname;
  const code = `const {spawnSync}=require('child_process');
    const file=process.env.TYPE_RB_NATIVE_RECOVERY_STAGES;
    if(!file)process.exit(8);
    const result=spawnSync('python3',[${JSON.stringify(record)},'start',file,'source-preparation']);
    process.exitCode=result.status;`;
  const peer = `if(process.env.TYPE_RB_NATIVE_RECOVERY_STAGES)process.exitCode=9;`;
  const result = await runSuites({ executable: process.execPath, evidence: dir, cwd: root,
    recoveryStages: true, env: { ...process.env, TYPE_RB_NATIVE_RECOVERY_STAGES: '/ignored/shared' }, suites: [
      { name: 'root', args: ['-e', code] }, { name: 'compiler', args: ['-e', peer] },
    ] });
  assert.equal(result, 1);
  const status = readStatus(dir);
  assert.equal(status.suites[0].code, 0);
  assert.match(status.suites[0].stageEvidenceError, /Incomplete/);
  assert.equal(status.suites[1].code, 0);
  const report = JSON.parse(fs.readFileSync(dir + '/recovery-stages.summary.json'));
  assert.equal(report.complete, false);
  assert.equal(report.stages[0].state, 'incomplete');
});

test('controller cancellation retains an interrupted recovery phase', { timeout: 10000 }, async t => {
  const dir = workspace(t);
  const root = new URL('../', import.meta.url).pathname;
  const recorder = new URL('./recovery-stage.py', import.meta.url).pathname;
  const code = `const fs=require('fs');const {spawnSync}=require('child_process');
    const r=spawnSync('python3',[${JSON.stringify(recorder)},'start',process.env.TYPE_RB_NATIVE_RECOVERY_STAGES,'source-preparation']);
    if(r.status!==0)process.exit(7);fs.writeFileSync(${JSON.stringify(dir + '/ready')},'ready');setInterval(()=>{},1000);`;
  const suites = [{name:'root',args:['-e',code]}, {name:'compiler',args:['-e','setInterval(()=>{},1000)']}];
  const script = `import {runSuites} from ${JSON.stringify(moduleUrl.href)};
    process.exitCode=await runSuites({executable:process.execPath,evidence:${JSON.stringify(dir)},cwd:${JSON.stringify(root)},recoveryStages:true,suites:${JSON.stringify(suites)},graceMs:100});`;
  const controller = spawn(process.execPath, ['--input-type=module', '-e', script], { stdio: 'ignore' });
  const done = once(controller, 'close');
  t.after(() => { if (controller.exitCode === null) controller.kill('SIGTERM'); });
  for (let attempt=0; !fs.existsSync(dir + '/ready'); attempt++) {
    assert(attempt < 200, 'root should publish its stage before cancellation');
    await delay(10);
  }
  controller.kill('SIGTERM');
  assert.deepEqual(await done, [143, null]);
  const summary = JSON.parse(fs.readFileSync(dir + '/recovery-stages.summary.json'));
  assert.equal(summary.complete, false);
  assert.equal(summary.suiteOutcome, 'cancelled');
  assert.equal(summary.stages[0].state, 'cancelled');
});
