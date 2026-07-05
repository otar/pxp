# PXP Changeset

pxp: 2
title: Header dash and keyword variants
intent: Em-dash, en-dash, and hyphen separators plus heading-prefixed SEARCH/REPLACE.

## Op 1: edit — src/a.js

### SEARCH
````
alpha
beta
gamma
````
### REPLACE
````
alpha
BETA
gamma
````

## Op 2: edit – src/b.js

SEARCH
````
one
two
three
````
REPLACE
````
one
TWO
three
````

## Op 3: edit - src/c.js

SEARCH
````
red
green
blue
````
REPLACE
````
red
GREEN
blue
````
