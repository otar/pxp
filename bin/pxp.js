#!/usr/bin/env node
// pxp — Plan eXecution Protocol CLI.
//
// Commands: validate | plan | apply | prompt.
// Exit codes: 0 success · 1 changeset/apply failure · 2 usage error.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import process from 'node:process';

import { parse, ParseError } from '../src/parser.js';
import { validate } from '../src/validate.js';
import { colors } from '../src/colors.js';

const COMMANDS = new Set(['validate', 'plan', 'apply', 'prompt', 'help']);

function main(argv) {
  const { command, file, flags, error } = parseArgs(argv);
  if (error) return fail(2, error);

  switch (command) {
    case 'help':
      printUsage(process.stdout);
      return 0;
    case 'prompt':
      return cmdPrompt();
    case 'validate':
      return cmdValidate(file, flags);
    case 'plan':
    case 'apply':
      return cmdExecute(command, file, flags);
    default:
      printUsage(process.stderr);
      return 2;
  }
}

/**
 * @param {string[]} argv raw args after `node pxp.js`
 * @returns {{command?:string, file?:string, flags:object, error?:string}}
 */
function parseArgs(argv) {
  const flags = {
    strict: false,
    allowRun: false,
    keepOnFail: false,
    noCommit: false,
    color: process.stdout.isTTY === true,
    json: null,
  };
  let command;
  let file;

  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--version' || a === '-v') {
      process.stdout.write(`${pkgVersion()}\n`);
      process.exit(0);
    } else if (a === '--help' || a === '-h') {
      command = 'help';
    } else if (a === '--strict') flags.strict = true;
    else if (a === '--allow-run') flags.allowRun = true;
    else if (a === '--keep-on-fail') flags.keepOnFail = true;
    else if (a === '--no-commit') flags.noCommit = true;
    else if (a === '--no-color') flags.color = false;
    else if (a === '--json') {
      flags.json = argv[++i];
      if (flags.json == null) return { flags, error: '--json requires a path' };
    } else if (a.startsWith('--json=')) flags.json = a.slice('--json='.length);
    else if (a.startsWith('-')) return { flags, error: `unknown flag: ${a}` };
    else if (!command) {
      if (!COMMANDS.has(a)) return { flags, error: `unknown command: ${a}` };
      command = a;
    } else if (!file) file = a;
    else return { flags, error: `unexpected argument: ${a}` };
  }

  if (!command) command = 'help';
  return { command, file, flags };
}

function cmdValidate(file, flags) {
  if (!file) return fail(2, 'validate requires a changeset file');
  const text = readChangeset(file);
  if (text == null) return 1;

  let cs;
  try {
    cs = parse(text);
  } catch (e) {
    if (e instanceof ParseError) return fail(1, e.message);
    throw e;
  }

  const { ok, issues } = validate(cs);
  const c = colors(flags.color);
  for (const is of issues) {
    const tag = is.severity === 'error' ? c.red('error') : c.yellow('warn ');
    const where = is.op != null ? `op ${is.op}` : 'header';
    process.stdout.write(`  ${tag} ${c.dim(`[${is.code}]`)} ${where}: ${is.message}\n`);
  }
  if (ok) {
    const n = cs.ops.length;
    process.stdout.write(`${c.green('✓')} valid — ${n} op${n === 1 ? '' : 's'}, ${issues.length} warning(s)\n`);
    return 0;
  }
  process.stdout.write(`${c.red('✗')} invalid\n`);
  return 1;
}

async function cmdExecute(command, file, flags) {
  if (!file) return fail(2, `${command} requires a changeset file`);
  const text = readChangeset(file);
  if (text == null) return 1;

  let cs;
  try {
    cs = parse(text);
  } catch (e) {
    if (e instanceof ParseError) return fail(1, e.message);
    throw e;
  }
  const { ok, issues } = validate(cs);
  if (!ok) {
    const c = colors(flags.color);
    for (const is of issues.filter((x) => x.severity === 'error')) {
      process.stderr.write(`  ${c.red('error')} ${c.dim(`[${is.code}]`)} ${is.op != null ? `op ${is.op}` : 'header'}: ${is.message}\n`);
    }
    return 1;
  }

  const { execute } = await import('../src/executor.js');
  const { renderTerminal, renderJson, writeJson } = await import('../src/report.js');
  const report = execute(cs, { mode: command, cwd: process.cwd(), flags, changesetPath: file });

  process.stdout.write(renderTerminal(report, { color: flags.color }));
  if (flags.json) writeJson(flags.json, renderJson(report));
  else if (report.result === 'error' && command === 'apply') {
    process.stderr.write(`${JSON.stringify(renderJson(report), null, 2)}\n`);
  }
  return report.result === 'error' ? 1 : 0;
}

function cmdPrompt() {
  try {
    const p = fileURLToPath(new URL('../PROMPT.md', import.meta.url));
    process.stdout.write(readFileSync(p, 'utf8'));
    return 0;
  } catch {
    return fail(1, 'PROMPT.md not found in the package');
  }
}

// --- helpers ---

function readChangeset(file) {
  try {
    return readFileSync(file, 'utf8');
  } catch {
    fail(1, `cannot read changeset: ${file}`);
    return null;
  }
}

function pkgVersion() {
  try {
    const p = fileURLToPath(new URL('../package.json', import.meta.url));
    return JSON.parse(readFileSync(p, 'utf8')).version;
  } catch {
    return '0.0.0';
  }
}

function fail(code, message) {
  process.stderr.write(`pxp: ${message}\n`);
  return code;
}

function printUsage(stream) {
  stream.write(`pxp ${pkgVersion()} — Plan eXecution Protocol

Usage:
  pxp validate <changeset.pxp.md>   Parse + static checks only
  pxp plan     <changeset.pxp.md>   Dry-run against the working tree, no writes
  pxp apply    <changeset.pxp.md>   Apply atomically with git rollback on failure
  pxp prompt                        Print the planner prompt contract (PROMPT.md)

Flags (plan/apply):
  --strict         Byte-exact matching only (disable indent-normalized pass)
  --allow-run      Approve execution of run ops non-interactively
  --json <path>    Write the machine-readable report to <path>
  --keep-on-fail   Leave partial state on failure (default: rollback)
  --no-commit      Stage changes instead of committing (apply only)
  --no-color       Disable ANSI color
`);
}

const code = await main(process.argv.slice(2));
process.exit(code);
