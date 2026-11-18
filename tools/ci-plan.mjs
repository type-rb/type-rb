import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { pathToFileURL } from 'node:url';
import { appendFileSync, readFileSync } from 'node:fs';

const staticDocumentationTools = new Set([
  'tools/capability-map-check.mjs',
  'tools/dependency-pages.py',
  'tools/benchmark-pages-data.mjs',
  'tools/benchmark-pages-check.mjs',
  'tools/result_archive.py',
  'tools/result_archive_test.py',
  'tools/evidence_bundle.py',
  'tools/evidence_bundle_test.py',
  '.github/workflows/documentation.yml',
  '.github/workflows/capability-map-pages.yml',
]);
const documentation = path => staticDocumentationTools.has(path) || path.endsWith('.md') ||
  ['.agents/', '.github/ISSUE_TEMPLATE/', 'docs/', 'results/'].some(prefix => path.startsWith(prefix));
// These exact files are exercised by the unconditional planning job. They do
// not build or execute the compiler. Execution workflows/controllers are not
// included: changing those still needs the authorities they orchestrate.
const planningTools = new Set(['tools/ci-plan.mjs', 'tools/ci-plan-test.mjs']);
const languageRegistry = 'tools/native-language-cases.json';
const registryInputs = new Set([
  languageRegistry,
  'docs/native-language-feature-inventory.md',
  'docs/capabilities/ordinary-language.js',
]);
// Only these synthetic tests can use tooling-only validation. Their production
// controllers and unknown neighboring paths still require the full code lane.
// Tests with a second Linux authority retain it through planning or quick.
export const quickToolingTests = new Set([
  'tools/compiler-project-test.sh',
  'tools/compiler-cost-test.sh',
]);
export const toolingTests = new Set([
  ...quickToolingTests,
  'tools/ci-run-suites-test.mjs',
  'tools/recovery-workspace-test.mjs',
  'tools/complete_compiler_units_test.py',
  'tools/bootstrap-seed-manifest-test.sh',
  'tools/bootstrap-seed-arguments-test.sh',
  'tools/measure-command-test.py',
  'tools/benchmarksgame-formal/runtime-controller-test.sh',
  'tools/benchmarksgame-build-formal/build-controller-test.sh',
  'tools/runtime-memory-soak/analyze-rss-test.sh',
  'tools/runtime-worker-soak/analyze-gc-trace-test.sh',
  'tools/runtime-worker-soak/analyze-process-series-test.sh',
  'tools/recovery_stage_test.py',
]);
// The tooling job discovers this daily measurement suite. Compiler and CLI
// validation do not execute these controllers, even in the complete lane.
export const dailyMeasurementInputs = new Set([
  'tools/daily-performance/measure.py',
  'tools/daily-performance/state.py',
  'tools/daily-performance/test_daily.py',
  'tools/daily-performance/rss_policy.py',
  'tools/daily-performance/test_rss_policy.py',
]);
// These existing test modules are excluded from ordinary compiler builds.
// Keep complete correctness validation; only unchanged-binary measurements
// are unnecessary. New test paths, configurations and fixtures default to code.
export const compilerTestInputs = new Set([
  'compiler/src/frontend/checking/body/statements_bindings_test.trb',
  'compiler/src/frontend/checking/body/expressions_test.trb',
  'compiler/src/tests/compiler/source_loading_test.trb',
  'compiler/src/support/literals_test.trb',
  'compiler/src/mir/model/module_test.trb',
  'compiler/src/frontend/checking/collections/hashes_test.trb',
  'compiler/src/frontend/syntax/syntax_tokens_test.trb',
  'compiler/src/frontend/syntax/syntax_forms_test.trb',
  'compiler/src/project/config_test.trb',
  'compiler/src/backend/qbe/output_test.trb',
  'compiler/src/frontend/resolution/type_resolution_test.trb',
  'compiler/src/state/state_test.trb',
]);
// These adapters are outside the ordinary compiler source closure. Core edits
// mixed with them restore all core authorities; new paths default to code.
export const cliInputs = new Set([
  'trbn',
  'tools/build-native.sh',
  'tools/check-native-cli.sh',
  'tools/native-cli-test.py',
  'tools/native-enum-test.py',
  'tools/native-callable-test.py',
  'tools/native-repl-flow-test.py',
  'tools/native-process-test.py',
  'tools/native-hash-test.py',
  'tools/native-cli-key-test.py',
  'tools/native-puts-test.py',
  'tools/native-diagnostics-test.py',
  'tools/native-interpolation-test.py',
  'tools/native-bootstrap-test.py',
  'tools/native-repl-editor-test.py',
  'tools/native-language-coverage.py',
  'tools/native-language-coverage-test.py',
  'tools/native-language-cases.json',
  'tools/repl-test-requirements.txt',
  'compiler/cli/diagnostics.trb',
  'compiler/cli/host.trb',
  'compiler/cli/main.trb',
  'compiler/cli/repl/session.trb',
  'compiler/cli/repl/check.trb',
  'compiler/cli/repl/defaults.trb',
  'compiler/cli/repl/project.trb',
  'compiler/cli/repl/editor.trb',
  'compiler/cli/repl/eval.trb',
  'compiler/cli/repl/history.trb',
  'compiler/cli/repl/syntax.trb',
  'compiler/cli/repl/values.trb',
  'compiler/cli/repl/model.trb',
  'compiler/cli/repl/callables.trb',
  'compiler/cli/repl/types.trb',
  'compiler/cli/repl/hash.trb',
]);

