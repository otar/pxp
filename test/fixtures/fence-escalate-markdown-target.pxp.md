# PXP Changeset

pxp: 2
title: Edit a Markdown file that contains fenced blocks
intent: SEARCH/REPLACE payloads contain a 4-backtick fence, so the carrier escalates to 5.

## Op 1: edit — README.md

SEARCH
`````
Example:

````
outer code block
````
`````
REPLACE
`````
Example (updated):

````
outer code block v2
````
`````
