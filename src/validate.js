// validate.js — pure static checks over a parsed Changeset.
//
// Returns structured issues; `ok` is false only when at least one `error`-level
// issue is present. Warnings never block apply, but the CLI surfaces them.

/**
 * @typedef {Object} Issue
 * @property {number|null} op   1-based op index, or null for document-level
 * @property {string} code
 * @property {string} message
 * @property {'error'|'warn'} severity
 */

const KNOWN_HEADER_KEYS = new Set(['pxp', 'title', 'intent']);

/**
 * @param {import('./parser.js').Changeset} cs
 * @returns {{ok: boolean, issues: Issue[]}}
 */
export function validate(cs) {
  /** @type {Issue[]} */
  const issues = [];
  const err = (op, code, message) => issues.push({ op, code, message, severity: 'error' });
  const warn = (op, code, message) => issues.push({ op, code, message, severity: 'warn' });

  // --- document header ---
  if (cs.version !== 2) {
    err(null, 'E_VERSION', `expected "pxp: 2", got ${cs.version == null ? 'nothing' : cs.version}`);
  }
  if (!cs.title || cs.title.trim() === '') {
    warn(null, 'W_NO_TITLE', 'changeset has no title');
  }
  for (const key of Object.keys(cs.header)) {
    if (!KNOWN_HEADER_KEYS.has(key)) warn(null, 'W_UNKNOWN_HEADER', `unknown header key "${key}"`);
  }
  if (cs.ops.length === 0) {
    err(null, 'E_NO_OPS', 'changeset contains no ops');
  }

  // --- per-op + ordering ---
  let seenRun = false;
  /** @type {Set<string>} paths that have been renamed away from */
  const renamedFrom = new Set();
  /** @type {Set<string>} paths that now exist as a rename destination */
  const renamedTo = new Set();

  for (const op of cs.ops) {
    switch (op.type) {
      case 'edit':
        checkPath(op, op.path, err);
        if (!Array.isArray(op.search) || op.search.length === 0) {
          err(op.index, 'E_EMPTY_SEARCH', 'edit op has no SEARCH block');
        } else if (op.search.length < 3) {
          warn(op.index, 'W_SHORT_SEARCH', `SEARCH is ${op.search.length} line(s); < 3 risks ambiguity`);
        }
        if (!Array.isArray(op.replace)) {
          err(op.index, 'E_NO_REPLACE', 'edit op has no REPLACE block');
        }
        if (renamedFrom.has(op.path)) {
          err(op.index, 'E_PATH_CONTINUITY', `edits "${op.path}" after it was renamed away`);
        }
        break;
      case 'create':
        checkPath(op, op.path, err);
        if (!Array.isArray(op.content)) err(op.index, 'E_NO_CONTENT', 'create op has no content block');
        break;
      case 'delete':
        checkPath(op, op.path, err);
        break;
      case 'rename':
        checkPath(op, op.from, err, 'from');
        checkPath(op, op.to, err, 'to');
        if (!op.to) err(op.index, 'E_RENAME_DEST', 'rename op has no destination path');
        renamedFrom.add(op.from);
        renamedTo.add(op.to);
        break;
      case 'run':
        seenRun = true;
        if (!Array.isArray(op.command) || op.command.join('').trim() === '') {
          err(op.index, 'E_NO_COMMAND', 'run op has no command');
        }
        if (!Number.isInteger(op.expectExit) || op.expectExit < 0) {
          err(op.index, 'E_BAD_EXPECT', `invalid expect_exit: ${op.expectExit}`);
        }
        break;
      default:
        err(op.index, 'E_BAD_TYPE', `unknown op type "${op.type}"`);
    }

    if (seenRun && op.type !== 'run') {
      warn(op.index, 'W_RUN_ORDER', 'file op appears after a run op; put run ops last');
    }
  }

  void renamedTo; // reserved for future continuity checks; kept for symmetry

  return { ok: !issues.some((i) => i.severity === 'error'), issues };
}

/** Reject absolute paths and `..` escapes; flag empties. */
function checkPath(op, p, err, which = 'path') {
  if (p == null || p === '') {
    err(op.index, 'E_EMPTY_PATH', `${op.type} op has an empty ${which}`);
    return;
  }
  if (p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p)) {
    err(op.index, 'E_ABS_PATH', `${which} "${p}" is absolute; paths must be repo-relative`);
    return;
  }
  if (p.split(/[\\/]/).some((seg) => seg === '..')) {
    err(op.index, 'E_PATH_ESCAPE', `${which} "${p}" escapes the repo root`);
  }
}
