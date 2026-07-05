// ops/delete.js — stage a file deletion via `git rm`.
export function plan(op, ctx) {
  const base = { op, type: 'delete', path: op.path };
  if (!ctx.vfs.exists(op.path)) return { ...base, status: 'error', code: 'E_FILE_NOT_FOUND' };
  ctx.vfs.remove(op.path);
  return { ...base, status: 'ok' };
}

export function apply(result, ctx) {
  const r = ctx.git(ctx.root, ['rm', '-q', '--', result.path]);
  if (r.status !== 0) return { ok: false, code: 'E_GIT', message: r.stderr.trim() };
  return { ok: true };
}