// A tiered PR gate integrates ordinary compiler, CLI and conformance edits with
// the pre-merge lanes; main then runs the complete lanes. Cache and bootstrap
// inputs, recovery sources, workflows and unknown paths still need the complete
// lanes before merge because only those lanes execute them.
const completeCliInputs = new Set(['trbn', 'tools/build-native.sh', 'tools/native-bootstrap-test.py']);
const pullRequestLane = path =>
  ['compiler/src/', 'compiler/cli/', 'compiler/conformance/'].some(prefix => path.startsWith(prefix)) ||
  ['compiler/trbconfig.jsonc', 'recovery/src/compiler/layout.trb', 'recovery/src/compiler/mutations.trb',
    'tools/check-conformance-sources.py'].includes(path) ||
  (cliInputs.has(path) && !completeCliInputs.has(path)) || toolingTests.has(path);
export const gates = ['complete', 'tiered'];

// Only an unchanged recovery harness may narrow the expensive per-module
// mutations. Unknown paths and source identities restore the full control set.
export function recoveryModules(paths, moduleNames) {
  const names = new Set();
  for (const path of paths) {
    if (documentation(path)) continue;
    const match = /^compiler\/src\/([a-z][a-z0-9_]*(?:\/[a-z][a-z0-9_]*)*)\.trb$/.exec(path);
    if (!match) return 'all';
    const name = match[1];
    if (moduleNames.has(name)) {
      if (name !== 'compiler') names.add(name);
    } else if (!name.endsWith('_test')) {
      return 'all';
    }
  }
  return names.size ? [...names].sort().join(',') : 'none';
}

