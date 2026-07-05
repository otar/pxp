import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { parse } from '../src/parser.js';
import { validate } from '../src/validate.js';

const fx = (name) => parse(readFileSync(fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url)), 'utf8'));
const codes = (issues) => issues.map((i) => i.code);

test('valid fixtures pass validation', () => {
  for (const name of ['minimal.pxp.md', 'mixed-ops.pxp.md', 'header-dash-variants.pxp.md']) {
    const { ok, issues } = validate(fx(name));
    assert.ok(ok, `${name} should be valid; issues: ${JSON.stringify(issues)}`);
  }
});

test('wrong version is an error', () => {
  const cs = parse('pxp: 1\ntitle: x\n\n## Op 1: delete — a.js\n');
  const { ok, issues } = validate(cs);
  assert.equal(ok, false);
  assert.ok(codes(issues).includes('E_VERSION'));
});

test('missing version is an error', () => {
  const cs = parse('title: x\n\n## Op 1: delete — a.js\n');
  assert.equal(validate(cs).ok, false);
});

test('no ops is an error', () => {
  const cs = parse('pxp: 2\ntitle: empty\n');
  const { ok, issues } = validate(cs);
  assert.equal(ok, false);
  assert.ok(codes(issues).includes('E_NO_OPS'));
});

test('short SEARCH block warns but does not fail', () => {
  const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: edit — a.js\n\nSEARCH\n````\nonly one line\n````\nREPLACE\n````\nchanged\n````\n');
  const { ok, issues } = validate(cs);
  assert.ok(ok);
  assert.ok(codes(issues).includes('W_SHORT_SEARCH'));
});

test('edit missing REPLACE is an error', () => {
  const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: edit — a.js\n\nSEARCH\n````\na\nb\nc\n````\n');
  const { ok, issues } = validate(cs);
  assert.equal(ok, false);
  assert.ok(codes(issues).includes('E_NO_REPLACE'));
});

test('absolute and escaping paths are errors', () => {
  const abs = parse('pxp: 2\ntitle: x\n\n## Op 1: delete — /etc/passwd\n');
  assert.ok(codes(validate(abs).issues).includes('E_ABS_PATH'));
  const esc = parse('pxp: 2\ntitle: x\n\n## Op 1: delete — ../outside.js\n');
  assert.ok(codes(validate(esc).issues).includes('E_PATH_ESCAPE'));
});

test('unknown header key warns', () => {
  const cs = parse('pxp: 2\ntitle: x\nauthor: nobody\n\n## Op 1: delete — a.js\n');
  assert.ok(codes(validate(cs).issues).includes('W_UNKNOWN_HEADER'));
});

test('run op before a file op warns about ordering', () => {
  const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: run — early\n````\ntrue\n````\n\n## Op 2: delete — a.js\n');
  assert.ok(codes(validate(cs).issues).includes('W_RUN_ORDER'));
});

test('editing a path after renaming it away is an error', () => {
  const cs = parse('pxp: 2\ntitle: x\n\n## Op 1: rename — a.js -> b.js\n\n## Op 2: edit — a.js\n\nSEARCH\n````\nx\ny\nz\n````\nREPLACE\n````\nq\n````\n');
  assert.ok(codes(validate(cs).issues).includes('E_PATH_CONTINUITY'));
});
