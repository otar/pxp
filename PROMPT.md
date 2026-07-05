# PXP Changeset — how to emit one

When asked to implement a change, do not edit files directly. Emit a **single
PXP changeset** in the fenced-Markdown format below. A deterministic executor
(`pxp apply`) applies it with zero further model calls, so it must be exact.

## Format by example

````md
# PXP Changeset

pxp: 2
title: Add a request-rate limit
intent: Cap API requests per client using a shared limiter.

## Op 1: create — src/config/limits.js

````
export const MAX_PER_MINUTE = 100;
````

## Op 2: edit — src/middleware/index.js

SEARCH
````
import { auth } from './auth.js';

export { auth };
````
REPLACE
````
import rateLimit from 'express-rate-limit';

import { auth } from './auth.js';
import { MAX_PER_MINUTE } from '../config/limits.js';

export const limiter = rateLimit({ windowMs: 60_000, max: MAX_PER_MINUTE });
export { auth };
````

## Op 3: edit — src/routes/api.js

SEARCH
````
const router = express.Router();
````
REPLACE
````
const router = express.Router();
router.use(limiter);
````

## Op 4: run — verify

````
npm test
````
expect_exit: 0
````

Op types: `edit` (SEARCH/REPLACE), `create` (full file), `delete — path`,
`rename — old -> new`, `run` (shell command, optional `expect_exit: N`).

## Five hard rules

1. **Copy SEARCH verbatim from the file as it exists now** — never reconstruct
   it from memory. Whitespace and punctuation must match. (Trailing whitespace
   and CRLF are normalized for you; nothing else is.)
2. **Include enough surrounding lines that SEARCH is unique in the file.** If a
   block appears more than once, the apply fails with `E_AMBIGUOUS_MATCH`. Aim
   for ≥ 3 lines.
3. **Fence length must exceed the longest backtick run inside the payload,
   minimum 4.** If your code contains a triple backtick, fence it with four; if
   it contains four, use five.
4. **One SEARCH/REPLACE pair per `edit` op.** Never batch distant edits into one
   block — split them into separate ops, each with its own local context.
5. **Put `run` verification steps last.** File ops are dry-run-verified before
   anything is written; commands are not.

## Adversarial example — editing a file that contains fenced code

The payload has its own ``` ``` ``` block, so the carrier fence must be longer
(here, five backticks):

````md
## Op 1: edit — README.md

SEARCH
`````
## Usage

```
old command
```
`````
REPLACE
`````
## Usage

```
new command
```
`````
````

If SEARCH cannot be found, `pxp` fails loudly and returns a repair packet with
the file's actual surrounding lines — emit a corrected changeset using that as
ground truth. It never guesses.
