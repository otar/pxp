import { test } from 'node:test';
import assert from 'node:assert/strict';

import { renderTerminal, renderJson } from '../src/report.js';

const okResult = (index, type, extra = {}) => ({ op: { index }, type, status: 'ok', ...extra });

function appliedReport() {
  return {
    pxp: 2,
    mode: 'apply',
    title: 'Demo',
    intent: 'Because.',
    sha256: 'abc123',
    result: 'applied',
    results: [
      okResult(1, 'create', { path: 'a.js' }),
      okResult(2, 'edit', { path: 'b.js', pass: 1, fuzz: false }),
      okResult(3, 'edit', { path: 'c.js', pass: 2, fuzz: true }),
      okResult(4, 'rename', { from: 'd.js', to: 'e.js' }),
    ],
    completed: [1, 2, 3, 4],
    runSideEffects: [],
    committed: true,
    commit: { sha: 'deadbeefcafe', subject: 'pxp: Demo' },
  };
}

test('terminal: applied report shows terraform-style symbols and commit line', () => {
  const out = renderTerminal(appliedReport(), { color: false });
  assert.match(out, /\+ create a\.js/);
  assert.match(out, /~ edit\s+b\.js \(1 match\)/);
  assert.match(out, /≈ edit\s+c\.js \(1 match, indent fuzz\)/);
  assert.match(out, /» rename d\.js » e\.js/);
  assert.match(out, /Committed deadbee/);
  assert.match(out, /1 fuzzy/);
});

test('terminal: planned report boxes run commands and notes dry-run', () => {
  const report = {
    pxp: 2,
    mode: 'plan',
    title: 'Plan demo',
    sha256: 'x',
    result: 'planned',
    results: [okResult(1, 'edit', { path: 'a.js', pass: 1, fuzz: false }), okResult(2, 'run', { label: 'test', command: 'npm test' })],
    runOps: [{ command: ['npm test'] }],
    allowRun: false,
  };
  const out = renderTerminal(report, { color: false });
  assert.match(out, /run steps/);
  assert.match(out, /npm test/);
  assert.match(out, /Dry run only/);
});

test('json: applied report carries a summary and commit sha', () => {
  const j = renderJson(appliedReport());
  assert.equal(j.result, 'applied');
  assert.deepEqual(j.summary, { create: 1, edit: 2, delete: 0, rename: 1, run: 0, fuzz: 1 });
  assert.equal(j.commit, 'deadbeefcafe');
});

test('json: E_NO_MATCH repair packet has nearest_miss + instruction and stays small', () => {
  const nearestMiss = {
    fileStartLine: 42,
    score: 4,
    of: 6,
    context: Array.from({ length: 13 }, (_, i) => ({ line: 36 + i, text: 'x'.repeat(120) })),
    firstDivergence: { searchLine: 3, fileLine: 44, expected: 'a', actual: 'b' },
    hint: 'file content differs from SEARCH at 2 line(s)',
  };
  const report = {
    pxp: 2,
    result: 'error',
    phase: 'dryrun',
    sha256: 'sha',
    failedOp: {
      op: { index: 3, type: 'edit', path: 'src/x.js', search: ['l1', 'l2', 'l3', 'l4', 'l5', 'l6', 'l7'] },
      type: 'edit',
      path: 'src/x.js',
      code: 'E_NO_MATCH',
      nearestMiss,
    },
    completed: [],
    rolledBack: false,
    runSideEffects: [],
  };
  const j = renderJson(report);
  assert.equal(j.result, 'failed');
  assert.equal(j.failed_op.error, 'E_NO_MATCH');
  assert.equal(j.failed_op.search_first_line, 'l1');
  assert.deepEqual(j.failed_op.search_preview, ['l1', 'l2', 'l3', '…', 'l5', 'l6', 'l7']);
  assert.ok(j.failed_op.nearest_miss.context.length <= 13);
  assert.match(j.instruction, /corrected changeset/);
  // ~2K token cap: JSON well under ~8KB of text.
  assert.ok(JSON.stringify(j).length < 8000, `packet too large: ${JSON.stringify(j).length}`);
});

test('json: E_AMBIGUOUS_MATCH packet lists match lines and a disambiguation hint', () => {
  const report = {
    pxp: 2,
    result: 'error',
    phase: 'dryrun',
    sha256: 'sha',
    failedOp: { op: { index: 1, type: 'edit', path: 'a.js', search: ['x', 'y'] }, type: 'edit', path: 'a.js', code: 'E_AMBIGUOUS_MATCH', locations: [3, 17] },
    completed: [],
    rolledBack: false,
    runSideEffects: [],
  };
  const j = renderJson(report);
  assert.deepEqual(j.failed_op.match_lines, [3, 17]);
  assert.match(j.failed_op.hint, /disambiguate/);
});

test('terminal: preflight error prints code and "no changes"', () => {
  const report = {
    pxp: 2,
    mode: 'apply',
    title: 'x',
    result: 'error',
    phase: 'preflight',
    results: [],
    failedOp: { code: 'E_DIRTY_TREE', message: 'working tree is not clean' },
    rolledBack: false,
  };
  const out = renderTerminal(report, { color: false });
  assert.match(out, /E_DIRTY_TREE/);
  assert.match(out, /before any changes/);
});
