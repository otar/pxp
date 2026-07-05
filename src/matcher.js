// matcher.js — the safety-critical core. PURE: string in, result out, no IO.
//
// Two passes, no third. Pass 1 is byte-exact after trailing-whitespace + CRLF
// normalization. Pass 2 is indent-normalized EXACT equality (strip the common
// leading-whitespace prefix, compare, re-apply the target's prefix). There is no
// similarity/fuzzy/edit-distance path anywhere — an edit either matches exactly
// (modulo whitespace normalization) in exactly one place, or it fails loudly.

/**
 * @typedef {Object} MatchOk
 * @property {true} ok
 * @property {string} newText
 * @property {1|2} pass
 * @property {boolean} fuzz     true when Pass 2 (indent-normalized) was used
 * @property {number} startLine 1-based first matched line
 * @property {number} endLine   1-based last matched line
 */
/**
 * @typedef {Object} MatchFail
 * @property {false} ok
 * @property {'E_NO_MATCH'|'E_AMBIGUOUS_MATCH'} code
 * @property {number} [count]
 * @property {number[]} [locations]     1-based start lines of every match
 * @property {NearestMiss} [nearestMiss]
 */
/** @typedef {MatchOk|MatchFail} MatchResult */

/**
 * Apply a SEARCH/REPLACE edit to file text.
 * @param {string} fileText
 * @param {string[]} search  SEARCH lines (LF-normalized, no terminators)
 * @param {string[]} replace REPLACE lines (empty array = deletion)
 * @param {{strict?:boolean}} [opts]
 * @returns {MatchResult}
 */
export function applyEdit(fileText, search, replace, opts = {}) {
  const { eol, hadFinalNL } = detectEol(fileText);
  const F = splitLines(fileText, hadFinalNL);
  const n = search.length;
  if (n === 0) return { ok: false, code: 'E_NO_MATCH' };

  const Fn = F.map(norm);
  const Sn = search.map(norm);

  const rejoin = (lines) => lines.join(eol) + (hadFinalNL ? eol : '');
  const splice = (start, repl) => F.slice(0, start).concat(repl, F.slice(start + n));

  // ---- Pass 1: byte-exact after normalization ----
  const hits1 = windows(F.length, n).filter((i) => eq(Fn, i, Sn));
  if (hits1.length === 1) {
    return {
      ok: true,
      newText: rejoin(splice(hits1[0], replace)),
      pass: 1,
      fuzz: false,
      startLine: hits1[0] + 1,
      endLine: hits1[0] + n,
    };
  }
  if (hits1.length > 1) {
    return { ok: false, code: 'E_AMBIGUOUS_MATCH', count: hits1.length, locations: hits1.map((i) => i + 1) };
  }

  if (opts.strict) {
    return { ok: false, code: 'E_NO_MATCH', nearestMiss: nearestMiss(F, search) };
  }

  // ---- Pass 2: indent-normalized ----
  const Ps = commonWsPrefix(search);
  const Ss = stripPrefix(search, Ps);
  const hits2 = [];
  for (const i of windows(F.length, n)) {
    const W = F.slice(i, i + n);
    const Pw = commonWsPrefix(W);
    if (arrEq(stripPrefix(W, Pw), Ss)) hits2.push({ i, Pw });
  }
  if (hits2.length === 1) {
    const { i, Pw } = hits2[0];
    return {
      ok: true,
      newText: rejoin(splice(i, reindent(replace, Pw))),
      pass: 2,
      fuzz: true,
      startLine: i + 1,
      endLine: i + n,
    };
  }
  if (hits2.length > 1) {
    return { ok: false, code: 'E_AMBIGUOUS_MATCH', count: hits2.length, locations: hits2.map((h) => h.i + 1) };
  }
  return { ok: false, code: 'E_NO_MATCH', nearestMiss: nearestMiss(F, search) };
}

/**
 * @typedef {Object} NearestMiss
 * @property {number} fileStartLine 1-based
 * @property {number} score  count of positionally exact-matching lines
 * @property {number} of     SEARCH line count
 * @property {{line:number,text:string}[]} context  ±6 lines around the best window
 * @property {{searchLine:number,fileLine:number,expected:string,actual:string}|null} firstDivergence
 * @property {string} hint
 */