function recoveryModuleNames() {
  const layout = readFileSync(new URL('../recovery/src/compiler/layout.trb', import.meta.url), 'utf8');
  const names = [...layout.matchAll(/RecoveryCompilerModule\.new\(name: "([a-z][a-z0-9_]*(?:\/[a-z][a-z0-9_]*)*)"/g)]
    .map(match => match[1]);
  if (!names.includes('compiler') || new Set(names).size !== names.length ||
      names.length !== [...layout.matchAll(/RecoveryCompilerModule\.new\(name:/g)].length) {
    throw new Error('Recovery module inventory is missing or duplicated');
  }
  return new Set(names);
}

export function classify(paths, draft, gate = 'complete', changes = []) {
  if (!gates.includes(gate)) throw new Error('Invalid CI gate');
  // Only content edits to the existing registry and its exact generated views
  // may omit unrelated PR units. Name-only inventories cannot prove this.
  // With rename detection disabled, a rename includes a deletion/addition.
  const registryOnly = gate === 'tiered' && paths.includes(languageRegistry) &&
    paths.every(path => registryInputs.has(path)) && Array.isArray(changes) &&
    changes.length === paths.length && paths.every((path, index) =>
      changes[index]?.path === path && changes[index].status === 'M');
  const executable = paths.filter(path => !documentation(path) && !planningTools.has(path));
  const toolingInput = path => toolingTests.has(path) || dailyMeasurementInputs.has(path);
  const codePaths = executable.filter(path => !toolingInput(path) && !cliInputs.has(path));
  const code = codePaths.length > 0;
  const routing = paths.some(path => path.startsWith('.github/workflows/') || path.startsWith('tools/ci-'));
  const compiler = codePaths.some(path => path.startsWith('compiler/')) &&
    !codePaths.every(path => compilerTestInputs.has(path));
  const policy = codePaths.some(path => path.startsWith('tools/native-mir-') ||
    path.startsWith('tools/compiler-project') || path.startsWith('tools/compiler-cost'));
  const memory = code && (routing || compiler || policy || codePaths.some(path => path.startsWith('tools/runtime-worker-soak/')));
  const cli = code || executable.some(path => cliInputs.has(path));
  const complete = cli && (gate === 'complete' || executable.some(path => !pullRequestLane(path)));
  return {
    code, quick: code || executable.some(path => cliInputs.has(path) || quickToolingTests.has(path)),
    compiler_units: (code || cli) && !registryOnly,
    documentation: registryOnly || routing || paths.some(documentation),
    memory, draft,
    tooling: code || executable.some(toolingInput),
    cli, complete, registry_only: registryOnly,
  };
}

export function acceptance(needs) {
  if (needs.plan?.result !== 'success') return ['CI planning did not succeed'];
  const plan = needs.plan.outputs;
  if (!plan || ['code', 'documentation', 'memory', 'draft', 'tooling', 'cli', 'quick', 'compiler_units', 'complete', 'registry_only']
    .some(key => !['true', 'false'].includes(plan[key]))) {
    return ['CI planning outputs are missing or malformed'];
  }
  if ((plan.code === 'true' || plan.cli === 'true') && plan.quick !== 'true') {
    return ['Code and CLI validation require quick feedback'];
  }
  if (plan.registry_only === 'true' && Object.entries({
    code: 'false', quick: 'true', compiler_units: 'false', documentation: 'true',
    memory: 'false', tooling: 'false', cli: 'true', complete: 'false',
  }).some(([key, value]) => plan[key] !== value)) {
    return ['Registry-only validation has inconsistent authorities'];
  }
  if ((plan.code === 'true' || plan.cli === 'true') && plan.registry_only !== 'true' && plan.compiler_units !== 'true') {
    return ['Code and CLI validation require complete compiler units'];
  }
  const draft = plan.draft === 'true';
  const required = {
    quick: plan.quick, compiler_units: plan.compiler_units, documentation: plan.documentation,
    native: draft || plan.complete === 'false' ? 'false' : plan.code, targets: draft ? 'false' : plan.code,
    memory: draft ? 'false' : plan.memory,
    tooling: plan.tooling, cli: draft ? 'false' : plan.cli,
  };
  return Object.entries(required).flatMap(([job, enabled]) => {
    const expected = enabled === 'true' ? 'success' : 'skipped';
    return needs[job]?.result === expected ? [] :
      [`${job}: expected ${expected}, got ${needs[job]?.result ?? 'missing'}`];
  });
}

// Main validation owns the complete lanes after a tiered PR merge. Its planner
// compares against the last fully validated main commit, not the push delta.
export function mainAcceptance(needs) {
  if (needs.plan?.result !== 'success') return ['Main planning did not succeed'];
  const plan = needs.plan.outputs;
  if (!plan || ['code', 'documentation', 'memory', 'tooling', 'cli']
    .some(key => !['true', 'false'].includes(plan[key]))) {
    return ['Main planning outputs are missing or malformed'];
  }
  const required = {
    documentation: plan.documentation, tooling: plan.tooling, memory: plan.memory,
    native: plan.code, targets: plan.code, cli: plan.cli,
  };
  return Object.entries(required).flatMap(([job, enabled]) => {
    const expected = enabled === 'true' ? 'success' : 'skipped';
    return needs[job]?.result === expected ? [] :
      [`${job}: expected ${expected}, got ${needs[job]?.result ?? 'missing'}`];
  });
}

export async function changedFiles(base, head, cwd, direct = false) {
  // Stream the NUL-delimited list: full evidence snapshots can exceed the
  // synchronous child-process buffer, and truncation could hide a code change.
  const child = spawn('git', ['diff', '--no-renames', '--name-status', '-z',
    `${base}${direct ? '..' : '...'}${head}`, '--'], { cwd, stdio: ['ignore', 'pipe', 'inherit'] });
  const completion = once(child, 'close');
  child.stdout.setEncoding('utf8');
  const changes = [];
  const read = async () => {
    let pending = '';
    let status;
    for await (const chunk of child.stdout) {
      const parts = (pending + chunk).split('\0');
      pending = parts.pop();
      for (const part of parts) {
        if (status === undefined) status = part;
        else {
          changes.push({ status, path: part });
          status = undefined;
        }
      }
    }
    if (pending !== '' || status !== undefined) throw new Error('Git change list is incomplete');
  };
  const [, [status, signal]] = await Promise.all([read(), completion]);
  if (status !== 0) throw new Error(`Git path inventory failed: ${signal ?? status}`);
  return changes;
}

export async function changedPaths(base, head, cwd, direct = false) {
  return (await changedFiles(base, head, cwd, direct)).map(change => change.path);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.argv[2] === 'accept' || process.argv[2] === 'accept-main') {
    const needs = JSON.parse(process.env.NEEDS_JSON ?? '{}');
    const errors = process.argv[2] === 'accept' ? acceptance(needs) : mainAcceptance(needs);
    for (const error of errors) console.error(error);
    const outputs = needs.plan?.outputs ?? {};
    const message = errors.length || process.argv[2] !== 'accept' ? '' :
      outputs.draft === 'true' ? 'Draft feedback only: complete validation runs when the PR is marked ready.' :
      outputs.registry_only === 'true' ?
        'Registry-only validation passed: reference and Native language checks retained; unrelated compiler units omitted.' :
      outputs.complete === 'false' && outputs.cli === 'true' ?
        'Pre-merge lanes passed: Native recovery, CLI cache and arm64 regression run on main after merge.' : '';
    if (message) {
      console.log(message);
      if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY,
        `### ${message}\n`);
    }
    process.exitCode = errors.length ? 1 : 0;
  } else {
    const [base, head, draft, mode] = process.argv.slice(2);
    if (!/^[a-f0-9]{40}$/.test(base ?? '') ||
        !/^[a-f0-9]{40}$/.test(head ?? '') || !['true', 'false'].includes(draft) || (mode !== undefined && mode !== 'push') || process.argv.length > 6) {
      throw new Error('Usage: ci-plan.mjs BASE_SHA HEAD_SHA true|false [push]');
    }
    // Include both sides of renames, and preserve arbitrary path characters.
    const changes = await changedFiles(base, head, undefined, mode === 'push');
    const paths = changes.map(change => change.path);
    // Main always plans the complete lanes; only PRs may use the tiered gate.
    const gate = mode === 'push' ? 'complete' : process.env.NATIVE_CI_GATE ?? 'complete';
    for (const [key, value] of Object.entries(classify(paths, draft === 'true', gate, changes))) {
      console.log(`${key}=${value}`);
    }
    if (mode === 'push') console.log(`recovery_modules=${recoveryModules(paths, recoveryModuleNames())}`);
  }
}
