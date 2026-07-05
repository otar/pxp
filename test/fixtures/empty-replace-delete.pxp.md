# PXP Changeset

pxp: 2
title: Delete a region via empty REPLACE
intent: An empty REPLACE block removes the matched region entirely.

## Op 1: edit — src/a.js

SEARCH
````
// BEGIN deprecated
const legacy = true;
// END deprecated
````
REPLACE
````
````
