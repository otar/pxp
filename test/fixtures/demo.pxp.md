# PXP Changeset

pxp: 2
title: Add a request-rate limit
intent: Cap API requests per client using a shared limiter defined in one place.

## Op 1: create — src/config/limits.js

The single source of truth for the rate cap.

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

export default router;
````
REPLACE
````
const router = express.Router();
router.use(limiter);

export default router;
````
