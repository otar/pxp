import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, mkdirSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';

import { parse } from '../src/parser.js';
import { execute } from '../src/executor.js';

const FLAGS = { strict: false, allowRun: false, keepOnFail: false, noCommit: false, color: false, json: null };
const flags = (o = {}) => ({ ...FLAGS, ...o });

function g(dir, args, input) {
  return execFileSync('git', args, { cwd: dir, encoding: 'utf8', input });
}

/** Create a throwaway git repo with the given {path: content} files committed. */
function setupRepo(files) {
  const dir = mkdtempSync(join(tmpdir(), 'pxp-test-'));
  g(dir, ['init', '-q']);
  g(dir, ['config', 'user.email', 'test@pxp.dev']);
  g(dir, ['config', 'user.name', 'PXP Test']);
  g(dir, ['config', 'commit.gpgsign', 'false']);
  for (const [p, content] of Object.entries(files)) {
    const abs = join(dir, p);
    mkdirSync(join(abs, '..'), { recursive: true });
    writeFileSync(abs, content);
  }
  g(dir, ['add', '-A']);
  g(dir, ['commit', '-q', '-m', 'init']);
  return dir;
}

const head = (dir) => g(dir, ['rev-parse', 'HEAD']).trim();
const porcelain = (dir) => g(dir, ['status', '--porcelain']).trim();
const cleanup = (dir) => rmSync(dir, { recursive: true, force: true });

test('happy path: create + edit + rename + delete applies and commits with trailer', () => {
  const dir = setupRepo({
    'src/order.js': 'export function total(items) {\n  return items.length;\n}\n',
    'src/old.js': 'export const old = 1;\n',
    'src/legacy.js': 'export const legacy = 1;\n',
  });
  try {
    const cs = parse([
      'pxp: 2',
      'title: Bootstrap order limits',
      'intent: Add a cap and reorganize.',
      '',
      '## Op 1: create — src/limits.js',
      '````',
      'export const MAX = 100;',
      '````',
      '',
      '## Op 2: edit — src/order.js',
      'SEARCH',
      '````',
      'export function total(items) {',
      '  return items.length;',
      '}',
      '````',
      'REPLACE',
      '````',
      "import { MAX } from './limits.js';",
      '',
      'export function total(items) {',
      '  return Math.min(items.length, MAX);',
      '}',
      '````',
      '',
      '## Op 3: rename — src/old.js -> src/renamed.js',
      '',
      '## Op 4: delete — src/legacy.js',
      '',
    ].join('\n'));

    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags() });
    assert.equal(report.result, 'applied', JSON.stringify(report.failedOp));
    assert.equal(porcelain(dir), '', 'tree clean after commit');

    assert.ok(existsSync(join(dir, 'src/limits.js')));
    assert.ok(readFileSync(join(dir, 'src/order.js'), 'utf8').includes('Math.min(items.length, MAX)'));
    assert.ok(existsSync(join(dir, 'src/renamed.js')));
    assert.ok(!existsSync(join(dir, 'src/old.js')));
    assert.ok(!existsSync(join(dir, 'src/legacy.js')));

    const msg = g(dir, ['log', '-1', '--pretty=%B']);
    assert.match(msg, /^pxp: Bootstrap order limits/);
    assert.match(msg, /PXP-Changeset: [0-9a-f]{64}/);
    assert.match(msg, /Add a cap and reorganize\./);
  } finally {
    cleanup(dir);
  }
});

test('dirty tree is refused with E_DIRTY_TREE and no writes', () => {
  const dir = setupRepo({ 'a.js': 'const a = 1;\n' });
  try {
    writeFileSync(join(dir, 'a.js'), 'const a = 999;\n'); // uncommitted change
    const before = head(dir);
    const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: create — b.js\n````\nx\n````\n');
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags() });
    assert.equal(report.result, 'error');
    assert.equal(report.failedOp.code, 'E_DIRTY_TREE');
    assert.equal(head(dir), before);
    assert.ok(!existsSync(join(dir, 'b.js')));
  } finally {
    cleanup(dir);
  }
});

test('atomicity: a failing run op rolls the tree back to the checkpoint', () => {
  const dir = setupRepo({ 'src/order.js': 'export const v = 1;\n' });
  try {
    const before = head(dir);
    const cs = parse([
      'pxp: 2',
      'title: partial',
      '',
      '## Op 1: create — src/new.js',
      '````',
      'export const n = 1;',
      '````',
      '',
      '## Op 2: edit — src/order.js',
      'SEARCH',
      '````',
      'export const v = 1;',
      '````',
      'REPLACE',
      '````',
      'export const v = 2;',
      '````',
      '',
      '## Op 3: run — boom',
      '````',
      'exit 1',
      '````',
    ].join('\n'));

    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags({ allowRun: true }) });
    assert.equal(report.result, 'error');
    assert.equal(report.failedOp.code, 'E_RUN_FAILED');
    assert.equal(report.rolledBack, true);
    // Tree byte-identical to checkpoint:
    assert.equal(head(dir), before);
    assert.equal(porcelain(dir), '');
    assert.ok(!existsSync(join(dir, 'src/new.js')), 'created file removed');
    assert.equal(readFileSync(join(dir, 'src/order.js'), 'utf8'), 'export const v = 1;\n', 'edit reverted');
  } finally {
    cleanup(dir);
  }
});

