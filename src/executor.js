// executor.js — preflight → full dry-run → ordered apply → commit | rollback.
//
// Side-effecting orchestrator. All git goes through git.js (array args); the
// only shell is in ops/run.js. Atomicity guarantee: no file op writes to disk
// until every file op has passed the dry-run, and any failure during apply
// rolls the tree back to the pre-apply checkpoint.

import { relative, resolve } from 'node:path';
import { git } from './git.js';
import { createVfs } from './vfs.js';
import * as editOp from './ops/edit.js';
import * as createOp from './ops/create.js';
import * as deleteOp from './ops/delete.js';
import * as renameOp from './ops/rename.js';
import * as runOp from './ops/run.js';

const OPS = { edit: editOp, create: createOp, delete: deleteOp, rename: renameOp, run: runOp };

/**
 * @param {import('./parser.js').Changeset} cs
 * @param {{mode:'plan'|'apply', cwd:string, flags:object}} env
 * @returns {object} report
 */
export function execute(cs, env) {
  const { mode, cwd, flags, changesetPath } = env;

  // --- Preflight ---
  const pre = preflight(cwd, changesetPath);
  if (pre.error) {
    return baseReport(cs, mode, [], { result: 'error', phase: 'preflight', failedOp: { status: 'error', ...pre.error }, rolledBack: false, completed: [], runSideEffects: [] });
  }
  const { root, head: checkpoint } = pre;

  // --- Full dry-run over ALL ops (no writes) ---
  const vfs = createVfs(root);
  const ctx = { root, vfs, git, flags, created: new Set() };
  const results = [];
  let failed = null;
  for (const op of cs.ops) {
    const res = OPS[op.type].plan(op, ctx);
    results.push(res);
    if (res.status === 'error') {
      failed = res;
      break;
    }
  }

  if (failed) {
    return baseReport(cs, mode, results, {
      result: 'error',
      phase: 'dryrun',
      failedOp: failed,
      rolledBack: false,
      completed: [], // dry-run failure: nothing was written
      runSideEffects: [],
      checkpoint,
    });
  }

  const runOps = cs.ops.filter((o) => o.type === 'run');

  // --- plan command stops here ---
  if (mode === 'plan') {
    return baseReport(cs, mode, results, { result: 'planned', runOps, allowRun: flags.allowRun, checkpoint });
  }

  // --- apply: refuse run ops without explicit approval (before any write) ---
  if (runOps.length > 0 && !flags.allowRun) {
    const runResult = results.find((r) => r.type === 'run');
    return baseReport(cs, mode, results, {
      result: 'error',
      phase: 'preflight',
      failedOp: { ...runResult, status: 'error', code: 'E_RUN_NOT_ALLOWED', message: 'changeset contains run ops; re-run with --allow-run to execute them' },
      rolledBack: false,
      completed: [],
      runSideEffects: [],
      checkpoint,
    });
  }

  // --- apply in order ---
  const completed = [];
  const runSideEffects = [];
  for (const res of results) {
    const ap = OPS[res.type].apply(res, ctx);
    if (res.type === 'run' && ap.ok) runSideEffects.push({ command: res.command, exit: ap.actualExit });
    if (!ap.ok) {
      const rolledBack = flags.keepOnFail ? false : rollback(root, checkpoint, ctx.created);
      return baseReport(cs, mode, results, {
        result: 'error',
        phase: 'apply',
        failedOp: { ...res, status: 'error', code: ap.code, message: ap.message, actualExit: ap.actualExit },
        rolledBack,
        completed,
        runSideEffects,
        checkpoint,
      });
    }
    completed.push(res.op.index);
  }

  // --- success: commit ---
  let commit = null;
  if (!flags.noCommit) commit = commitChangeset(root, cs);

  return baseReport(cs, mode, results, { result: 'applied', completed, runSideEffects, committed: !flags.noCommit, commit, checkpoint });
}

function preflight(cwd, changesetPath) {
  const top = git(cwd, ['rev-parse', '--show-toplevel']);
  if (top.status !== 0) return { error: { code: 'E_NOT_REPO', message: 'not inside a git repository' } };
  const root = top.stdout.trim();
  const head = git(root, ['rev-parse', 'HEAD']);
  if (head.status !== 0) return { error: { code: 'E_NO_HEAD', message: 'repository has no commits (HEAD unborn)' } };

  // The changeset file itself commonly lives (untracked) in the repo; it does
  // not count as a dirty tree, so exclude it from the clean-tree check.
  const ignore = changesetPath ? relative(root, resolve(cwd, changesetPath)) : null;
  const dirty = git(root, ['status', '--porcelain'])
    .stdout.split('\n')
    .filter((l) => l.trim() !== '')
    .filter((l) => l.slice(3).trim() !== ignore);
  if (dirty.length > 0) return { error: { code: 'E_DIRTY_TREE', message: 'working tree is not clean; commit or stash first' } };

  return { root, head: head.stdout.trim() };
}

/** Restore the tree to the pre-apply checkpoint and remove created files. */
function rollback(root, checkpoint, created) {
  git(root, ['reset', '-q', '--hard', checkpoint]);
  const paths = [...created];
  if (paths.length) git(root, ['clean', '-fdq', '--', ...paths]);
  return true;
}

function commitChangeset(root, cs) {
  let msg = `pxp: ${cs.title || 'changeset'}\n`;
  if (cs.intent && cs.intent.trim()) msg += `\n${cs.intent.trim()}\n`;
  msg += `\nPXP-Changeset: ${cs.sha256}\n`;
  const r = git(root, ['commit', '-q', '-F', '-'], { input: msg });
  if (r.status !== 0) return { error: r.stderr.trim() };
  return { sha: git(root, ['rev-parse', 'HEAD']).stdout.trim(), subject: `pxp: ${cs.title || 'changeset'}` };
}

function baseReport(cs, mode, results, extra) {
  return { pxp: 2, mode, title: cs.title, intent: cs.intent, sha256: cs.sha256, results, ...extra };
}
