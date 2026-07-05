import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { parse, ParseError } from '../src/parser.js';

const fx = (name) => readFileSync(fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url)), 'utf8');

test('minimal: header + single edit op', () => {
  const cs = parse(fx('minimal.pxp.md'));
  assert.equal(cs.version, 2);
  assert.equal(cs.title, 'Minimal single edit');
  assert.match(cs.intent, /smallest useful/);
  assert.equal(cs.ops.length, 1);
  const op = cs.ops[0];
  assert.equal(op.type, 'edit');
  assert.equal(op.path, 'src/app.js');
  assert.equal(op.rationale, 'Bump the middle constant.');
  assert.deepEqual(op.search, ['const x = 1;', 'const y = 2;', 'const z = 3;']);
  assert.deepEqual(op.replace, ['const x = 1;', 'const y = 20;', 'const z = 3;']);
});

test('adversarial: triple-backtick payload stays inert under a 4-backtick carrier', () => {
  const cs = parse(fx('fence-3backticks-in-payload.pxp.md'));
  assert.equal(cs.ops.length, 1);
  const op = cs.ops[0];
  assert.equal(op.type, 'create');
  assert.equal(op.path, 'docs/snippet.md');
  assert.ok(op.content.includes('```js'), 'inner ```js line preserved verbatim');
  assert.ok(op.content.includes('```'), 'inner closing ``` preserved verbatim');
  assert.equal(op.content[0], 'Here is some example code:');
});

test('adversarial: literal SEARCH/REPLACE/op-header lines inside a fence are inert', () => {
  const cs = parse(fx('payload-literal-keywords.pxp.md'));
  assert.equal(cs.ops.length, 1, 'the fake "## Op 9" line did not start a new op');
  const op = cs.ops[0];
  assert.equal(op.type, 'create');
  assert.deepEqual(op.content, [
    'SEARCH',
    'REPLACE',
    '## Op 9: edit — nope.js',
    'expect_exit: 7',
    'plain trailing text',
  ]);
});

test('adversarial: nested 4-backtick fences survive a 5-backtick carrier', () => {
  const cs = parse(fx('fence-escalate-markdown-target.pxp.md'));
  const op = cs.ops[0];
  assert.equal(op.type, 'edit');
  assert.equal(op.path, 'README.md');
  assert.ok(op.search.includes('````'), 'inner 4-backtick fence preserved in SEARCH');
  assert.ok(op.replace.includes('````'), 'inner 4-backtick fence preserved in REPLACE');
});

test('tabs are preserved verbatim in payloads', () => {
  const cs = parse(fx('tabs-indent.pxp.md'));
  const op = cs.ops[0];
  assert.ok(op.search[1].startsWith('\t'), 'SEARCH line keeps its leading tab');
  assert.ok(op.replace[1].startsWith('\t'), 'REPLACE line keeps its leading tab');
  assert.equal(op.search[1], '\tconst a = 1;');
});

test('em-dash, en-dash, hyphen separators and ### SEARCH all parse', () => {
  const cs = parse(fx('header-dash-variants.pxp.md'));
  assert.equal(cs.ops.length, 3);
  assert.deepEqual(cs.ops.map((o) => o.path), ['src/a.js', 'src/b.js', 'src/c.js']);
  assert.ok(cs.ops.every((o) => o.type === 'edit' && o.search.length === 3 && o.replace.length === 3));
});

test('empty REPLACE block yields an empty array (deletion)', () => {
  const cs = parse(fx('empty-replace-delete.pxp.md'));
  const op = cs.ops[0];
  assert.deepEqual(op.replace, []);
  assert.equal(op.search.length, 3);
});

test('mixed ops parse with correct types, paths, and run fields', () => {
  const cs = parse(fx('mixed-ops.pxp.md'));
  assert.deepEqual(cs.ops.map((o) => o.type), ['create', 'edit', 'rename', 'delete', 'run']);
  const rename = cs.ops[2];
  assert.equal(rename.from, 'src/old.js');
  assert.equal(rename.to, 'src/new.js');
  const run = cs.ops[4];
  assert.equal(run.label, 'verify');
  assert.deepEqual(run.command, ['npm test']);
  assert.equal(run.expectExit, 0);
  assert.match(run.rationale, /after all file ops/);
});

test('CRLF line endings are normalized at the split boundary', () => {
  const doc = ['pxp: 2', 'title: crlf', '', '## Op 1: create — a.txt', '', '````', 'x', 'y', '````', ''].join('\r\n');
  const cs = parse(doc);
  assert.equal(cs.ops.length, 1);
  assert.deepEqual(cs.ops[0].content, ['x', 'y']);
});

test('sha256 is stable over the raw bytes', () => {
  const text = fx('minimal.pxp.md');
  assert.equal(parse(text).sha256, parse(text).sha256);
  assert.match(parse(text).sha256, /^[0-9a-f]{64}$/);
});

test('unterminated fence throws ParseError with a line number', () => {
  const doc = 'pxp: 2\ntitle: x\n\n## Op 1: create — a.txt\n\n````\nnever closed\n';
  assert.throws(() => parse(doc), (e) => e instanceof ParseError && e.line != null);
});

test('SEARCH keyword outside an edit op throws', () => {
  const doc = 'pxp: 2\ntitle: x\n\n## Op 1: create — a.txt\n\nSEARCH\n````\nx\n````\n';
  assert.throws(() => parse(doc), ParseError);
});
