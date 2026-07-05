import { test } from 'node:test';
import assert from 'node:assert/strict';

import { applyEdit, nearestMiss } from '../src/matcher.js';

// Deterministic PRNG (mulberry32) so property tests are reproducible.
function rng(seed) {
  return () => {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const randint = (r, lo, hi) => lo + Math.floor(r() * (hi - lo + 1));

test('property: identity round-trip — REPLACE==SEARCH reproduces the file byte-for-byte', () => {
  const r = rng(1);
  for (let trial = 0; trial < 200; trial++) {
    const N = randint(r, 5, 30);
    // Unique lines guarantee every contiguous window is unique (single Pass-1 hit).
    const lines = Array.from({ length: N }, (_, i) => `line ${i} tok${randint(r, 0, 1e6)}`);
    const file = lines.join('\n') + '\n';
    const start = randint(r, 0, N - 1);
    const len = randint(r, 1, N - start);
    const search = lines.slice(start, start + len);
    const res = applyEdit(file, search, search.slice());
    assert.ok(res.ok, `trial ${trial}: expected match`);
    assert.equal(res.pass, 1);
    assert.equal(res.fuzz, false);
    assert.equal(res.newText, file, `trial ${trial}: byte-identical`);
    assert.equal(res.startLine, start + 1);
    assert.equal(res.endLine, start + len);
  }
});

test('Pass 2: indent-normalized match re-applies the file indentation', () => {
  const file = [
    'function outer() {',
    '    if (cond) {',
    '        doThing();',
    '    }',
    '}',
    '',
  ].join('\n');
  const search = ['if (cond) {', '    doThing();', '}'];
  const replace = ['if (cond) {', '    doThing();', '    doMore();', '}'];
  const res = applyEdit(file, search, replace);
  assert.ok(res.ok);
  assert.equal(res.pass, 2);
  assert.equal(res.fuzz, true);
  assert.ok(res.newText.includes('    if (cond) {'), 'if kept at 4 spaces');
  assert.ok(res.newText.includes('        doThing();'), 'body kept at 8 spaces');
  assert.ok(res.newText.includes('        doMore();'), 'new line placed at 8 spaces');
});

test('Pass 2: internal relative indentation of REPLACE is preserved when re-based', () => {
  const file = ['class A {', '  method() {', '    return 1;', '  }', '}', ''].join('\n');
  const search = ['method() {', '  return 1;', '}'];
  const replace = ['method() {', '  const x = 1;', '  if (x) {', '    return x;', '  }', '}'];
  const res = applyEdit(file, search, replace);
  assert.ok(res.ok && res.pass === 2);
  // Base '  ' from the window, REPLACE authored at col 0 with 2-space nesting.
  assert.ok(res.newText.includes('  method() {'));
  assert.ok(res.newText.includes('    const x = 1;'));
  assert.ok(res.newText.includes('      return x;'), 'nested return at 6 spaces');
});

test('ambiguity: a SEARCH occurring twice yields E_AMBIGUOUS_MATCH with count 2', () => {
  const file = ['a', 'b', 'c', 'a', 'b', 'd', ''].join('\n');
  const res = applyEdit(file, ['a', 'b'], ['A', 'B']);
  assert.equal(res.ok, false);
  assert.equal(res.code, 'E_AMBIGUOUS_MATCH');
  assert.equal(res.count, 2);
  assert.deepEqual(res.locations, [1, 4]);
});

test('--strict disables Pass 2: an indent-only match fails with E_NO_MATCH', () => {
  const file = ['function outer() {', '    if (cond) {', '        doThing();', '    }', '}', ''].join('\n');
  const search = ['if (cond) {', '    doThing();', '}'];
  const res = applyEdit(file, search, search.slice(), { strict: true });
  assert.equal(res.ok, false);
  assert.equal(res.code, 'E_NO_MATCH');
  assert.ok(res.nearestMiss);
});

test('empty REPLACE deletes the matched region', () => {
  const file = ['keep', '// BEGIN', 'gone1', 'gone2', '// END', 'keep2', ''].join('\n');
  const res = applyEdit(file, ['// BEGIN', 'gone1', 'gone2', '// END'], []);
  assert.ok(res.ok);
  assert.equal(res.newText, ['keep', 'keep2', ''].join('\n'));
});

test('trailing whitespace: normalized for matching, but REPLACE bytes are written verbatim', () => {
  const file = 'const a = 1;   \nconst b = 2;\nconst c = 3;\n'; // note trailing spaces on line 1
  const search = ['const a = 1;', 'const b = 2;', 'const c = 3;']; // no trailing space
  const res = applyEdit(file, search, ['const a = 1;', 'const b = 20;', 'const c = 3;']);
  assert.ok(res.ok, 'trailing ws is ignored for matching');
  assert.equal(res.pass, 1);
  assert.ok(res.newText.includes('const b = 20;'));
});

test('CRLF files: matched by LF SEARCH and re-emitted with CRLF', () => {
  const file = 'alpha\r\nbeta\r\ngamma\r\n';
  const res = applyEdit(file, ['alpha', 'beta', 'gamma'], ['alpha', 'BETA', 'gamma']);
  assert.ok(res.ok);
  assert.equal(res.newText, 'alpha\r\nBETA\r\ngamma\r\n');
});

test('no final newline is preserved', () => {
  const file = 'a\nb\nc'; // no trailing newline
  const res = applyEdit(file, ['a', 'b', 'c'], ['a', 'B', 'c']);
  assert.ok(res.ok);
  assert.equal(res.newText, 'a\nB\nc');
});

test('tabs are matched and preserved', () => {
  const file = 'function f() {\n\tconst a = 1;\n\treturn a;\n}\n';
  const search = ['function f() {', '\tconst a = 1;', '\treturn a;', '}'];
  const replace = ['function f() {', '\tconst a = 2;', '\treturn a;', '}'];
  const res = applyEdit(file, search, replace);
  assert.ok(res.ok);
  assert.ok(res.newText.includes('\tconst a = 2;'));
});

test('nearestMiss points at the first divergent line', () => {
  const file = ['const total = items.sum();', 'return total;', 'done'].join('\n') + '\n';
  const search = ['const total = items.sum("price");', 'return total;'];
  const res = applyEdit(file, search, ['x']);
  assert.equal(res.ok, false);
  assert.equal(res.code, 'E_NO_MATCH');
  const nm = res.nearestMiss;
  assert.equal(nm.firstDivergence.searchLine, 1);
  assert.ok(nm.context.length > 0);
  assert.match(nm.hint, /differs from SEARCH/);
});

test('nearestMiss standalone ranks the best window by exact-line count', () => {
  const F = ['x', 'a', 'b', 'z', 'a', 'B', 'c'];
  const nm = nearestMiss(F, ['a', 'b', 'c']);
  assert.equal(nm.fileStartLine, 2); // window [a,b,z] shares 2 lines; [a,B,c] shares 2 — first wins
  assert.equal(nm.of, 3);
});
