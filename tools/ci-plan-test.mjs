import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, renameSync, rmSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { acceptance, changedFiles, changedPaths, classify, mainAcceptance, recoveryModules, toolingTests, dailyMeasurementInputs, quickToolingTests, compilerTestInputs, cliInputs } from './ci-plan.mjs';

test('main recovery selects only changed compiler modules and fails closed on unknown inputs', () => {
  const modules = new Set(['compiler', 'lexer', 'parser', 'mir']);
  assert.equal(recoveryModules(['compiler/src/lexer.trb', 'compiler/src/mir.trb',
    'compiler/src/tests/compiler/source_loading_test.trb', 'docs/architecture.md'], modules), 'lexer,mir');
  assert.equal(recoveryModules(['compiler/src/compiler.trb', 'compiler/src/parser_test.trb'], modules), 'none');
  for (const paths of [
    ['compiler/src/new_module.trb'], ['recovery/src/tests/compiler/bootstrap_test.trb'],
    ['recovery/src/compiler/mutations.trb'], ['tools/recovery_layout_sync.py'],
    ['.github/workflows/native-validation.yml'], ['TYPE_RB_REVISION'],
    ['compiler/src/lexer.trb', 'fixtures/recovery/changed.trb'],
  ]) assert.equal(recoveryModules(paths, modules), 'all', String(paths));
});

test('nested module identities remain distinct and moves retain complete validation', () => {
  const modules = new Set(['compiler', 'checking/value', 'mir/value']);
  assert.equal(recoveryModules(['compiler/src/checking/value.trb',
    'compiler/src/mir/value.trb', 'compiler/src/checking/value_test.trb'], modules), 'checking/value,mir/value');
  for (const invalid of ['compiler/src/../escape.trb', 'compiler/src/checking//value.trb',
    'compiler/src/checking/missing.trb']) assert.equal(recoveryModules([invalid], modules), 'all');
  const moved = ['compiler/src/value.trb', 'compiler/src/checking/value.trb',
    'recovery/src/compiler/layout.trb', 'tools/compiler-project.sh'];
  const plan = classify(moved, false, 'tiered');
  assert.equal(plan.complete, true);
  assert.equal(plan.compiler_units, true);
  assert.equal(plan.memory, true);
});

// Planner subprocesses must not inherit the workflow's own gate setting.
function plannerEnv(extra = {}) {
  const { NATIVE_CI_GATE, ...env } = process.env;
  return { ...env, ...extra };
}

function results(plan) {
  return {
    plan: { result: 'success', outputs: Object.fromEntries(
      Object.entries(plan).map(([key, value]) => [key, String(value)])) },
    ...Object.fromEntries(Object.entries({ quick: plan.quick, compiler_units: plan.compiler_units,
      documentation: plan.documentation, native: !plan.draft && plan.code && plan.complete, targets: !plan.draft && plan.code,
      memory: !plan.draft && plan.memory,
      tooling: plan.tooling, cli: !plan.draft && plan.cli,
    }).map(([key, value]) => [key, { result: value ? 'success' : 'skipped' }])),
  };
}

test('compiler and routing changes require every correctness authority', () => {
  for (const paths of [
    ['compiler/src/compiler.trb'],
    ['compiler/src/new-mir-pass.trb', 'docs/architecture.md'],
    ['compiler/conformance/valid/new.trb', 'tools/native-cli-test.py'],
  ]) {
    const plan = classify(paths, false);
    assert.equal(plan.memory, true);
    const needs = results(plan);
    assert.deepEqual(acceptance(needs), []);
    for (const job of ['quick', 'compiler_units', 'native', 'targets', 'memory', 'cli', 'tooling']) {
      for (const result of ['failure', 'cancelled', 'skipped']) {
        assert(acceptance({ ...needs, [job]: { result } }).length > 0);
      }
    }
  }
  for (const path of ['.github/workflows/native-validation.yml', 'tools/ci-run-suites.mjs',
    'tools/compiler-cost.sh', 'tools/compiler-project.sh']) {
    const plan = classify([path], false);
    assert.equal(plan.code, true, path);
    assert.equal(plan.memory, true, path);
  }
});

test('integration explicitly passes migration mode while standalone workflows remain strict', () => {
  for (const name of ['pull-request', 'push-validation']) {
    const workflow = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), 'utf8');
    assert.match(workflow, /^env:\n[\s\S]*?  NATIVE_MIR_COST_MODE: mir-migration$/m);
    const calls = [...workflow.matchAll(/    uses: \.\/\.github\/workflows\/(native-validation|linux-amd64-targets|runtime-worker-memory)\.yml\n([^]*?)(?=\n  \w|$)/g)];
    assert.equal(calls.length, 3);
    for (const call of calls) assert.match(call[2], /    with:\n      cost_mode: mir-migration/);
  }
  for (const name of ['native-validation', 'linux-amd64-targets', 'runtime-worker-memory']) {
    const workflow = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), 'utf8');
    assert.match(workflow, /workflow_call:\n    inputs:\n      cost_mode:[\s\S]*?default: strict/);
    assert(workflow.includes("NATIVE_MIR_COST_MODE: ${{ inputs.cost_mode || 'strict' }}"));
  }
  // Diagnostic trends must reach measurement even when a new compiler exceeds
  // the historical seed byte ceiling; the measured workload is unchanged.
  for (const name of ['daily-performance', 'weekly-performance']) {
    const workflow = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), 'utf8');
    const prepare = workflow.match(/      - name: Close the current[^]*?(?=\n      - name:)/)?.[0];
    assert(prepare?.includes('          NATIVE_MIR_COST_MODE: mir-migration'));
    assert(prepare.includes('/bin/sh tools/daily-performance/prepare-compilers.sh'));
  }
});

