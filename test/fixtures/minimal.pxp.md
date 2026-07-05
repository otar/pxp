# PXP Changeset

pxp: 2
title: Minimal single edit
intent: Demonstrate the smallest useful changeset.

## Op 1: edit — src/app.js

Bump the middle constant.

SEARCH
````
const x = 1;
const y = 2;
const z = 3;
````
REPLACE
````
const x = 1;
const y = 20;
const z = 3;
````
