# PXP Changeset

pxp: 2
title: Mixed ops across the repo
intent: Exercise create, edit, rename, delete, and run in one changeset.

## Op 1: create — src/limits.js

````
export const MAX = 100;
````

## Op 2: edit — src/order.js

SEARCH
````
export function total(items) {
  return items.length;
}
````
REPLACE
````
import { MAX } from './limits.js';

export function total(items) {
  return Math.min(items.length, MAX);
}
````

## Op 3: rename — src/old.js -> src/new.js

## Op 4: delete — src/legacy.js

## Op 5: run — verify

Run the test suite last, after all file ops.

````
npm test
````
expect_exit: 0
