// ops/rename.js — move a file via `git mv`.
import { mkdirSync } from 'node:fs';
import { join, dirname } from 'node:path';

export function plan(op, ctx) {
  const base = { op, type: 'rename', from: op.from, to: op.to };
  if (!ctx.vfs.exists(op.from)) return { ...base, status: 'error', code: 'E_FILE_NOT_FOUND', path: op.from };
  if (ctx.vfs.exists(op.to)) return { ...base, status: 'error', code: 'E_EXISTS', path: op.to };
  ctx.vfs.move(op.from, op.to);
  return { ...base, status: 'ok' };
}

export function apply(result, ctx) {
  mkdirSync(dirname(join(ctx.root, result.to)), { recursive: true });
  const r = ctx.git(ctx.root, ['mv', '--', result.from, result.to]);
  if (r.status !== 0) return { ok: false, code: 'E_GIT', message: r.stderr.trim() };
  ctx.created.add(result.to);
  return { ok: true };
}