/**
 * Rank file windows by count of positionally exact-matching lines (a count, not
 * a similarity score) and return the best with ±6 lines of context.
 * @param {string[]} F file lines
 * @param {string[]} search SEARCH lines
 * @returns {NearestMiss}
 */
export function nearestMiss(F, search) {
  const n = search.length;
  const Sn = search.map(norm);
  let best = { i: 0, count: -1 };
  const last = Math.max(0, F.length - n);
  for (let i = 0; i <= last; i++) {
    let count = 0;
    for (let j = 0; j < n; j++) {
      if (F[i + j] != null && norm(F[i + j]) === Sn[j]) count++;
    }
    if (count > best.count) best = { i, count };
  }
  const ctxStart = Math.max(0, best.i - 6);
  const ctxEnd = Math.min(F.length, best.i + n + 6);
  const context = F.slice(ctxStart, ctxEnd).map((t, k) => ({ line: ctxStart + k + 1, text: cap(t, 120) }));

  let firstDivergence = null;
  for (let j = 0; j < n; j++) {
    const fileLine = F[best.i + j];
    if (fileLine == null || norm(fileLine) !== Sn[j]) {
      firstDivergence = {
        searchLine: j + 1,
        fileLine: best.i + j + 1,
        expected: cap(search[j], 120),
        actual: fileLine == null ? '<end of file>' : cap(fileLine, 120),
      };
      break;
    }
  }
  const diff = n - Math.max(0, best.count);
  return {
    fileStartLine: best.i + 1,
    score: Math.max(0, best.count),
    of: n,
    context,
    firstDivergence,
    hint: `file content differs from SEARCH at ${diff} line(s)`,
  };
}

// --- pure helpers ---

/** Strip trailing spaces/tabs (CR already removed at line split). */
function norm(s) {
  return s.replace(/[ \t]+$/, '');
}

function detectEol(text) {
  return { eol: /\r\n/.test(text) ? '\r\n' : '\n', hadFinalNL: /\r?\n$/.test(text) };
}

function splitLines(text, hadFinalNL) {
  const lines = text.split(/\r\n|\r|\n/);
  if (hadFinalNL) lines.pop(); // drop the empty element the final newline created
  return lines;
}

/** All valid start indices for a window of length n. */
function windows(len, n) {
  const out = [];
  for (let i = 0; i + n <= len; i++) out.push(i);
  return out;
}

/** Do file-normalized lines starting at `i` equal the search-normalized lines? */
function eq(Fn, i, Sn) {
  for (let j = 0; j < Sn.length; j++) if (Fn[i + j] !== Sn[j]) return false;
  return true;
}

function arrEq(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

const isBlank = (l) => l.trim() === '';
const leadingWs = (s) => s.match(/^[ \t]*/)[0];

/** Longest common leading-whitespace prefix over the non-blank lines. */
function commonWsPrefix(lines) {
  let prefix = null;
  for (const l of lines) {
    if (isBlank(l)) continue;
    const w = leadingWs(l);
    if (prefix === null) prefix = w;
    else {
      let i = 0;
      while (i < prefix.length && i < w.length && prefix[i] === w[i]) i++;
      prefix = prefix.slice(0, i);
    }
    if (prefix === '') break;
  }
  return prefix ?? '';
}

/** Remove `prefix` from each non-blank line and trailing-normalize; blanks → ''. */
function stripPrefix(lines, prefix) {
  return lines.map((l) => (isBlank(l) ? '' : norm(l.slice(prefix.length))));
}

/**
 * Re-base REPLACE onto a window whose common prefix is `Pw`: strip REPLACE's own
 * base prefix and prepend `Pw`, preserving internal relative indentation. Blank
 * lines stay empty (no injected trailing whitespace).
 */
function reindent(replace, Pw) {
  const Pr = commonWsPrefix(replace);
  return replace.map((l) => (isBlank(l) ? '' : Pw + l.slice(Pr.length)));
}

function cap(s, max) {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}
