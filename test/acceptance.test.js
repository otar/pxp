import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, mkdirSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { parse } from '../src/parser.js';
import { validate } from '../src/validate.js';
import { execute } from '../src/executor.js';

const ROOT = fileURLToPath(new URL('..', import.meta.url));
const fx = (name) => readFileSync(join(ROOT, 'test/fixtures', name), 'utf8');
const FLAGS = { strict: false, allowRun: false, keepOnFail: false, noCommit: false, color: false, json: null };

function g(dir, args, input) {
  return execFileSync('git', args, { cwd: dir, encoding: 'utf8', input });
}
function seedRepo(files) {
  const dir = mkdtempSync(join(tmpdir(), 'pxp-accept-'));
  g(dir, ['init', '-q']);
  g(dir, ['config', 'user.email', 't@pxp.dev']);
  g(dir, ['config', 'user.name', 't']);
  g(dir, ['config', 'commit.gpgsign', 'false']);
  for (const [p, content] of Object.entries(files)) {
    const abs = join(dir, p);
    mkdirSync(dirname(abs), { recursive: true });
    writeFileSync(abs, content);
  }
  g(dir, ['add', '-A']);
  g(dir, ['commit', '-q', '-m', 'init']);
  return dir;
}

test('acceptance: every fixture parses and is valid (or intentionally invalid)', () => {
  const fixtures = [
    'minimal.pxp.md',
    'fence-3backticks-in-payload.pxp.md',
    'payload-literal-keywords.pxp.md',
    'fence-escalate-markdown-target.pxp.md',
    'tabs-indent.pxp.md',
    'header-dash-variants.pxp.md',
    'empty-replace-delete.pxp.md',
    'mixed-ops.pxp.md',
    'demo.pxp.md',
  ];
  for (const name of fixtures) {
    const cs = parse(fx(name));
    const { ok, issues } = validate(cs);
    assert.ok(ok, `${name} invalid: ${JSON.stringify(issues.filter((i) => i.severity === 'error'))}`);
  }
});

test('acceptance: dogfood demo changeset applies cleanly and matches expected diff', () => {
  const dir = seedRepo({
    'src/middleware/auth.js': 'export const auth = () => {};\n',
    'src/middleware/index.js': "import { auth } from './auth.js';\n\nexport { auth };\n",
    'src/routes/api.js': "import express from 'express';\n\nconst router = express.Router();\n\nexport default router;\n",
  });
  try {
    const cs = parse(fx('demo.pxp.md'));
    const report = execute(cs, { mode: 'apply', cwd: dir, flags: { ...FLAGS }, changesetPath: null });
    assert.equal(report.result, 'applied', JSON.stringify(report.failedOp));

    assert.equal(readFileSync(join(dir, 'src/config/limits.js'), 'utf8'), 'export const MAX_PER_MINUTE = 100;\n');
    const idx = readFileSync(join(dir, 'src/middleware/index.js'), 'utf8');
    assert.ok(idx.includes("import rateLimit from 'express-rate-limit';"));
    assert.ok(idx.includes('export const limiter = rateLimit({ windowMs: 60_000, max: MAX_PER_MINUTE });'));
    const api = readFileSync(join(dir, 'src/routes/api.js'), 'utf8');
    assert.ok(api.includes('router.use(limiter);'));

    assert.equal(g(dir, ['status', '--porcelain']).trim(), '');
    assert.match(g(dir, ['log', '-1', '--pretty=%B']), /PXP-Changeset: [0-9a-f]{64}/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('acceptance: no fuzzy-matching code exists in src (safety gate)', () => {
  let hit = '';
  try {
    hit = execFileSync('git', ['grep', '-iE', 'similarity|levenshtein', '--', 'src/'], { cwd: ROOT, encoding: 'utf8' });
  } catch (e) {
    // git grep exits 1 when there are no matches — that's what we want.
    assert.equal(e.status, 1, `unexpected git grep failure: ${e.message}`);
    hit = '';
  }
  assert.equal(hit.trim(), '', `fuzzy-matching code found:\n${hit}`);
});

test('acceptance: npm pack stays under 50KB', () => {
  const out = execFileSync('npm', ['pack', '--dry-run', '--json'], { cwd: ROOT, encoding: 'utf8' });
  const meta = JSON.parse(out)[0];
  const bytes = meta.size ?? meta.unpackedSize;
  assert.ok(bytes < 50 * 1024, `package size ${bytes} bytes exceeds 50KB`);
});

test('acceptance: cold `pxp validate` runs in well under a second', () => {
  const start = process.hrtime.bigint();
  execFileSync('node', [join(ROOT, 'bin/pxp.js'), 'validate', join(ROOT, 'test/fixtures/mixed-ops.pxp.md')], { encoding: 'utf8' });
  const ms = Number(process.hrtime.bigint() - start) / 1e6;
  assert.ok(ms < 1000, `validate took ${ms.toFixed(0)}ms`);
});
