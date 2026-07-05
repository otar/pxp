# PXP Changeset

pxp: 2
title: Payload with literal keyword lines
intent: Lines reading SEARCH, REPLACE, and a fake op header must be inert inside a fence.

## Op 1: create — weird.txt

````
SEARCH
REPLACE
## Op 9: edit — nope.js
expect_exit: 7
plain trailing text
````
