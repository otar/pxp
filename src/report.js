// report.js — terraform-style terminal output + machine-readable JSON packet.
import { writeFileSync } from 'node:fs';
import { colors } from './colors.js';

/**
 * Render a human report for `plan` / `apply`.
 * @param {object} report  the object returned by executor.execute()
 * @param {{color?:boolean}} [opts]
 * @returns {string}
 */
export function renderTerminal(report, opts = {}) {
  const c = colors(opts.color !== false);
  const out = [];
  out.push(c.bold(c.cyan(`PXP: ${report.title || 'untitled'}`)));
  out.push('');

  const failedIdx = report.result === 'error' ? report.failedOp?.op?.index ?? -1 : null;

  // preflight / no-op errors have no per-op results
  if (report.results.length === 0 && report.result === 'error') {
    out.push(`  ${c.red('✗')} ${report.failedOp.code}: ${report.failedOp.message}`);
    out.push('');
    out.push(c.red('Failed before any changes were made.'));
    return out.join('\n') + '\n';
  }

  for (const r of report.results) {
    if (report.result === 'error' && r.op?.index === failedIdx) {
      out.push(...failureLines(report.failedOp, c));
      break; // stop at the first failure
    }
    out.push(opLine(r, c, report));
  }

  out.push('');
  out.push(...footer(report, c));
  return out.join('\n') + '\n';
}

function opLine(r, c, report) {
  const applied = report.result === 'applied';
  switch (r.type) {
    case 'create':
      return `  ${c.green('+')} create ${r.path}`;
    case 'edit':
      return r.fuzz
        ? `  ${c.yellow('≈')} edit   ${r.path} ${c.dim('(1 match, indent fuzz)')}`
        : `  ${c.yellow('~')} edit   ${r.path} ${c.dim('(1 match)')}`;
    case 'delete':
      return `  ${c.red('-')} delete ${r.path}`;
    case 'rename':
      return `  ${c.cyan('»')} rename ${r.from} ${c.dim('»')} ${r.to}`;
    case 'run': {
      const label = r.label || r.command.split('\n')[0];
      const suffix = applied ? c.dim(' (ran)') : c.dim(' (deferred)');
      return `  ${c.blue('▶')} run    ${label}${suffix}`;
    }
    default:
      return `  ? ${r.type}`;
  }
}

function failureLines(f, c) {
  const path = f.path || f.op?.path || f.op?.from || '';
  const lines = [`  ${c.red('✗')} ${f.type || 'op'} ${path} ${c.red(`— ${f.code}`)}`];
  if (f.message) lines.push(`      ${c.dim(f.message)}`);
  if (f.code === 'E_AMBIGUOUS_MATCH' && f.locations) {
    lines.push(`      ${c.dim(`matches at lines ${f.locations.join(', ')} — expand SEARCH to disambiguate`)}`);
  }
  if (f.code === 'E_NO_MATCH' && f.nearestMiss) {
    const nm = f.nearestMiss;
    lines.push(`      ${c.dim(`${nm.hint}; nearest window at line ${nm.fileStartLine}:`)}`);
    for (const ln of nm.context) {
      const mark = nm.firstDivergence && ln.line === nm.firstDivergence.fileLine ? c.red(' >') : '  ';
      lines.push(`     ${mark} ${c.dim(String(ln.line).padStart(4))}  ${ln.text}`);
    }
  }
  return lines;
}

