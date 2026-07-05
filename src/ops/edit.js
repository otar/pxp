// ops/edit.js — SEARCH/REPLACE edit.
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { applyEdit } from '../matcher.js';

/** Dry-run: match against the vfs and, on success, write the result back so
 * later ops on the same file chain. Never throws. */
export function plan(op, ctx) {
  const base = { op, type: 'edit', path: op.path };
  if (!ctx.vfs.exists(op.path)) return { ...base, status: 'error', code: 'E_FILE_NOT_FOUND' };
  const res = applyEdit(ctx.vfs.read(op.path), op.search, op.replace, { strict: ctx.flags.strict });
  if (!res.ok) {
    return { ...base, status: 'error', code: res.code, count: res.count, locations: res.locations, nearestMiss: res.nearestMiss };
  }
  ctx.vfs.write(op.path, res.newText);
  return { ...base, status: 'ok', pass: res.pass, fuzz: res.fuzz, newText: res.newText, startLine: res.startLine, endLine: res.endLine };
}

/** Apply: write the precomputed newText and stage it. */
export function apply(result, ctx) {
  writeFileSync(join(ctx.root, result.path), result.newText);
  const r = ctx.git(ctx.root, ['add', '--', result.path]);
  if (r.status !== 0) return { ok: false, code: 'E_GIT', message: r.stderr.trim() };
  return { ok: true };
}
