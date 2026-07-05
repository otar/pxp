// parser.js — PXP v2 changeset dialect parser.
//
// Single-pass, line-based. We parse a *dialect*, not Markdown — no Markdown
// library. The dominant rule that makes the carrier adversary-proof is
// FENCE-FIRST: once inside a fenced payload, only a matching close line ends it,
// so payload lines that read `SEARCH`, `## Op ...`, or ``` are inert.
//
// Pure: no filesystem, no side effects. Throws ParseError (with a line number)
// on structurally malformed input; everything softer is left to validate.js.

import { createHash } from 'node:crypto';

/** Structured parse failure carrying the offending 1-based line number. */
export class ParseError extends Error {
  /** @param {string} message @param {number|null} [line] */
  constructor(message, line = null) {
    super(line != null ? `Line ${line}: ${message}` : message);
    this.name = 'ParseError';
    this.line = line;
    this.code = 'E_PARSE';
  }
}

/**
 * @typedef {Object} Changeset
 * @property {number|null} version   parsed `pxp:` header value
 * @property {string} title
 * @property {string} intent
 * @property {Record<string,string>} header  all recognized header key/values
 * @property {Op[]} ops
 * @property {string} raw             original document text
 * @property {string} sha256          sha256 of the raw document bytes
 */

/** @typedef {Object} Op discriminated by `type`; see newOp() for shapes. */

const OP_HEADER = /^#{1,6}\s+Op\s+(\d+)\s*:\s*(edit|create|delete|rename|run)\b[ \t]*(?:[—–-][ \t]*)?(.*?)[ \t]*$/i;
const HEADER_KV = /^([A-Za-z][\w-]*):[ \t]?(.*)$/;
const OPEN_FENCE = /^(`{4,})[ \t]*$/;
const FENCE_ONLY = /^(`+)[ \t]*$/;
const SEARCH_KW = /^#{0,3}[ \t]*SEARCH[ \t]*$/;
const REPLACE_KW = /^#{0,3}[ \t]*REPLACE[ \t]*$/;
const EXPECT_EXIT = /^[ \t]*expect_exit:[ \t]*(\d+)[ \t]*$/;
const RENAME_ARROW = /^(.*?)\s+(?:->|→)\s+(.*)$/;

/**
 * Parse a `.pxp.md` document into a Changeset IR.
 * @param {string} text
 * @returns {Changeset}
 */
export function parse(text) {
  const sha256 = createHash('sha256').update(text, 'utf8').digest('hex');
  const lines = text.split(/\r\n|\r|\n/);

  /** @type {Record<string,string>} */
  const header = {};
  /** @type {Op[]} */
  const ops = [];
  let mode = /** @type {'header'|'body'} */ ('header');
  let cur = /** @type {any} */ (null);
  let fence = /** @type {{len:number, field:string}|null} */ (null);
  let pendingField = /** @type {'search'|'replace'|null} */ (null);

  const finalize = () => {
    if (!cur) return;
    cur.rationale = trimBlank(cur.rationale).join('\n');
    ops.push(cur);
    cur = null;
    pendingField = null;
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const lineNo = i + 1;

    // 1. Inside a fence — only a close of >= the opening length ends it.
    if (fence) {
      const m = FENCE_ONLY.exec(line);
      if (m && m[1].length >= fence.len) {
        fence = null;
      } else {
        cur[fence.field].push(line);
      }
      continue;
    }

    // 2. Header block: `key: value` lines until the first op header.
    if (mode === 'header') {
      if (!OP_HEADER.test(line)) {
        if (line.trim() === '') continue;
        const kv = HEADER_KV.exec(line);
        if (kv) header[kv[1]] = kv[2];
        continue; // stray prose in the header is ignored
      }
      mode = 'body'; // fall through to body handling
    }

    // 3. Body.
    const oh = OP_HEADER.exec(line);
    if (oh) {
      finalize();
      cur = newOp(Number(oh[1]), oh[2].toLowerCase(), oh[3], lineNo);
      continue;
    }
    if (!cur) throw new ParseError('content before the first op header', lineNo);

    if (SEARCH_KW.test(line)) {
      if (cur.type !== 'edit') throw new ParseError('SEARCH outside an edit op', lineNo);
      pendingField = 'search';
      continue;
    }
    if (REPLACE_KW.test(line)) {
      if (cur.type !== 'edit') throw new ParseError('REPLACE outside an edit op', lineNo);
      pendingField = 'replace';
      continue;
    }
    const ee = EXPECT_EXIT.exec(line);
    if (ee && cur.type === 'run') {
      cur.expectExit = Number(ee[1]);
      continue;
    }

    const of = OPEN_FENCE.exec(line);
    if (of) {
      const field = resolveField(cur, pendingField, lineNo);
      if (cur[field] != null) {
        throw new ParseError(`duplicate ${field} block in Op ${cur.index}`, lineNo);
      }
      cur[field] = [];
      fence = { len: of[1].length, field };
      pendingField = null;
      continue;
    }

    cur.rationale.push(line); // free prose → rationale (report-only)
  }

  if (fence) throw new ParseError(`unterminated fenced block (${fence.field})`, lines.length);
  finalize();

  return {
    version: header.pxp != null && header.pxp !== '' ? Number(header.pxp) : null,
    title: header.title ?? '',
    intent: header.intent ?? '',
    header,
    ops,
    raw: text,
    sha256,
  };
}

/**
 * Build a fresh op record. Payload fields start as `null` (absent) and become
 * `string[]` once their fence is captured; an empty fence yields `[]`.
 */
function newOp(index, type, remainder, line) {
  const base = { index, type, rationale: /** @type {string[]} */ ([]), line };
  switch (type) {
    case 'edit':
      return { ...base, path: remainder, search: null, replace: null };
    case 'create':
      return { ...base, path: remainder, content: null };
    case 'delete':
      return { ...base, path: remainder };
    case 'rename': {
      const m = RENAME_ARROW.exec(remainder);
      const from = m ? m[1].trim() : remainder.trim();
      const to = m ? m[2].trim() : '';
      return { ...base, from, to };
    }
    case 'run':
      return { ...base, label: remainder, command: null, expectExit: 0 };
    default:
      throw new ParseError(`unknown op type "${type}"`, line);
  }
}

/** Decide which field an opening fence fills, given op type and pending keyword. */
function resolveField(cur, pendingField, lineNo) {
  if (pendingField) return pendingField;
  if (cur.type === 'create') return 'content';
  if (cur.type === 'run') return 'command';
  if (cur.type === 'edit') {
    throw new ParseError(`fenced block in edit Op ${cur.index} with no preceding SEARCH/REPLACE`, lineNo);
  }
  throw new ParseError(`${cur.type} Op ${cur.index} does not take a fenced block`, lineNo);
}

/** Drop leading/trailing all-blank lines. */
function trimBlank(arr) {
  let s = 0;
  let e = arr.length;
  while (s < e && arr[s].trim() === '') s++;
  while (e > s && arr[e - 1].trim() === '') e--;
  return arr.slice(s, e);
}
