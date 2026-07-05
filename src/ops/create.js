// ops/create.js — create a new file (fails if it already exists).
import { writeFileSync, mkdirSync } from 'node:fs';
import { join, dirname } from 'node:path';

export function plan(op, ctx) {
  const base = { op, type: 'create', path: op.path };
  if (ctx.vfs.exists(op.path)) return { ...base, status: 'error', code: 'E_EXISTS' };
  const text = op.content.length ? op.content.join('\n') + '\n' : '';
  ctx.vfs.create(op.path, text);
  return { ...base, status: 'ok', content: text };
}

export function apply(result, ctx) {
  const abs = join(ctx.root, result.path);
  mkdirSync(dirname(abs), { recursive: true });
  writeFileSync(abs, result.content);
  const r = ctx.git(ctx.root, ['add', '--', result.path]);
  if (r.status !== 0) return { ok: false, code: 'E_GIT', message: r.stderr.trim() };
  ctx.created.add(result.path);
  return { ok: true };
}
