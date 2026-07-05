# PXP Changeset

pxp: 2
title: Tab-indented edit
intent: Tabs in SEARCH and REPLACE payloads must survive verbatim.

## Op 1: edit — src/tabbed.js

SEARCH
````
function f() {
	const a = 1;
	return a;
}
````
REPLACE
````
function f() {
	const a = 1;
	const b = 2;
	return a + b;
}
````