test('dry-run failure (no match) aborts before any write', () => {
  const dir = setupRepo({ 'a.js': 'line one\nline two\nline three\n' });
  try {
    const before = head(dir);
    const cs = parse([
      'pxp: 2',
      'title: x',
      '',
      '## Op 1: create — b.js',
      '````',
      'created',
      '````',
      '',
      '## Op 2: edit — a.js',
      'SEARCH',
      '````',
      'this text',
      'is not',
      'in the file',
      '````',
      'REPLACE',
      '````',
      'nope',
      '````',
    ].join('\n'));

    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags() });
    assert.equal(report.result, 'error');
    assert.equal(report.failedOp.code, 'E_NO_MATCH');
    assert.ok(report.failedOp.nearestMiss, 'nearest miss attached');
    assert.equal(head(dir), before);
    assert.equal(porcelain(dir), '', 'no writes — op 1 never ran to disk');
    assert.ok(!existsSync(join(dir, 'b.js')));
  } finally {
    cleanup(dir);
  }
});

test('plan mode makes no changes', () => {
  const dir = setupRepo({ 'a.js': 'x = 1\ny = 2\nz = 3\n' });
  try {
    const before = head(dir);
    const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: edit — a.js\nSEARCH\n````\nx = 1\ny = 2\nz = 3\n````\nREPLACE\n````\nx = 9\ny = 2\nz = 3\n````\n');
    const report = execute(cs, { mode: 'plan', cwd: dir, flags: flags() });
    assert.equal(report.result, 'planned');
    assert.equal(head(dir), before);
    assert.equal(porcelain(dir), '');
    assert.equal(readFileSync(join(dir, 'a.js'), 'utf8'), 'x = 1\ny = 2\nz = 3\n');
  } finally {
    cleanup(dir);
  }
});

test('run ops are refused on apply without --allow-run', () => {
  const dir = setupRepo({ 'a.js': 'x\n' });
  try {
    const before = head(dir);
    const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: create — b.js\n````\ny\n````\n\n## Op 2: run — v\n````\ntrue\n````\n');
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags() });
    assert.equal(report.result, 'error');
    assert.equal(report.failedOp.code, 'E_RUN_NOT_ALLOWED');
    assert.equal(head(dir), before);
    assert.ok(!existsSync(join(dir, 'b.js')), 'nothing applied when run is refused');
  } finally {
    cleanup(dir);
  }
});

test('--keep-on-fail leaves partial state on disk', () => {
  const dir = setupRepo({ 'src/order.js': 'export const v = 1;\n' });
  try {
    const cs = parse([
      'pxp: 2',
      'title: keep',
      '',
      '## Op 1: create — src/new.js',
      '````',
      'export const n = 1;',
      '````',
      '',
      '## Op 2: run — boom',
      '````',
      'exit 3',
      '````',
    ].join('\n'));
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags({ allowRun: true, keepOnFail: true }) });
    assert.equal(report.result, 'error');
    assert.equal(report.rolledBack, false);
    assert.ok(existsSync(join(dir, 'src/new.js')), 'partial create preserved');
  } finally {
    cleanup(dir);
  }
});

test('--no-commit stages changes without committing', () => {
  const dir = setupRepo({ 'a.js': 'a = 1\nb = 2\nc = 3\n' });
  try {
    const before = head(dir);
    const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: edit — a.js\nSEARCH\n````\na = 1\nb = 2\nc = 3\n````\nREPLACE\n````\na = 1\nb = 9\nc = 3\n````\n');
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags({ noCommit: true }) });
    assert.equal(report.result, 'applied');
    assert.equal(head(dir), before, 'HEAD unchanged');
    assert.notEqual(g(dir, ['diff', '--cached', '--name-only']).trim(), '', 'change is staged');
    assert.ok(readFileSync(join(dir, 'a.js'), 'utf8').includes('b = 9'));
  } finally {
    cleanup(dir);
  }
});

test('chained edits to the same file apply in order', () => {
  const dir = setupRepo({ 'a.js': 'one\ntwo\nthree\nfour\n' });
  try {
    const cs = parse([
      'pxp: 2',
      'title: chain',
      '',
      '## Op 1: edit — a.js',
      'SEARCH',
      '````',
      'one',
      'two',
      '````',
      'REPLACE',
      '````',
      'ONE',
      'two',
      '````',
      '',
      '## Op 2: edit — a.js',
      'SEARCH',
      '````',
      'three',
      'four',
      '````',
      'REPLACE',
      '````',
      'three',
      'FOUR',
      '````',
    ].join('\n'));
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: flags() });
    assert.equal(report.result, 'applied');
    assert.equal(readFileSync(join(dir, 'a.js'), 'utf8'), 'ONE\ntwo\nthree\nFOUR\n');
  } finally {
    cleanup(dir);
  }
});
