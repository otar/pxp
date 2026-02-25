#!/bin/bash
# Test suite for pxp.sh (PXP executor)
# Usage: ./test_pxp.sh [path/to/pxp.sh]

PXP="${1:-./pxp.sh}"
PASS=0
FAIL=0
TOTAL=0
FAILURES=""

# --- Helpers ---
setup_repo() {
  local dir
  dir=$(mktemp -d)
  cd "$dir" || exit 1
  git init -q .
  git config user.email "test@test.com"
  git config user.name "Test"
  mkdir -p src config
  printf 'export function formatDate(date) {\n  return date.toISOString();\n}\n' > src/utils.js
  printf '{}\n' > config/app.json
  git add . && git commit -q -m "init"
  printf '%s' "$dir"
}

teardown_repo() {
  cd /tmp || true
  rm -rf "$1"
}

run_pxp() {
  local yml="$1"
  printf '%s' "$yml" > changeset.yml
  bash "$PXP" changeset.yml 2>&1
}

assert_exit() {
  local expected="$1" actual="$2" label="$3"
  TOTAL=$((TOTAL + 1))
  if [[ "$actual" -eq "$expected" ]]; then
    PASS=$((PASS + 1))
    printf "  \033[0;32m✓\033[0m %s\n" "$label"
  else
    FAIL=$((FAIL + 1))
    FAILURES="${FAILURES}\n  ✗ ${label} (expected exit $expected, got $actual)"
    printf "  \033[0;31m✗\033[0m %s (expected exit %d, got %d)\n" "$label" "$expected" "$actual"
  fi
}

assert_file_contains() {
  local file="$1" expected="$2" label="$3"
  TOTAL=$((TOTAL + 1))
  if grep -qF "$expected" "$file" 2>/dev/null; then
    PASS=$((PASS + 1))
    printf "  \033[0;32m✓\033[0m %s\n" "$label"
  else
    FAIL=$((FAIL + 1))
    FAILURES="${FAILURES}\n  ✗ ${label} ('$expected' not in $file)"
    printf "  \033[0;31m✗\033[0m %s ('%s' not found in %s)\n" "$label" "$expected" "$file"
  fi
}

assert_output_contains() {
  local output="$1" expected="$2" label="$3"
  TOTAL=$((TOTAL + 1))
  if printf '%s' "$output" | grep -qF "$expected"; then
    PASS=$((PASS + 1))
    printf "  \033[0;32m✓\033[0m %s\n" "$label"
  else
    FAIL=$((FAIL + 1))
    FAILURES="${FAILURES}\n  ✗ ${label} ('$expected' not in output)"
    printf "  \033[0;31m✗\033[0m %s ('%s' not in output)\n" "$label" "$expected"
  fi
}

assert_output_not_contains() {
  local output="$1" unexpected="$2" label="$3"
  TOTAL=$((TOTAL + 1))
  if ! printf '%s' "$output" | grep -qF "$unexpected"; then
    PASS=$((PASS + 1))
    printf "  \033[0;32m✓\033[0m %s\n" "$label"
  else
    FAIL=$((FAIL + 1))
    FAILURES="${FAILURES}\n  ✗ ${label} ('$unexpected' unexpectedly in output)"
    printf "  \033[0;31m✗\033[0m %s ('%s' unexpectedly in output)\n" "$label" "$unexpected"
  fi
}

# --- Tests ---

printf "\033[1m[Test 1] Single run step\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: echo test
steps:
  - run: echo hello')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "hello" "command output visible"
assert_output_contains "$output" "All 1 steps" "reports 1 step"
teardown_repo "$REPO"

printf "\033[1m[Test 2] Single patch step\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: patch test
steps:
  - patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "file patched"
assert_output_contains "$output" "Done" "reports done"
teardown_repo "$REPO"

printf "\033[1m[Test 3] Multi-file patch\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: multi-file
steps:
  - patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }
      --- a/config/app.json
      +++ b/config/app.json
      @@ -1 +1,3 @@
      -{}
      +{
      +  "debug": true
      +}')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "utils.js patched"
assert_file_contains "config/app.json" '"debug": true' "app.json patched"
teardown_repo "$REPO"

printf "\033[1m[Test 4] Mixed run + patch + run sequence\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: mixed
steps:
  - run: echo "step1"
  - patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }
  - run: echo "step3"')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "step1" "first run executed"
assert_output_contains "$output" "step3" "last run executed"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "patch applied between runs"
assert_output_contains "$output" "All 3 steps" "reports 3 steps"
teardown_repo "$REPO"

printf "\033[1m[Test 5] Bad patch stops execution\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: bad patch
steps:
  - run: echo "before"
  - patch: |
      --- a/src/nonexistent.js
      +++ b/src/nonexistent.js
      @@ -1,3 +1,4 @@
      +bad
       nope
  - run: echo "SHOULD_NOT_RUN"')
rc=$?
assert_exit 1 "$rc" "exits non-zero"
assert_output_contains "$output" "before" "steps before failure ran"
assert_output_contains "$output" "Patch failed" "shows error"
assert_output_not_contains "$output" "SHOULD_NOT_RUN" "subsequent steps skipped"
teardown_repo "$REPO"

printf "\033[1m[Test 6] Bad command stops execution\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: bad command
steps:
  - run: echo "before"
  - run: false
  - run: echo "SHOULD_NOT_RUN"')