test('amd64 migration still verifies binary format and records identities after skipped measurements', () => {
  const source = readFileSync(new URL('linux-amd64-targets.sh', import.meta.url), 'utf8');
  const tail = source.slice(source.lastIndexOf('\nverify_binary_format\n'));
  assert(tail.startsWith('\nverify_binary_format\n'));
  const directory = mkdtempSync(join(tmpdir(), 'native-cost-routing-'));
  try {
    const helper = fileURLToPath(new URL('compiler-cost.sh', import.meta.url));
    for (const shell of ['/bin/sh', '/bin/bash']) {
      for (const mode of ['strict', 'mir-migration']) {
        const script = `set -eu\n. "$HELPER"\nverify_binary_format() { echo binary; }\n` +
          `run_formal_evidence() { echo measured; }\nrecord_target_evidence() { echo identities; }\n${tail}`;
        const options = { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], env: {
          PATH: process.env.PATH, HELPER: helper, evidence: directory, NATIVE_MIR_COST_MODE: mode,
        } };
        assert.equal(execFileSync(shell, ['-c', script], options),
          mode === 'strict' ? 'binary\nmeasured\nidentities\n' : 'binary\nidentities\n');
        if (mode === 'mir-migration') assert.equal(readFileSync(join(directory, 'measurement-policy.txt'), 'utf8'),
          'cost_mode=mir-migration\ncomparative_measurements=not-run\n');
        assert.throws(() => execFileSync(shell, ['-c', script.replace('echo binary;', 'return 91;')], options),
          error => error.status === 91 && !error.stdout.includes('identities'));
      }
    }
    const identities = source.slice(source.indexOf('\nrecord_target_evidence() {'), source.indexOf('\ntest -x /usr/bin/time'));
    assert(identities.includes('candidate_fixed_point_qbe_sha256'));
    assert(identities.includes('require_clean_revision'));
    assert(identities.includes('require_no_intermediates'));
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('compatibility checks remain in quick and standalone validation', () => {
  const workflow = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const standalone = readFileSync(new URL('../.github/workflows/native-validation.yml', import.meta.url), 'utf8');
  const quick = workflow.match(/^  quick:\n([\s\S]*?)(?=^  compiler_units:)/m)?.[1];
  assert(quick, 'quick stage must exist');
  const build = quick.indexOf('python3 tools/build-reference.py .type-rb');
  const formatting = quick.indexOf('Check formatting and core types');
  const checkout = quick.indexOf('repository: type-rb/type-rb');
  const preflight = quick.indexOf('Validate every reference checkout before toolchain setup');
  assert(preflight >= 0 && preflight < checkout && checkout < build);
  const bundleCheck = 'python3 tools/official-sources.py --reference-checkout .type-rb --check';
  assert(quick.indexOf(bundleCheck) > checkout && quick.indexOf(bundleCheck) < build,
    'official source bytes must be compared before building the reference');
  assert(quick.includes('python3 tools/official-sources-test.py'));
  assert(standalone.includes(bundleCheck), 'Main must retain authoritative bundle comparison');
  for (const path of ['tools/official-sources.py', 'vendor/type-rb/official/provenance.json',
    'vendor/type-rb/official/packages/trb/web/src/index.trb']) {
    const plan = classify([path], false, 'tiered');
    assert.equal(plan.code, true);
    assert.equal(plan.quick, true);
    assert.equal(plan.complete, true);
    const needs = results(plan);
    needs.quick.result = 'failure';
    assert(acceptance(needs).length > 0, 'bundle comparison failures must reject PR acceptance');
  }
  for (const command of ['python3 -m unittest tools/compatibility_manifest_test.py',
    'python3 tools/compatibility_manifest.py\n']) {
    const check = quick.indexOf(command, preflight);
    assert(check > preflight && check < checkout, 'static pin checks precede reference checkout');
  }
  for (const command of [
    'python3 -m unittest tools/compatibility_manifest_test.py',
    'python3 tools/compatibility_manifest.py --reference-trb "$RUNNER_TEMP/trb"',
  ]) {
    const check = quick.indexOf(command, build);
    assert(build >= 0 && check > build && formatting > check,
      `${command} must run after reference build and before later quick checks`);
    assert(standalone.includes(command), 'standalone validation must retain the same check');
  }
  for (const job of ['cli', 'native', 'targets', 'memory']) {
    assert(workflow.includes(`  ${job}:\n    needs: plan\n`),
      `${job} should start alongside quick after planning`);
  }
  assert(workflow.includes('  compiler_units:\n    name: Complete compiler units\n    needs: plan\n'));
  assert(workflow.includes('needs: [plan, quick, compiler_units, documentation, native, targets, memory, tooling, cli]'),
    'acceptance must still require quick and all complete authorities');
});

test('complete compiler units run independently for code and CLI and fail closed', () => {
  const workflow = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const units = workflow.match(/^  compiler_units:\n([\s\S]*?)(?=^  documentation:)/m)?.[1];
  assert(units?.includes("if: needs.plan.outputs.compiler_units == 'true'"));
  assert(units.includes('runs-on: macos-15'), 'QBE execution tests assemble arm64_apple code');
  assert(units.includes('ref: cc8c734f73f5149464838825cdbb2165058ed26d'));
  assert(units.includes('test "$(cat TYPE_RB_REVISION)" = "$(git -C .type-rb rev-parse HEAD)"'));
  assert(units.includes('python3 tools/qbe_toolchain.py "$RUNNER_TEMP/qbe-1.3"'));
  const backend = JSON.parse(readFileSync(new URL('./qbe/backend.json', import.meta.url), 'utf8'));
  assert.equal(backend.sourceSHA256, 'd587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320');
  assert.equal(backend.variant, 'typerb-late-product-v1');
  assert(units.includes('TYPE_RB_NATIVE_QBE: ${{ runner.temp }}/qbe-1.3/qbe'));
  assert(units.includes('"$RUNNER_TEMP/trb" test --config compiler/trbconfig.jsonc'));
  assert(units.includes('--compile'));
  assert(units.includes('--outfile "$RUNNER_TEMP/compiler-units"'));
  assert(units.includes('python3 tools/complete_compiler_units.py'));
  assert(units.includes('--scratch "$RUNNER_TEMP" --jobs 2'));
  assert(!units.includes('TYPE_RB_NATIVE_RECOVERY_'));
  assert(units.includes('compiler-units.log') && units.includes('compile_seconds=')
    && units.includes('execution_seconds=') && units.includes('elapsed_seconds='));
  assert(units.includes('actions/upload-artifact@v6'));
  for (const path of ['compiler/src/mir.trb', 'compiler/src/mir_test.trb', 'compiler/cli/main.trb', 'trbn']) {
    for (const draft of [false, true]) {
      const plan = classify([path], draft, 'tiered');
      assert.equal(plan.compiler_units, true, path);
      const needs = results(plan);
      assert.deepEqual(acceptance(needs), []);
      for (const result of ['failure', 'cancelled', 'skipped', 'pending', undefined]) {
        const changed = { ...needs, compiler_units: result ? { result } : undefined };
        assert.notDeepEqual(acceptance(changed), [], `${path}: ${result}`);
      }
      needs.plan.outputs.compiler_units = 'false';
      needs.compiler_units.result = 'skipped';
      assert.notDeepEqual(acceptance(needs), [], 'planning cannot omit complete units');
    }
  }
  for (const path of ['README.md', 'tools/ci-plan.mjs', 'tools/recovery_stage_test.py']) {
    const plan = classify([path], false, 'tiered');
    assert.equal(plan.compiler_units, false, path);
    const needs = results(plan);
    assert.deepEqual(acceptance(needs), []);
    needs.compiler_units.result = 'success';
    assert.notDeepEqual(acceptance(needs), [], 'an unplanned unit job is inconsistent');
  }
});

test('documentation-only PRs do not run compiler matrices', () => {
  const plan = classify(['README.md', 'docs/index.html', 'results/a.json',
    'tools/native-mir-array-loop-recovery/README.md', '.github/ISSUE_TEMPLATE/task.yml'], false);
  assert.deepEqual(plan, { code: false, quick: false, compiler_units: false, documentation: true,
    memory: false, draft: false, tooling: false, cli: false, complete: false, registry_only: false });
  assert.deepEqual(acceptance(results(plan)), []);
});
test('ordinary language cases use a separate reference oracle and Go-free CLI checks', () => {
  const entry = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const cli = readFileSync(new URL('../.github/workflows/native-cli.yml', import.meta.url), 'utf8');
  const docs = readFileSync(new URL('../.github/workflows/documentation.yml', import.meta.url), 'utf8');
  assert(entry.includes('tools/native-language-coverage.py --reference "$RUNNER_TEMP/trb"'));
  assert(cli.includes('tools/native-language-coverage.py --native bin/trbn'));
  assert(!cli.includes('tools/native-language-coverage.py --reference'));
  assert(docs.includes('tools/native-language-coverage.py --check-feature-table docs/native-language-feature-inventory.md'));
  assert(!docs.includes('--check-table'), 'the retired full matrix is not generated or checked');
  for (const path of ['tools/native-language-cases.json', 'tools/native-language-coverage.py',
    'tools/native-language-coverage-test.py']) {
    const plan = classify([path], false);
    assert.equal(plan.quick, true);
    assert.equal(plan.cli, true);
    assert.equal(plan.code, false);
      assert.deepEqual(acceptance(results(plan)), []);
    assert.equal(classify([path, 'compiler/src/compiler.trb'], false).memory, true);
  }
});
test('compiler, conformance and execution workflows retain the full authority', () => {
  for (const path of ['compiler/src/storage.trb', 'compiler/trbconfig.jsonc',
    'compiler/conformance/runtime-invalid/new.trb',
    'tools/compiler-project.sh',
    'compiler/gate4/src/storage.trb',
    'compiler/gate4/conformance/runtime-invalid/new.trb',
    '.github/workflows/pull-request.yml', '.github/workflows/native-validation.yml',
    '.github/workflows/runtime-worker-memory.yml', 'tools/ci-run-suites.mjs',
    'tools/compiler-project.sh']) {
    const plan = classify([path], false);
    assert.equal(plan.code, true);
    assert.equal(plan.memory, true);
    assert.deepEqual(acceptance(results(plan)), []);
  }
});
test('static documentation and evidence tools do not run compiler matrices', () => {
  for (const tool of ['tools/capability-map-check.mjs',
    'tools/benchmark-pages-data.mjs', 'tools/benchmark-pages-check.mjs', 'tools/dependency-pages.py',
    'tools/result_archive.py', 'tools/result_archive_test.py',
    'tools/evidence_bundle.py', 'tools/evidence_bundle_test.py',
    '.github/workflows/documentation.yml']) {
    const plan = classify([tool, 'docs/capabilities/benchmarks/data.js'], false);
    assert.deepEqual(plan, { code: false, quick: false, compiler_units: false, documentation: true,
      memory: false, draft: false, tooling: false, cli: false, complete: false, registry_only: false });
    assert.deepEqual(acceptance(results(plan)), []);
    assert.equal(classify([`${tool}.unknown`], false).code, true);
    assert.equal(classify([tool, 'compiler/gate4/src/compiler.trb'], false).memory, true);
    assert.equal(classify([tool, 'tools/ci-plan.mjs'], false).memory, false);
  }
  assert.equal(classify(['tools/benchmarksgame-formal/run.sh'], false).code, true);
});
test('planning-only maintenance uses its unconditional tests, not compiler matrices', () => {
  const workflow = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const planning = workflow.match(/^  plan:\n([\s\S]*?)(?=^  quick:)/m)?.[1];
  assert(planning);
  assert(!/^\s+if:/m.test(planning), 'planning tests cannot be conditional on their own outputs');
  for (const command of ['node --test tools/ci-plan-test.mjs tools/ci-run-suites-test.mjs tools/recovery-workspace-test.mjs',
    'node tools/ci-plan.mjs "$BASE_SHA" "$HEAD_SHA" "$IS_DRAFT"']) {
    assert(planning.includes(command), 'test and execute the actual router on every PR');
  }
  for (const path of ['tools/ci-plan.mjs', 'tools/ci-plan-test.mjs']) {
    const plan = classify([path, 'docs/evidence-retention.md', 'results/historical/raw.tsv'], false);
    assert.deepEqual(plan, { code: false, quick: false, compiler_units: false, documentation: true,
      memory: false, draft: false, tooling: false, cli: false, complete: false, registry_only: false });
    assert.deepEqual(acceptance(results(plan)), []);
    for (const failure of ['failure', 'cancelled', 'skipped', undefined]) {
      const needs = results(plan);
      needs.plan.result = failure;
      assert.notDeepEqual(acceptance(needs), [], 'no acceptance without successful planning');
    }
    for (const other of ['compiler/src/compiler.trb', 'src/runtime.trb', 'TYPE_RB_REVISION',
      'tools/ci-run-suites.mjs', '.github/workflows/pull-request.yml',
      'tools/compiler-project.sh', `${path}.unknown`, 'unknown/file']) {
      const mixed = classify([path, other], false);
      assert.equal(mixed.code, true, other);
    }
  }
});
test('other executable changes retain complete correctness and target checks', () => {
  const plan = classify(['src/decoder.trb'], false);
  assert.equal(plan.code, true);
  assert.deepEqual(acceptance(results(plan)), []);
});

test('deletions, renames, mixed changes and unknown paths fail toward more checking', () => {
  assert.equal(classify(['README.md', 'compiler/gate4/src/old.trb'], false).memory, true);
  assert.equal(classify(['new-directory/file'], false).code, true);
  assert.equal(classify(['compiler/gate4/src/with\na newline.trb'], false).code, true);
});
test('draft feedback succeeds only for its selected authorities', () => {
  const documentation = results(classify(['README.md'], true));
  assert.deepEqual(acceptance(documentation), []);
  const code = results(classify(['compiler/src/compiler.trb'], true));
  assert.deepEqual(acceptance(code), []);
  for (const job of ['quick', 'compiler_units', 'documentation', 'tooling']) {
    if (code[job].result === 'success') {
      code[job].result = 'failure';
      assert.notDeepEqual(acceptance(code), [], `${job} failure must reject draft feedback`);
      code[job].result = 'success';
    }
  }
  code.native.result = 'success';
  assert.notDeepEqual(acceptance(code), [], 'draft cannot silently run a complete authority');
});
test('draft acceptance explicitly records partial feedback', () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-draft-summary-'));
  try {
    const summary = join(directory, 'summary.md');
    const plan = results(classify(['compiler/src/compiler.trb'], true));
    const output = execFileSync(process.execPath, [fileURLToPath(new URL('ci-plan.mjs', import.meta.url)), 'accept'], {
      encoding: 'utf8', env: { ...process.env, NEEDS_JSON: JSON.stringify(plan), GITHUB_STEP_SUMMARY: summary },
    });
    assert.match(output, /Draft feedback only/);
    assert.match(readFileSync(summary, 'utf8'), /complete validation runs when the PR is marked ready/);
    plan.quick.result = 'failure';
    assert.throws(() => execFileSync(process.execPath,
      [fileURLToPath(new URL('ci-plan.mjs', import.meta.url)), 'accept'], {
        encoding: 'utf8', stdio: 'pipe',
        env: { ...process.env, NEEDS_JSON: JSON.stringify(plan), GITHUB_STEP_SUMMARY: summary },
      }));
    assert.equal(readFileSync(summary, 'utf8').match(/Draft feedback only/g)?.length, 1);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
test('draft development defers expensive authorities without relaxing ready acceptance', () => {
  const workflow = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  for (const job of ['cli', 'native', 'targets', 'memory']) {
    const block = workflow.match(new RegExp(`^  ${job}:\\n([\\s\\S]*?)(?=^  [a-z]+:)`, 'm'))?.[1];
    assert(block?.includes('needs: plan'), `${job} must wait for planning`);
    assert(block.includes("needs.plan.outputs.draft == 'false'"), `${job} must wait for ready integration`);
  }
  for (const file of ['compiler/cli/main.trb', 'compiler/src/compiler.trb']) {
    const draft = results(classify([file], true));
    assert.deepEqual(acceptance(draft), []);
    const ready = results(classify([file], false));
    assert.deepEqual(acceptance(ready), []);
    ready.cli.result = 'skipped';
    assert.notDeepEqual(acceptance(ready), [], 'ready CLI validation cannot be skipped');
  }
});
test('failed, cancelled, skipped, missing and pending required jobs reject acceptance', () => {
  for (const job of ['quick', 'compiler_units', 'documentation', 'native', 'targets', 'memory', 'tooling', 'cli']) {
    for (const state of ['failure', 'cancelled', 'skipped', 'pending', undefined]) {
      const needs = results(classify(['compiler/gate4/src/compiler.trb', 'README.md'], false));
      needs[job] = state ? { result: state } : undefined;
      assert.notDeepEqual(acceptance(needs), [], `${job}: ${state}`);
    }
  }
});
test('missing or malformed planning never authorizes skipped validation', () => {
  assert.notDeepEqual(acceptance({}), []);
  for (const key of Object.keys(classify(['README.md'], false))) {
    for (const value of ['', undefined, true]) {
      const needs = results(classify(['README.md'], false));
      needs.plan.outputs[key] = value;
      assert.notDeepEqual(acceptance(needs), [], `${key}: ${value}`);
    }
  }
  const needs = results(classify(['README.md'], false));
  needs.plan.result = 'failure';
  assert.notDeepEqual(acceptance(needs), []);
});

test('CLI classifies real historical-to-documentation and current-project renames', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-ci-plan-test-'));
  const git = (...args) => execFileSync('git', [
    '-c', 'user.name=CI Test', '-c', 'user.email=ci-test@example.invalid',
    '-c', 'commit.gpgsign=false', '-c', 'core.hooksPath=/dev/null', ...args,
  ], { cwd: directory, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  try {
    git('init');
    mkdirSync(join(directory, 'compiler/gate4/src'), { recursive: true });
    const source = join(directory, 'compiler/gate4/src/old\nname.trb');
    writeFileSync(source, 'def main()\n\treturn\nend\n');
    git('add', '.');
    git('commit', '-m', 'Initial synthetic fixture');
    const base = git('rev-parse', 'HEAD');
    mkdirSync(join(directory, 'docs'));
    renameSync(source, join(directory, 'docs/example.md'));
    git('add', '-A');
    git('commit', '-m', 'Move synthetic fixture');
    const head = git('rev-parse', 'HEAD');
    const args = [fileURLToPath(new URL('./ci-plan.mjs', import.meta.url)), base, head, 'false'];
    const output = execFileSync(process.execPath, args, { cwd: directory, encoding: 'utf8', env: plannerEnv() });
    assert.deepEqual(Object.fromEntries(output.trim().split('\n').map(row => row.split('='))),
      { code: 'true', quick: 'true', compiler_units: 'true', documentation: 'true', memory: 'true',
        draft: 'false', tooling: 'true', cli: 'true', complete: 'true', registry_only: 'false' });
    mkdirSync(join(directory, 'compiler/src'), { recursive: true });
    renameSync(join(directory, 'docs/example.md'), join(directory, 'compiler/src/current.trb'));
    git('add', '-A');
    git('commit', '-m', 'Move synthetic fixture to current project');
    const movedPaths = await changedPaths(base, git('rev-parse', 'HEAD'), directory);
    assert.deepEqual(movedPaths.sort(), ['compiler/gate4/src/old\nname.trb', 'compiler/src/current.trb']);
    const movedPlan = classify(movedPaths, false);
    assert.equal(movedPlan.memory, true);
    assert.deepEqual(acceptance(results(movedPlan)), []);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('large evidence inventories retain every path and a final code change', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-ci-plan-large-test-'));
  const git = (args, input) => execFileSync('git', [
    '-c', 'user.name=CI Test', '-c', 'user.email=ci-test@example.invalid',
    '-c', 'commit.gpgsign=false', '-c', 'core.hooksPath=/dev/null', ...args,
  ], { cwd: directory, encoding: 'utf8', input, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
  try {
    git(['init']);
    const emptyTree = git(['mktree'], '');
    const base = git(['commit-tree', emptyTree], 'Empty synthetic baseline\n');
    const blob = git(['hash-object', '-w', '--stdin'], 'Synthetic observation\n');
    const names = Array.from({ length: 7000 }, (_, i) =>
      `${String(i).padStart(5, '0')}-${'観測-'.repeat(25)}.txt`);
    const evidenceTree = git(['mktree'],
      names.map(name => `100644 blob ${blob}\t${name}\n`).join(''));
    const docsTree = git(['mktree'], `040000 tree ${evidenceTree}\tresults\n`);
    const docsHead = git(['commit-tree', docsTree, '-p', base], 'Synthetic evidence\n');
    const paths = await changedPaths(base, docsHead, directory);
    assert.equal(paths.length, names.length);
    assert(Buffer.byteLength(paths.join('\0')) > 1024 * 1024);
    assert.deepEqual(paths, names.map(name => `results/${name}`));
    assert.equal(classify(paths, false).code, false);

    // This path sorts after the entire >1 MiB evidence list.
    const codeTree = git(['mktree'],
      `040000 tree ${evidenceTree}\tresults\n100644 blob ${blob}\tzz-final-code.trb\n`);
    const codeHead = git(['commit-tree', codeTree, '-p', base], 'Final synthetic code change\n');
    const complete = await changedPaths(base, codeHead, directory);
    assert.equal(complete.length, names.length + 1);
    assert.equal(complete.at(-1), 'zz-final-code.trb');
    assert.equal(classify(complete, false).code, true);
    await assert.rejects(changedPaths(base, '0'.repeat(40), directory), /Git path inventory failed/);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('exact CLI inputs run quick and CLI authorities without core measurements', () => {
  for (const file of cliInputs) {
    const plan = classify([file], false);
    assert.equal(plan.code, false, file);
    assert.equal(plan.cli, true, file);
    assert.equal(plan.memory, false, file);
    assert.deepEqual(acceptance(results(plan)), []);
    for (const state of ['skipped', 'failure', 'cancelled', undefined]) {
      const needs = results(plan);
      needs.cli = { result: state };
      assert.notDeepEqual(acceptance(needs), [], file);
    }
    assert.equal(classify([file + '.unknown'], false).code, true);
    assert.equal(classify([file, 'compiler/src/compiler.trb'], false).memory, true);
  }
  const workflow = readFileSync(new URL('../.github/workflows/native-cli.yml', import.meta.url), 'utf8');
  const cliTests = readFileSync(new URL('../tools/native-cli-test.py', import.meta.url), 'utf8');
  for (const name of ['native-enum-test.py', 'native-repl-flow-test.py', 'native-callable-test.py']) {
    assert(cliTests.includes(name), `the CLI authority must execute ${name}`);
  }
  for (const name of ['check.trb', 'defaults.trb', 'callables.trb', 'types.trb']) {
    assert(cliInputs.has(`compiler/cli/repl/${name}`));
  }
  assert(workflow.includes('  workflow_call:'));
  assert(!workflow.includes('  pull_request:'), 'one shared PR planner, no separate path-filtered run');
  const cliJobs = workflow.split('\n  cache:\n');
  assert.equal(cliJobs.length, 2, 'CLI smoke and cache invalidation must be separate required jobs');
  assert(cliJobs[0].includes('  build:\n') && cliJobs[0].includes('tools/native-cli-test.py bin/trbn'));
  assert(!cliJobs[0].includes('tools/native-bootstrap-test.py'));
  assert(cliJobs[1].includes('tools/native-bootstrap-test.py'));
  assert(cliJobs[1].includes('native-cli-cache-${{ matrix.platform }}'));
  assert(cliJobs[1].includes('if: always()') && cliJobs[1].includes('if-no-files-found: error'));
  assert.equal((workflow.match(/- runner: macos-15\r?$/gm) || []).length, 3, 'smoke, cache and archive qualification retain Darwin arm64');
  assert.equal((workflow.match(/- runner: ubuntu-24.04-arm/g) || []).length, 3, 'smoke, cache and archive qualification retain Linux arm64');
  assert(workflow.includes('- runner: macos-15-intel'));
  assert(workflow.includes('python3 tools/network_artifact_smoke.py'));
  assert(workflow.includes('python3 tools/native-network-distribution-test.py'));
  assert(workflow.includes('python3 tools/compression_artifact_smoke.py'));
  assert(workflow.includes('python3 tools/native-web-compression-test.py'));
  assert(workflow.includes('python3 tools/crypto_artifact_smoke.py'));
  assert(workflow.includes('python3 tools/native-crypto-test.py'));
  assert.equal((workflow.match(/timeout-minutes: 90/g) || []).length, 2);
  assert.equal(classify(['compiler/conformance/README.md'], false).cli, false);
});

test('synthetic tooling tests have an executable authority without compiler rebuilds', () => {
  const workflow = readFileSync(new URL('../.github/workflows/ci-tooling.yml', import.meta.url), 'utf8');
  for (const file of toolingTests) {
    assert(workflow.includes(file), `the tooling authority must execute ${file}`);
    const plan = classify([file], false);
    assert.equal(plan.code, false, file);
    assert.equal(plan.cli, false, file);
    assert.equal(plan.tooling, true, file);
    assert.deepEqual(acceptance(results(plan)), []);
    const needs = results(plan);
    needs.tooling.result = 'skipped';
    assert.notDeepEqual(acceptance(needs), []);
    assert.equal(classify([file + '.unknown'], false).code, true);
    assert.equal(classify([file, 'compiler/src/compiler.trb'], false).memory, true);
  }
  for (const file of ['tools/benchmarksgame-build-formal/build-controller.sh',
    'tools/recovery-stage.py', 'tools/ci-run-suites.mjs', '.github/workflows/ci-tooling.yml']) {
    assert.equal(classify([file], false).code, true, file);
  }
  const native = readFileSync(new URL('../.github/workflows/native-validation.yml', import.meta.url), 'utf8');
  assert(!native.includes('Verify bootstrap seed tooling'));
  const entry = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  assert(!entry.includes('  performance:'), 'strict comparative measurement is retired');
  assert(entry.includes('needs: [plan, quick, compiler_units, documentation, native, targets, memory, tooling, cli]'));
});

test('daily measurement controllers use their dedicated tooling suite', () => {
  const workflow = readFileSync(new URL('../.github/workflows/ci-tooling.yml', import.meta.url), 'utf8');
  assert(workflow.includes("python3 -m unittest discover -s tools/daily-performance -p 'test_*.py'"));
  for (const file of dailyMeasurementInputs) {
    assert(file.startsWith('tools/daily-performance/'));
    const plan = classify([file], false, 'tiered');
    assert.equal(plan.code, false, file);
    assert.equal(plan.cli, false, file);
    assert.equal(plan.tooling, true, file);
    assert.deepEqual(acceptance(results(plan)), []);
    assert.equal(classify([file + '.unknown'], false, 'tiered').code, true);
  }
  const mixed = classify(['compiler/src/compiler.trb', 'tools/daily-performance/measure.py'], false, 'tiered');
  assert.equal(mixed.code, true);
  assert.equal(mixed.complete, true);
});

test('main uses the same complete path classifier and Pages PR checks are not duplicated', () => {
  const main = readFileSync(new URL('../.github/workflows/push-validation.yml', import.meta.url), 'utf8');
  assert(main.includes('node tools/ci-plan.mjs "$BASE_SHA" "$HEAD_SHA" false push'));
  assert(main.includes('BASE_SHA: ${{ steps.base.outputs.sha }}'));
  assert(!main.includes('github.event.before'), 'main compares against its last fully validated commit');
  for (const job of ['native', 'targets', 'cli', 'memory', 'tooling', 'documentation']) assert(main.includes(`  ${job}:`));
  for (const name of ['native-validation', 'runtime-worker-memory', 'documentation']) {
    const source = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), 'utf8');
    assert(!source.includes('  push:'), `${name} must not independently rerun the same main validation`);
    assert(source.includes('  workflow_call:'));
  }
  const pages = readFileSync(new URL('../.github/workflows/capability-map-pages.yml', import.meta.url), 'utf8');
  assert(!pages.includes('  pull_request:'));
  assert(pages.includes('  push:') && pages.includes('  workflow_dispatch:'));
  assert.equal(classify(['.github/workflows/capability-map-pages.yml'], false).code, false);
  const doc = readFileSync(new URL('../.github/workflows/documentation.yml', import.meta.url), 'utf8');
  for (const command of ['node tools/capability-map-check.mjs', 'node tools/benchmark-pages-data.mjs --check', 'node tools/benchmark-pages-check.mjs']) assert(doc.includes(command));
});

test('mixed lightweight surfaces require the union and never skip a failed authority', () => {
  const plan = classify(['compiler/cli/main.trb', 'tools/recovery_stage_test.py', 'README.md'], false);
  assert.equal(plan.code, false);
  assert.equal(plan.cli, true);
  assert.equal(plan.tooling, true);
  assert.equal(plan.documentation, true);
  assert.deepEqual(acceptance(results(plan)), []);
  for (const job of ['quick', 'compiler_units', 'cli', 'tooling', 'documentation']) {
    for (const result of ['failure', 'cancelled', 'skipped', undefined]) {
      const needs = results(plan);
      needs[job] = { result };
      assert.notDeepEqual(acceptance(needs), []);
    }
  }
  for (const key of ['cli', 'tooling']) {
    const needs = results(plan);
    delete needs.plan.outputs[key];
    assert.notDeepEqual(acceptance(needs), []);
  }
});

test('push comparison includes changes against the actual before revision, not its merge base', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-ci-push-test-'));
  const git = (args, input) => execFileSync('git', [
    '-c', 'user.name=CI Test', '-c', 'user.email=ci-test@example.invalid',
    '-c', 'commit.gpgsign=false', ...args,
  ], { cwd: directory, encoding: 'utf8', input }).trim();
  try {
    git(['init']);
    const empty = git(['mktree'], '');
    const base = git(['commit-tree', empty], 'Base\n');
    const blob = git(['hash-object', '-w', '--stdin'], 'Synthetic\n');
    const codeTree = git(['mktree'], `100644 blob ${blob}\tcore.trb\n`);
    const before = git(['commit-tree', codeTree, '-p', base], 'Earlier code\n');
    const docsTree = git(['mktree'], `100644 blob ${blob}\tREADME.md\n`);
    const head = git(['commit-tree', docsTree, '-p', base], 'Documentation on another lineage\n');
    assert.deepEqual(await changedPaths(before, head, directory), ['README.md']);
    assert.deepEqual(await changedPaths(before, head, directory, true), ['README.md', 'core.trb']);
    const output = execFileSync(process.execPath, [fileURLToPath(new URL('./ci-plan.mjs', import.meta.url)),
      before, head, 'false', 'push'], { cwd: directory, encoding: 'utf8' });
    assert.match(output, /^code=true$/m);
    assert.throws(() => execFileSync(process.execPath, [fileURLToPath(new URL('./ci-plan.mjs', import.meta.url)),
      before, head, 'false', 'unknown'], { cwd: directory, stdio: 'pipe' }));
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('known compiler test modules retain correctness without unchanged-binary measurements', () => {
  assert.equal(compilerTestInputs.size, 12);
  for (const file of compilerTestInputs) {
    const plan = classify([file], false);
    assert.deepEqual(plan, { code: true, quick: true, compiler_units: true, documentation: false,
      memory: false, draft: false, tooling: true, cli: true, complete: true, registry_only: false });
    assert.deepEqual(acceptance(results(plan)), []);
    for (const job of ['quick', 'compiler_units', 'native', 'targets', 'tooling', 'cli']) {
      for (const state of ['failure', 'cancelled', 'skipped', undefined]) {
        const needs = results(plan);
        needs[job] = { result: state };
        assert.notDeepEqual(acceptance(needs), [], `${file}: ${job}: ${state}`);
      }
    }
    for (const other of ['compiler/src/compiler.trb', 'compiler/conformance/new.source',
      'compiler/trbconfig.jsonc', 'compiler/src/new_test.trb', 'src/decoder.trb',
      'TYPE_RB_REVISION', 'tools/compiler-project.sh',
      '.github/workflows/pull-request.yml', 'tools/ci-plan.mjs', 'unknown/file']) {
      assert.equal(classify([file, other], false).memory, true, other);
    }
    assert.equal(classify([file + '.unknown'], false).memory, true);
    assert.equal(classify([file, 'README.md'], false).memory, false);
  }
});

test('synthetic project and policy tests retain Linux without building the reference compiler', () => {
  const entry = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const quick = entry.match(/^  quick:\n([\s\S]*?)(?=^  compiler_units:)/m)?.[1];
  assert(quick.includes("if: needs.plan.outputs.quick == 'true'"));
  assert(entry.includes('quick: ${{ steps.plan.outputs.quick }}'));
  const steps = quick.split('      - ').slice(1);
  const compilerCondition = "needs.plan.outputs.code == 'true' || needs.plan.outputs.cli == 'true'";
  const recoveryImports = steps.find(step => step.startsWith('name: Check generated recovery import boundaries'));
  assert(recoveryImports?.includes('python3 tools/recovery_layout_sync.py --check'));
  assert(recoveryImports.includes("if: needs.plan.outputs.registry_only != 'true'"),
    'Recovery imports keep their Linux authority except for unrelated registry edits');
  const ownership = steps.find(step => step.startsWith('name: Check compiler responsibility boundaries'));
  assert(ownership?.includes('python3 tools/compiler_layout.py'));
  assert(ownership.includes("if: needs.plan.outputs.registry_only != 'true'"),
    'Compiler ownership needs only Python, including for synthetic tool edits');
  for (const step of steps) {
    if (step.includes('repository: type-rb/type-rb') || step.includes('actions/setup-go') ||
      step.startsWith('name: Build the pinned') ||
      (step.startsWith('name: Check ') && step !== recoveryImports && step !== ownership) ||
      step.startsWith('name: Run root')) assert(step.includes(compilerCondition), step);
  }
  const toolingStep = steps.find(step => step.startsWith('name: Verify project'));
  assert(toolingStep?.includes("if: needs.plan.outputs.registry_only != 'true'"),
    'Linux controls remain enabled for synthetic tool edits without compiler setup');
  for (const file of quickToolingTests) {
    assert(toolingStep.includes(`sh ${file}`));
    const plan = classify([file], false);
    assert.deepEqual(plan, { code: false, quick: true, compiler_units: false, documentation: false,
      memory: false, draft: false, tooling: true, cli: false, complete: false, registry_only: false });
    assert.deepEqual(acceptance(results(plan)), []);
    for (const job of ['quick', 'tooling']) {
      for (const state of ['failure', 'cancelled', 'skipped', undefined]) {
        const needs = results(plan);
        needs[job] = { result: state };
        assert.notDeepEqual(acceptance(needs), []);
      }
    }
  }
  const invalid = results(classify(['compiler/src/compiler.trb'], false));
  invalid.plan.outputs.quick = 'false';
  invalid.quick.result = 'skipped';
  assert.notDeepEqual(acceptance(invalid), [], 'code cannot bypass quick through inconsistent outputs');
});

test('controller-only test edits keep both Linux and macOS execution', () => {
  const pr = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const main = readFileSync(new URL('../.github/workflows/push-validation.yml', import.meta.url), 'utf8');
  const tooling = readFileSync(new URL('../.github/workflows/ci-tooling.yml', import.meta.url), 'utf8');
  const native = readFileSync(new URL('../.github/workflows/native-validation.yml', import.meta.url), 'utf8');
  for (const file of ['tools/ci-run-suites-test.mjs', 'tools/recovery-workspace-test.mjs']) {
    assert(pr.split('\n  quick:\n')[0].includes(file));
    assert(main.split('  native:')[0].includes(file));
    assert(tooling.includes(file));
    assert(!native.includes(file), 'the old macOS execution moved without duplication');
    const plan = classify([file], false);
    assert.equal(plan.code, false);
    assert.equal(plan.quick, false);
    assert.equal(plan.tooling, true);
    assert.deepEqual(acceptance(results(plan)), []);
  }
});

test('the tiered PR gate defers complete lanes only for ordinary compiler, CLI and conformance edits', () => {
  for (const paths of [
    ['compiler/src/compiler.trb'],
    ['compiler/src/new_mir_pass.trb', 'recovery/src/compiler/layout.trb', 'recovery/src/compiler/mutations.trb'],
    ['compiler/conformance/valid/new.trb', 'compiler/cli/repl/session.trb', 'tools/native-cli-test.py'],
    ['compiler/cli/main.trb', 'tools/recovery_stage_test.py', 'docs/architecture.md'],
  ]) {
    const tiered = classify(paths, false, 'tiered');
    assert.deepEqual(tiered, { ...classify(paths, false), complete: false }, paths.join());
    const needs = results(tiered);
    assert.equal(needs.native.result, 'skipped');
    assert.deepEqual(acceptance(needs), []);
    for (const job of ['quick', 'targets', 'cli']) {
      if (needs[job].result !== 'success') continue;
      for (const result of ['failure', 'cancelled', 'skipped', undefined]) {
        assert.notDeepEqual(acceptance({ ...needs, [job]: { result } }), [], `${paths}: ${job}: ${result}`);
      }
    }
    assert.notDeepEqual(acceptance({ ...needs, native: { result: 'success' } }), [],
      'a deferred authority cannot silently run');
  }
  for (const other of ['recovery/src/managed/mir.trb', 'recovery/src/tests/compiler/bootstrap_test.trb',
    'fixtures/recovery/programs/x.trb', 'corpus/recovery-scalar/a/main.trb', 'benchmarks/benchmarksgame/a.trb',
    'TYPE_RB_REVISION', 'compatibility/current.json', 'trbn', 'tools/build-native.sh',
    'tools/native-bootstrap-test.py', 'tools/check-bootstrap-snapshot.sh', 'tools/recovery-bootstrap.sh',
    '.github/workflows/native-validation.yml', '.github/workflows/pull-request.yml',
    'tools/ci-run-suites.mjs', 'tools/linux-amd64-targets.sh', 'unknown/file']) {
    const plan = classify(['compiler/src/compiler.trb', other], false, 'tiered');
    assert.equal(plan.complete, true, other);
    assert.deepEqual(plan, classify(['compiler/src/compiler.trb', other], false), other);
    const needs = results(plan);
    assert.equal(needs.native.result, 'success');
    assert.notDeepEqual(acceptance({ ...needs, native: { result: 'skipped' } }), [], other);
  }
  assert.equal(classify(['README.md'], false, 'tiered').complete, false);
  for (const gate of ['', 'TIERED', 'post-merge', null]) {
    assert.throws(() => classify(['compiler/src/compiler.trb'], false, gate), /Invalid CI gate/);
  }
});

test('PR workflows pass the tiered scope while main runs every complete lane', () => {
  const entry = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  const main = readFileSync(new URL('../.github/workflows/push-validation.yml', import.meta.url), 'utf8');
  const cli = readFileSync(new URL('../.github/workflows/native-cli.yml', import.meta.url), 'utf8');
  const targets = readFileSync(new URL('../.github/workflows/linux-amd64-targets.yml', import.meta.url), 'utf8');
  assert.match(entry, /^env:\n[\s\S]*?  NATIVE_CI_GATE: tiered$/m);
  assert(!main.includes('NATIVE_CI_GATE'), 'main always plans the complete lanes');
  assert(entry.includes('complete: ${{ steps.plan.outputs.complete }}'));
  const scope = "scope: ${{ needs.plan.outputs.complete == 'true' && 'complete' || 'pull-request' }}";
  for (const job of ['cli', 'targets']) {
    const block = entry.match(new RegExp(`^  ${job}:\\n([\\s\\S]*?)(?=^  [a-z]+:)`, 'm'))?.[1];
    assert(block?.includes(scope), `${job} must follow the planned scope`);
  }
  const native = entry.match(/^  native:\n([\s\S]*?)(?=^  [a-z]+:)/m)?.[1];
  assert(native?.includes("needs.plan.outputs.complete == 'true'"));
  const quick = entry.match(/^  quick:\n([\s\S]*?)(?=^  documentation:)/m)?.[1];
  assert(quick.includes('tools/check-bootstrap-snapshot.sh "$RUNNER_TEMP/trb"'),
    'the snapshot-v4 subset check must run before merge');
  const [build, cache] = cli.split('\n  cache:\n');
  assert(build.includes('python3 tools/check-conformance-sources.py .trb/bootstrap/core/compiler'));
  assert.match(cache, /^(?:    #.*\n)*    if: inputs.scope != 'pull-request'\n/);
  assert.match(cli, /workflow_call:\n    inputs:\n      scope:[\s\S]*?default: complete/);
  assert.match(targets, /      scope:\n[^]*?default: complete\n  workflow_dispatch:/);
  for (const job of ['regress-linux-arm64', 'compare-target-neutral-evidence']) {
    const block = targets.match(new RegExp(`^  ${job}:\\n([\\s\\S]*?)(?=^    runs-on:|^    needs:)`, 'm'))?.[1];
    assert(block?.includes("if: inputs.scope != 'pull-request'"), job);
  }
  assert(!targets.match(/^  verify-linux-amd64:\n([\s\S]*?)(?=^    runs-on:)/m)[1].includes('if:'),
    'x64 target verification stays in the PR gate');
  for (const call of ['uses: ./.github/workflows/native-validation.yml', 'uses: ./.github/workflows/native-cli.yml',
    'uses: ./.github/workflows/linux-amd64-targets.yml']) assert(main.includes(call));
  assert(!/native-cli\.yml\n    with:\n      scope/.test(main), 'main CLI keeps the complete default');
  assert.match(main, /concurrency:\n  group: native-main-validation-\$\{\{ github.event_name \}\}\n  cancel-in-progress: false/);
  assert(main.includes("- cron: '17 18 * * *'"), 'daily full recovery is retained');
  assert(main.includes('if test "$GITHUB_EVENT_NAME" = schedule; then'));
  assert(main.includes('recovery_modules: ${{ steps.plan.outputs.recovery_modules }}'));
  assert(main.includes('recovery_modules: ${{ needs.plan.outputs.recovery_modules }}'));
  const nativeValidation = readFileSync(new URL('../.github/workflows/native-validation.yml', import.meta.url), 'utf8');
  assert(nativeValidation.includes("TYPE_RB_NATIVE_RECOVERY_MODULES: ${{ inputs.recovery_modules || 'all' }}"));
  assert(main.includes('--workflow push-validation.yml') && main.includes('--status success'));
  assert(main.includes('git merge-base --is-ancestor "$sha" "$GITHUB_SHA"'));
  assert(main.includes('git hash-object -t tree /dev/null'), 'no validated ancestor plans everything');
  assert(main.includes('needs: [plan, native, targets, cli, tooling, memory, documentation]'));
  assert(main.includes('node tools/ci-plan.mjs accept-main'));
  assert(main.includes('gh issue create') && main.includes('gh issue close'));
});

test('main acceptance requires every lane planned for changes since the last validated commit', () => {
  const lanes = { code: true, documentation: true, memory: true, tooling: true, cli: true };
  const needs = {
    plan: { result: 'success', outputs: Object.fromEntries(Object.entries(lanes).map(([k, v]) => [k, String(v)])) },
    ...Object.fromEntries(['native', 'targets', 'cli', 'memory', 'tooling', 'documentation']
      .map(job => [job, { result: 'success' }])),
  };
  assert.deepEqual(mainAcceptance(needs), []);
  for (const job of ['native', 'targets', 'cli', 'memory', 'tooling', 'documentation']) {
    for (const result of ['failure', 'cancelled', 'skipped', undefined]) {
      assert.notDeepEqual(mainAcceptance({ ...needs, [job]: { result } }), [], `${job}: ${result}`);
    }
  }
  const documentation = structuredClone(needs);
  for (const key of ['code', 'memory', 'tooling', 'cli']) documentation.plan.outputs[key] = 'false';
  for (const job of ['native', 'targets', 'cli', 'memory', 'tooling']) documentation[job].result = 'skipped';
  assert.deepEqual(mainAcceptance(documentation), []);
  documentation.native.result = 'success';
  assert.notDeepEqual(mainAcceptance(documentation), [], 'planned and executed lanes must agree');
  const malformed = structuredClone(needs);
  delete malformed.plan.outputs.cli;
  assert.notDeepEqual(mainAcceptance(malformed), []);
  assert.notDeepEqual(mainAcceptance({ ...needs, plan: { result: 'failure' } }), []);
  assert.notDeepEqual(mainAcceptance({}), []);
});

test('the planner CLI reads the PR gate, ignores it on main and reports deferred lanes', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-ci-gate-test-'));
  const git = (args, input) => execFileSync('git', [
    '-c', 'user.name=CI Test', '-c', 'user.email=ci-test@example.invalid',
    '-c', 'commit.gpgsign=false', ...args,
  ], { cwd: directory, encoding: 'utf8', input }).trim();
  const planner = fileURLToPath(new URL('./ci-plan.mjs', import.meta.url));
  try {
    git(['init']);
    const emptyTree = git(['mktree'], '');
    const base = git(['commit-tree', emptyTree], 'Base\n');
    const blob = git(['hash-object', '-w', '--stdin'], 'Synthetic\n');
    const src = git(['mktree'], `100644 blob ${blob}\tcompiler.trb\n`);
    const compiler = git(['mktree'], `040000 tree ${src}\tsrc\n`);
    const head = git(['commit-tree', git(['mktree'], `040000 tree ${compiler}\tcompiler\n`), '-p', base], 'Code\n');
    const plan = (args, gate) => Object.fromEntries(execFileSync(process.execPath, [planner, ...args], {
      cwd: directory, encoding: 'utf8',
      env: plannerEnv({ NATIVE_MIR_COST_MODE: 'mir-migration', ...(gate ? { NATIVE_CI_GATE: gate } : {}) }),
    }).trim().split('\n').map(row => row.split('=')));
    assert.equal(plan([base, head, 'false']).complete, 'true', 'an unset gate stays complete');
    assert.equal(plan([base, head, 'false'], 'tiered').complete, 'false');
    assert.equal(plan([base, head, 'false', 'push'], 'tiered').complete, 'true');
    assert.equal(plan([emptyTree, head, 'false', 'push']).code, 'true', 'the empty tree plans every path');
    assert.throws(() => plan([base, head, 'false'], 'invalid'));

    const summary = join(directory, 'summary.md');
    const needs = results(classify(['compiler/src/compiler.trb'], false, 'tiered'));
    const output = execFileSync(process.execPath, [planner, 'accept'], {
      encoding: 'utf8', env: { ...process.env, NEEDS_JSON: JSON.stringify(needs), GITHUB_STEP_SUMMARY: summary },
    });
    assert.match(output, /Pre-merge lanes passed/);
    assert.match(readFileSync(summary, 'utf8'), /run on main after merge/);
    const complete = results(classify(['compiler/src/compiler.trb'], false));
    assert.doesNotMatch(execFileSync(process.execPath, [planner, 'accept'], {
      encoding: 'utf8', env: { ...process.env, NEEDS_JSON: JSON.stringify(complete), GITHUB_STEP_SUMMARY: '' },
    }), /Pre-merge lanes passed/);
    assert.throws(() => execFileSync(process.execPath, [planner, 'accept-main'], {
      encoding: 'utf8', stdio: 'pipe', env: { ...process.env, NEEDS_JSON: '{}' },
    }));
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

const registry = 'tools/native-language-cases.json';
const registryViews = ['docs/native-language-feature-inventory.md', 'docs/capabilities/ordinary-language.js'];
const modified = paths => paths.map(path => ({ path, status: 'M' }));

test('only modified registry and exact generated views select the tiered PR exception', () => {
  for (const paths of [[registry], [registry, registryViews[0]], [registry, registryViews[1]],
    [registry, ...registryViews]]) {
    for (const draft of [false, true]) {
      const plan = classify(paths, draft, 'tiered', modified(paths));
      assert.deepEqual(plan, { code: false, quick: true, compiler_units: false, documentation: true,
        memory: false, draft, tooling: false, cli: true, complete: false, registry_only: true });
      assert.deepEqual(acceptance(results(plan)), []);
      assert.equal(classify(paths, draft, 'complete', modified(paths)).registry_only, false);
      assert.equal(classify(paths, draft, 'complete', modified(paths)).compiler_units, true);
      for (const changes of [undefined, null, [], new Array(paths.length), modified(paths).slice(1),
        [...modified(paths), { path: 'unknown', status: 'M' }]]) {
        assert.equal(classify(paths, draft, 'tiered', changes).registry_only, false);
      }
      for (let index = 0; index < paths.length; index++) {
        for (const status of ['A', 'D', 'T', 'R100', 'C100', 'U', 'X', '', undefined]) {
          const changes = modified(paths);
          changes[index].status = status;
          const fallback = classify(paths, draft, 'tiered', changes);
          assert.equal(fallback.registry_only, false, `${paths[index]}: ${status}`);
          assert.equal(fallback.compiler_units, true);
        }
      }
    }
  }
  for (const other of ['compiler/src/mir.trb', 'compiler/cli/main.trb',
    'tools/native-language-coverage.py', 'tools/native-language-coverage-test.py',
    'tools/project-scenarios.py', 'tools/ci-plan.mjs', 'tools/recovery_stage_test.py',
    '.github/workflows/pull-request.yml', 'docs/other.md', 'README.md',
    'docs/capabilities/new.js', registry + '.unknown', 'unknown/file']) {
    const paths = [registry, other];
    assert.deepEqual(classify(paths, false, 'tiered', modified(paths)), classify(paths, false, 'tiered'), other);
    assert.equal(classify(paths, false, 'tiered', modified(paths)).compiler_units, true, other);
  }
  assert.equal(classify(registryViews, false, 'tiered', modified(registryViews)).registry_only, false);
  assert.equal(classify([registry], false, 'tiered', [{ path: registryViews[0], status: 'M' }]).registry_only, false);
});

test('registry acceptance requires language jobs and rejects inconsistent exception outputs', () => {
  const plan = classify([registry], false, 'tiered', modified([registry]));
  const output = execFileSync(process.execPath, [fileURLToPath(new URL('ci-plan.mjs', import.meta.url)), 'accept'], {
    encoding: 'utf8', env: { ...process.env, NEEDS_JSON: JSON.stringify(results(plan)), GITHUB_STEP_SUMMARY: '' },
  });
  assert.match(output, /Registry-only validation passed/);
  assert.doesNotMatch(output, /Native recovery.*run on main/);
  for (const job of ['quick', 'documentation', 'cli']) {
    for (const state of ['failure', 'cancelled', 'skipped', 'pending', undefined]) {
      const needs = results(plan);
      needs[job] = state ? { result: state } : undefined;
      assert.notDeepEqual(acceptance(needs), [], `${job}: ${state}`);
    }
  }
  for (const key of Object.keys(plan)) {
    const needs = results(plan);
    delete needs.plan.outputs[key];
    assert.notDeepEqual(acceptance(needs), [], `missing ${key}`);
    needs.plan.outputs[key] = 'unknown';
    assert.notDeepEqual(acceptance(needs), [], `malformed ${key}`);
    if (key === 'draft') continue;
    const inconsistent = results({ ...plan, [key]: !plan[key] });
    assert.notDeepEqual(acceptance(inconsistent), [], `inconsistent ${key}`);
  }
  for (const job of ['compiler_units', 'native', 'targets', 'memory', 'tooling']) {
    const needs = results(plan);
    needs[job].result = 'success';
    assert.notDeepEqual(acceptance(needs), [], `unexpected ${job}`);
  }
});

test('registry quick retains the reference oracle and views while skipping unrelated steps', () => {
  const entry = readFileSync(new URL('../.github/workflows/pull-request.yml', import.meta.url), 'utf8');
  assert(entry.includes('registry_only: ${{ steps.plan.outputs.registry_only }}'));
  const quick = entry.match(/^  quick:\n([\s\S]*?)(?=^  compiler_units:)/m)?.[1];
  const steps = quick.split('      - ').slice(1);
  for (const name of ['Check generated recovery import boundaries',
    'Check formatting and core types',
    'Check the Native compiler closure against snapshot v4', 'Verify project and cost tooling on Linux',
    'Verify project scenarios against the exact reference compiler', 'Run root units and focused MIR correctness tests']) {
    const step = steps.find(step => step.startsWith(`name: ${name}\n`));
    assert(step?.includes("needs.plan.outputs.registry_only != 'true'"), name);
  }
  for (const step of steps.filter(step => step.includes('repository: type-rb/type-rb') ||
    step.includes('actions/setup-go') || step.startsWith('name: Build the pinned') ||
    step.startsWith('name: Check canonical compatibility') || step.startsWith('name: Verify language fixtures') ||
    step.startsWith('name: Retain reference language') ||
    step.startsWith('name: Check that the coverage views'))) {
    assert(!step.includes('registry_only'), 'Registry edits retain reference setup, all observations and views');
    assert(step.includes("needs.plan.outputs.cli == 'true'"));
  }
});

test('real Git change statuses enable registry edits but exclude additions, deletions, renames and type changes', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'native-ci-registry-test-'));
  const git = (...args) => execFileSync('git', ['-c', 'user.name=CI Test',
    '-c', 'user.email=ci-test@example.invalid', '-c', 'commit.gpgsign=false',
    '-c', 'core.hooksPath=/dev/null', ...args], { cwd: directory, encoding: 'utf8', stdio: 'pipe' }).trim();
  const planner = fileURLToPath(new URL('ci-plan.mjs', import.meta.url));
  const write = (path, contents) => {
    mkdirSync(join(directory, path, '..'), { recursive: true });
    writeFileSync(join(directory, path), contents);
  };
  try {
    git('init');
    for (const path of [registry, ...registryViews]) write(path, 'Original\n');
    git('add', '.');
    git('commit', '-m', 'Synthetic existing registry and views');
    const base = git('rev-parse', 'HEAD');
    const check = async (expected, statuses) => {
      git('add', '-A');
      git('commit', '-m', 'Synthetic registry delta');
      const head = git('rev-parse', 'HEAD');
      const changes = await changedFiles(base, head, directory);
      assert.deepEqual(changes.map(change => change.status).sort(), statuses.sort());
      const run = mode => Object.fromEntries(execFileSync(process.execPath,
        [planner, base, head, 'false', ...(mode ? [mode] : [])], {
          cwd: directory, encoding: 'utf8', env: plannerEnv({ NATIVE_CI_GATE: 'tiered' }),
        }).trim().split('\n').map(row => row.split('=')));
      assert.equal(run().registry_only, String(expected));
      assert.equal(run().compiler_units, String(!expected));
      assert.equal(run('push').registry_only, 'false', 'Main never uses the PR exception');
      git('reset', '--hard', base);
    };
    write(registry, 'Modified\n');
    await check(true, ['M']);
    for (const path of [registry, ...registryViews]) write(path, 'Modified\n');
    await check(true, ['M', 'M', 'M']);
    rmSync(join(directory, registry));
    await check(false, ['D']);
    write(registry, 'Modified\n');
    rmSync(join(directory, registryViews[0]));
    await check(false, ['D', 'M']);
    renameSync(join(directory, registry), join(directory, 'tools/renamed.json'));
    await check(false, ['A', 'D']);
    write(registry, 'Modified\n');
    renameSync(join(directory, registryViews[0]), join(directory, registryViews[1]));
    await check(false, ['D', 'M']);
    rmSync(join(directory, registry));
    symlinkSync('../docs/native-language-feature-inventory.md', join(directory, registry));
    await check(false, ['T']);
    write(registry, 'Modified\n');
    write('unknown\npath', 'Added\n');
    await check(false, ['A', 'M']);
    // Reversing a deletion is an addition, even when the path is allowlisted.
    git('rm', registry);
    git('commit', '-m', 'Synthetic absent registry');
    const absent = git('rev-parse', 'HEAD');
    write(registry, 'Restored\n');
    git('add', '.');
    git('commit', '-m', 'Synthetic added registry');
    const changes = await changedFiles(absent, git('rev-parse', 'HEAD'), directory);
    assert.deepEqual(changes, [{ status: 'A', path: registry }]);
    assert.equal(classify([registry], false, 'tiered', changes).registry_only, false);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
