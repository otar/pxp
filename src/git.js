// git.js — the single choke point for shelling out to git.
//
// Every call passes an ARGUMENT ARRAY (never a shell string), so paths and
// refs can never be interpreted by a shell. This is the only file that runs
// git; audit shell-injection safety here and nowhere else.

import { execFileSync } from 'node:child_process';

/**
 * Run a git command. Never throws — returns a structured result so callers can
 * branch on `status`.
 * @param {string} root  cwd for the command (repo root)
 * @param {string[]} args
 * @param {{input?:string}} [opts]
 * @returns {{stdout:string, stderr:string, status:number}}
 */
export function git(root, args, opts = {}) {
  try {
    const stdout = execFileSync('git', args, {
      cwd: root,
      input: opts.input,
      encoding: 'utf8',
      maxBuffer: 64 * 1024 * 1024,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    return { stdout, stderr: '', status: 0 };
  } catch (e) {
    return {
      stdout: e.stdout != null ? String(e.stdout) : '',
      stderr: e.stderr != null ? String(e.stderr) : '',
      status: typeof e.status === 'number' ? e.status : 1,
    };
  }
}
