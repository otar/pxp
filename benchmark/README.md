# PXP benchmark framework

`pxp-bench` runs the same task from the same committed fixture in two modes:

1. `direct`: one normal coding-agent invocation edits and verifies the workspace.
2. `pxp`: one agent invocation may create only a closed PXP 1.1 plan; the harness hashes that plan, invokes `pxp run --expect-sha256`, and then runs the same independent verifier.

There are no retries or automatic follow-up model calls. A failed PXP run or final verifier marks `replanning_required: true`; the harness does not perform that replan.

## Build and run

Build the binaries and authenticate the agent CLI normally:

```sh
make build
```

Run the three included small, medium, and large offline fixtures with an exact model name when the agent supports it:

```sh
./bin/pxp-bench \
  --agent codex \
  --model MODEL_NAME \
  --agent-bin codex \
  --pxp ./bin/pxp \
  --suite benchmark/suite.json \
  --runs 3 \
  --output codex-results.json

./bin/pxp-bench \
  --agent claude \
  --model MODEL_NAME \
  --agent-bin claude \
  --pxp ./bin/pxp \
  --suite benchmark/suite.json \
  --runs 3 \
  --output claude-results.json
```

Use `--task slugify` for a single task and `--keep-workspaces` when debugging. Each strategy receives a fresh temporary Git repository initialized from its fixture. Raw prompts, agent JSONL, stderr, generated plans, PXP output, and verifier output are stored next to the report under `OUTPUT.artifacts/`.

The harness records the suite SHA-256, OS/architecture, requested model, agent version, and PXP version. Keep the suite, binaries, model, agent settings, account tier, region, and run count constant when comparing strategies. Run multiple repetitions because remote model latency and model behavior vary.

## Metrics and honesty

The JSON report distinguishes sources in each result's `measurements` map:

| Metric | Source |
|---|---|
| input/output/total tokens | Agent machine-readable output when present |
| model turns | Claude assistant events; unavailable for Codex because `codex exec --json` does not expose underlying model-call count |
| tool calls | Agent tool/item events |
| agent, planning, PXP, and wall-clock duration | Harness monotonic clock |
| success and first-pass success | Independent verifier after exactly one agent attempt |
| replanning required | PXP strategy failed before or during final verification |

Unavailable metrics are JSON `null` and labeled `unavailable`; the harness never estimates them. Cached-token fields remain in raw output but are not double-counted in `total_tokens`. `planning_duration_ms` applies only to the PXP strategy because direct-mode reasoning and implementation are not reliably separable. PXP execution duration excludes the agent planning invocation.

Aggregate by task, complexity, agent, and strategy to compare token totals, PXP execution latency, failure/replanning rates, and the complexity level where the measured tradeoff changes. Results are evidence for the tested fixture and configuration, not a general performance claim.

## Security and approval caveat

Benchmarks invoke external coding agents on trusted local fixtures. The Claude adapter uses `bypassPermissions` so a non-interactive direct run can use its normal tools; the Codex adapter uses a workspace-write sandbox. Run the harness only on trusted fixtures in an isolated environment and review the agent vendors' current automation guidance.

For measurement, the harness automatically hashes the generated PXP plan and immediately supplies that hash to PXP. This exercises the exact-byte enforcement but is not human approval. Production use must insert the manual review described in [the integration guide](../docs/integrations.md).