function footer(report, c) {
  const s = summarize(report.results);
  const lines = [];
  if (report.result === 'error') {
    const idx = report.failedOp?.op?.index;
    lines.push(c.red(`Failed${idx != null ? ` at op ${idx}` : ''} (${report.failedOp.code}).`));
    if (report.phase === 'apply') {
      lines.push(report.rolledBack ? c.dim('Working tree rolled back to the pre-apply state.') : c.dim('Working tree left as-is (--keep-on-fail).'));
    } else {
      lines.push(c.dim('No changes were made.'));
    }
    if (report.runSideEffects?.length) {
      lines.push(c.yellow(`Note: ${report.runSideEffects.length} run op(s) already executed; their side effects outside the repo were not rolled back.`));
    }
    return lines;
  }

  lines.push(summaryLine(s, c));

  if (report.result === 'planned') {
    if (report.runOps?.length) {
      lines.push('');
      lines.push(c.yellow('┌─ run steps ' + (report.allowRun ? '' : '(require --allow-run to apply) ') + '─────────'));
      for (const op of report.runOps) {
        for (const cmd of op.command) lines.push(c.yellow('│ ') + cmd);
      }
      lines.push(c.yellow('└──────────────────────────────────────────'));
    }
    lines.push('');
    lines.push(c.dim('Dry run only — no files were changed.'));
  } else if (report.result === 'applied') {
    if (report.committed && report.commit?.sha) {
      lines.push(c.green(`Committed ${report.commit.sha.slice(0, 7)}  ${report.commit.subject}`));
    } else {
      lines.push(c.dim('Changes staged (--no-commit); nothing committed.'));
    }
  }
  return lines;
}

function summaryLine(s, c) {
  const parts = [`${s.create} to create`, `${s.edit} to change` + (s.fuzz ? ` (${s.fuzz} fuzzy)` : '')];
  if (s.delete) parts.push(`${s.delete} to delete`);
  if (s.rename) parts.push(`${s.rename} to rename`);
  if (s.run) parts.push(`${s.run} to run`);
  return c.bold('Plan: ') + parts.join(', ');
}

function summarize(results) {
  const s = { create: 0, edit: 0, delete: 0, rename: 0, run: 0, fuzz: 0 };
  for (const r of results) {
    if (r.status !== 'ok') continue;
    if (s[r.type] != null) s[r.type]++;
    if (r.type === 'edit' && r.fuzz) s.fuzz++;
  }
  return s;
}

/**
 * Build the machine-readable JSON packet. On failure this is the "repair packet"
 * designed to be pasted back into a planning model (~2K token cap).
 * @param {object} report
 * @returns {object}
 */
export function renderJson(report) {
  if (report.result === 'error') {
    return {
      pxp: 2,
      result: 'failed',
      changeset_sha256: report.sha256,
      failed_op: failedOpPacket(report.failedOp),
      completed_ops: report.completed || [],
      rolled_back: !!report.rolledBack,
      run_side_effects: report.runSideEffects || [],
      instruction: 'Emit a corrected changeset containing only the failed op(s), using the context above as ground truth.',
    };
  }
  return {
    pxp: 2,
    result: report.result,
    changeset_sha256: report.sha256,
    summary: summarize(report.results),
    results: report.results
      .filter((r) => r.status === 'ok')
      .map((r) => ({ op: r.op.index, type: r.type, path: r.path ?? r.to, ...(r.type === 'edit' ? { pass: r.pass, fuzz: r.fuzz } : {}) })),
    ...(report.result === 'applied' ? { committed: !!report.committed, commit: report.commit?.sha ?? null } : {}),
  };
}

function failedOpPacket(f) {
  const op = f.op || {};
  const packet = {
    index: op.index ?? null,
    type: f.type || op.type || null,
    path: f.path || op.path || op.from || null,
    error: f.code,
  };
  if (f.message) packet.message = f.message;
  if (op.search?.length) {
    packet.search_first_line = op.search[0];
    packet.search_preview = op.search.length <= 6 ? op.search : [...op.search.slice(0, 3), '…', ...op.search.slice(-3)];
  }
  if (f.code === 'E_AMBIGUOUS_MATCH' && f.locations) {
    packet.match_lines = f.locations;
    packet.hint = 'expand SEARCH with more surrounding lines to disambiguate';
  }
  if (f.code === 'E_NO_MATCH' && f.nearestMiss) {
    const nm = f.nearestMiss;
    packet.nearest_miss = {
      line: nm.fileStartLine,
      context: nm.context.map((x) => ({ line: x.line, text: x.text })),
      first_divergence: nm.firstDivergence,
      hint: nm.hint,
    };
  }
  return packet;
}

/** @param {string} path @param {object} obj */
export function writeJson(path, obj) {
  writeFileSync(path, JSON.stringify(obj, null, 2) + '\n');
}
