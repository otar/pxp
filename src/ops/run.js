// ops/run.js — execute a shell command. This is the ONLY op that uses a shell,
// and the only op that can fail mid-apply (file ops are fully dry-run-verified).
import { spawnSync } from 'node:child_process';

export function plan(op, ctx) {
  void ctx;
  // Not executed in dry-run — only shape-checked upstream by validate.js.
  return {
    op,
    type: 'run',
    label: op.label,
    command: op.command.join('\n'),
    expectExit: op.expectExit,
    status: 'ok',
    deferred: true,
  };
}

export function apply(result, ctx) {
  const r = spawnSync('bash', ['-c', result.command], { cwd: ctx.root, stdio: 'inherit' });
  const actualExit = typeof r.status === 'number' ? r.status : 1;
  if (actualExit !== result.expectExit) {
    return { ok: false, code: 'E_RUN_FAILED', message: `command exited ${actualExit}, expected ${result.expectExit}`, actualExit };
  }
  return { ok: true, actualExit };
}
