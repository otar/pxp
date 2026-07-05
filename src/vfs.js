// vfs.js — an in-memory overlay over the working tree.
//
// The dry-run and the state tracking both go through one overlay so that
// chained ops see each other (op 3's edit reads op 1's result) and the full
// changeset can be simulated without touching disk. Actual disk writes happen
// in the op apply() functions; the vfs is the simulation + bookkeeping layer.

import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

/**
 * @param {string} root repo root
 * @returns {Vfs}
 */
export function createVfs(root) {
  /** @type {Map<string, string|null>} overlay: text, or null = deleted */
  const overlay = new Map();
  const abs = (p) => join(root, p);

  return {
    /** @param {string} p @returns {string} */
    read(p) {
      if (overlay.has(p)) {
        const v = overlay.get(p);
        if (v === null) throw new Error(`vfs: ${p} was deleted`);
        return v;
      }
      return readFileSync(abs(p), 'utf8');
    },
    /** @param {string} p @returns {boolean} */
    exists(p) {
      if (overlay.has(p)) return overlay.get(p) !== null;
      return existsSync(abs(p));
    },
    /** @param {string} p @param {string} text */
    write(p, text) {
      overlay.set(p, text);
    },
    /** @param {string} p @param {string} text */
    create(p, text) {
      overlay.set(p, text);
    },
    /** @param {string} p */
    remove(p) {
      overlay.set(p, null);
    },
    /** @param {string} from @param {string} to */
    move(from, to) {
      const text = this.read(from);
      overlay.set(from, null);
      overlay.set(to, text);
    },
  };
}

/** @typedef {ReturnType<typeof createVfs>} Vfs */
