// colors.js — minimal ANSI helpers; every function is a no-op when color is off.

/** @param {boolean} on @returns {Record<string,(s:any)=>string>} */
export function colors(on) {
  const wrap = (code) => (s) => (on ? `\x1b[${code}m${s}\x1b[0m` : String(s));
  return {
    red: wrap('31'),
    green: wrap('32'),
    yellow: wrap('33'),
    blue: wrap('34'),
    cyan: wrap('36'),
    dim: wrap('2'),
    bold: wrap('1'),
  };
}