rc=$?
assert_exit 1 "$rc" "exits non-zero"
assert_output_contains "$output" "before" "steps before failure ran"
assert_output_contains "$output" "Failed" "shows error"
assert_output_not_contains "$output" "SHOULD_NOT_RUN" "subsequent steps skipped"
teardown_repo "$REPO"

printf "\033[1m[Test 7] Missing file\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(bash "$PXP" nonexistent.yml 2>&1)
rc=$?
assert_exit 1 "$rc" "exits 1"
assert_output_contains "$output" "not found" "shows file not found"
teardown_repo "$REPO"

printf "\033[1m[Test 8] No arguments\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(bash "$PXP" 2>&1)
rc=$?
assert_exit 1 "$rc" "exits 1"
assert_output_contains "$output" "Usage" "shows usage"
teardown_repo "$REPO"

printf "\033[1m[Test 9] Changeset name displayed\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: My Custom Changeset
steps:
  - run: echo ok')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "My Custom Changeset" "displays changeset name"
teardown_repo "$REPO"

printf "\033[1m[Test 10] Changeset without name\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'steps:
  - run: echo ok')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_not_contains "$output" "Changeset" "no changeset header when unnamed"
teardown_repo "$REPO"

printf "\033[1m[Test 11] Patch preserves file content integrity\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: integrity check
steps:
  - patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_file_contains "src/utils.js" "return date.toISOString();" "original content preserved"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "new content added"
lc=$(wc -l < src/utils.js)
TOTAL=$((TOTAL + 1))
if [[ "$lc" -eq 5 ]]; then
  PASS=$((PASS + 1))
  printf "  \033[0;32m✓\033[0m correct line count (%d)\n" "$lc"
else
  FAIL=$((FAIL + 1))
  FAILURES="${FAILURES}\n  ✗ expected 5 lines, got $lc"
  printf "  \033[0;31m✗\033[0m expected 5 lines, got %d\n" "$lc"
fi
teardown_repo "$REPO"

printf "\033[1m[Test 12] Multiple consecutive run steps\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: multi run
steps:
  - run: echo "aaa"
  - run: echo "bbb"
  - run: echo "ccc"')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "aaa" "first run"
assert_output_contains "$output" "bbb" "second run"
assert_output_contains "$output" "ccc" "third run"
assert_output_contains "$output" "All 3 steps" "correct step count"
teardown_repo "$REPO"

printf "\033[1m[Test 13] Run step with special characters\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: special chars
steps:
  - run: echo "hello world & goodbye | pipe > /dev/null"')
rc=$?
assert_exit 0 "$rc" "exits 0"
teardown_repo "$REPO"

printf "\033[1m[Test 14] Step name on run step\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: step names
steps:
  - name: Install dependencies
    run: echo "installing"')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "Install dependencies" "step name displayed"
teardown_repo "$REPO"

printf "\033[1m[Test 15] Step name on patch step\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: patch with name
steps:
  - name: Apply refactor
    patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "Apply refactor" "patch step name displayed"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "patch applied correctly"
teardown_repo "$REPO"

printf "\033[1m[Test 16] Mixed named and unnamed steps\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: mixed names
steps:
  - name: Install deps
    run: echo "installing"
  - run: echo "unnamed_step"
  - name: Apply changes
    patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }
  - run: echo "final"')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "Install deps" "first step name shown"
assert_output_contains "$output" "unnamed_step" "unnamed step shows command"
assert_output_contains "$output" "Apply changes" "patch step name shown"
assert_output_contains "$output" "All 4 steps" "correct step count"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "patch applied"
teardown_repo "$REPO"

printf "\033[1m[Test 17] Full spec example\033[0m\n"
REPO=$(setup_repo)
cp "$PXP" "$REPO/"
cd "$REPO"
output=$(run_pxp 'name: Add lodash utility and enable debug mode

steps:
  - name: Install package
    run: echo "npm install lodash"
  - name: Apply changes
    patch: |
      --- a/src/utils.js
      +++ b/src/utils.js
      @@ -1,3 +1,5 @@
      +import _ from "lodash";
      +
       export function formatDate(date) {
         return date.toISOString();
       }
      --- a/config/app.json
      +++ b/config/app.json
      @@ -1 +1,3 @@
      -{}
      +{
      +  "debug": true
      +}
  - run: echo "npx some-tool generate"
  - name: Lint the JS files
    run: echo "npm run lint"')
rc=$?
assert_exit 0 "$rc" "exits 0"
assert_output_contains "$output" "Add lodash utility" "changeset name"
assert_output_contains "$output" "Install package" "step 1 name"
assert_output_contains "$output" "Apply changes" "step 2 name"
assert_output_contains "$output" "Lint the JS files" "step 4 name"
assert_output_contains "$output" "All 4 steps" "correct count"
assert_file_contains "src/utils.js" 'import _ from "lodash"' "utils.js patched"
assert_file_contains "config/app.json" '"debug": true' "app.json patched"
teardown_repo "$REPO"

# --- Summary ---
printf "\n\033[1m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"
if [[ $FAIL -eq 0 ]]; then
  printf "\033[1m\033[0;32m✓ All %d tests passed.\033[0m\n" "$TOTAL"
else
  printf "\033[1m\033[0;31m✗ %d/%d tests failed:\033[0m\n" "$FAIL" "$TOTAL"
  printf "$FAILURES\n"
fi
printf "\033[1m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"

exit "$FAIL"
