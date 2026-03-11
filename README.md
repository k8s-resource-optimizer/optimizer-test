# Optimizer Tests

Dedicated test repository for [intelligent-cluster-optimizer](https://github.com/k8s-resource-optimizer/intelligent-cluster-optimizer).
Tests are kept in a separate repo so the production codebase stays clean.

## Repository Structure

```
optimizer-test/
├── unit/                   # Unit tests — no external dependencies
│   ├── circuit_breaker_test.go
│   ├── hpa_pdb_test.go
│   ├── pareto_test.go
│   ├── recommendation_test.go
│   ├── leakdetector_test.go
│   ├── policy_test.go
│   ├── holtwinters_test.go
│   ├── sla_test.go
│   └── trends_test.go
├── integration/            # Integration tests — in-memory, no cluster
│   └── pipeline_test.go
├── e2e/                    # E2E tests — requires kind cluster
│   └── smoke_test.go
├── scripts/
│   ├── setup-kind.sh       # Spin up kind cluster + deploy controller
│   └── teardown-kind.sh    # Tear down kind cluster
└── .github/workflows/
    └── ci.yml              # CI pipeline
```

## Setup

```bash
git clone https://github.com/k8s-resource-optimizer/optimizer-test
cd optimizer-test
go mod download
```

`go.mod` contains a `replace` directive pointing to `../intelligent-cluster-optimizer`.
By default the main repo is expected as a sibling directory. If you cloned it elsewhere,
edit the one line in `go.mod`:

```
replace intelligent-cluster-optimizer => /your/path/to/intelligent-cluster-optimizer
```

> Note: `go.work` is not usable here because the main repo's module name
> (`intelligent-cluster-optimizer`) lacks a dot in the first path element,
> which Go's workspace tooling does not support.

## Running Tests

```bash
# Unit tests only (fast, ~0.5s)
go test ./unit/... -v

# Integration tests
go test ./integration/... -v

# All tests with coverage
go test ./unit/... ./integration/... \
  -coverpkg=intelligent-cluster-optimizer/pkg/... \
  -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# E2E tests (requires kind cluster — see scripts/setup-kind.sh)
KUBECONFIG=$(kind get kubeconfig --name optimizer-test) \
  go test ./e2e/... -v -timeout 5m
```

## Current Test Status

### Coverage by Layer

| Layer       | Test Files | Test Cases | Coverage of `pkg/` |
|-------------|:----------:|:----------:|:------------------:|
| Unit        | 9          | 88         | 36.5%              |
| Integration | 1          | 5          | 32.3%              |
| E2E         | 1          | 6          | N/A (cluster)      |
| **Total**   | **11**     | **99**     | **37.4%**          |

> Target: **80%** — additional tests are in progress.

### Coverage by Package

| Package           | Coverage | Status |
|-------------------|:--------:|:------:|
| `pkg/pareto`      | 61.9%    | 🟡     |
| `pkg/leakdetector`| 56.4%    | 🟡     |
| `pkg/recommendation`| 42.2% | 🟠     |
| `pkg/policy`      | 41.9%    | 🟠     |
| `pkg/prediction`  | 41.5%    | 🟠     |
| `pkg/cost`        | 30.0%    | 🔴     |
| `pkg/safety`      | 29.7%    | 🔴     |
| `pkg/sla`         | 25.6%    | 🔴     |
| `pkg/trends`      | 19.3%    | 🔴     |
| `pkg/storage`     | 17.4%    | 🔴     |
| `pkg/apis`        |  0.0%    | ⚪     |

> `pkg/apis` contains the Kubernetes CRD client — covered by E2E tests only.

### What Is Tested

| Package | What is covered |
|---------|----------------|
| `pkg/safety` | CircuitBreaker open/close/half-open, HPA conflict detection, PDB safety check |
| `pkg/pareto` | Solution set generation (≥6 solutions), Pareto frontier, crowding distance, dominance |
| `pkg/recommendation` | P95/P99 percentile accuracy (±1%), over/under-provisioning detection |
| `pkg/leakdetector` | Linear leak detection, GC sawtooth false-positive, severity escalation, 85% accuracy |
| `pkg/policy` | Allow/deny/skip/require-approval actions, priority ordering, disabled rules, latency <50ms |
| `pkg/prediction` | Holt-Winters MAPE <15%, confidence intervals, seasonal forecasting |
| `pkg/sla` | Latency/error-rate/availability violations, add/remove SLA, severity range |
| `pkg/trends` | Growth pattern detection (linear, stable, exponential, logarithmic, volatile) |
| `pkg/storage` | Metric ingestion, retention cleanup, pipeline integration |

## CI Pipeline

```
push / pull_request / nightly
         │
         ├── unit-tests        go test ./unit/...        (~30s)
         ├── integration       go test ./integration/... (~1m)
         ├── e2e               kind cluster + smoke test (~5m)
         └── coverage-report   merged coverage ≥ 80%
```

CI checks out both repos as siblings under `$GITHUB_WORKSPACE`, which satisfies
the `replace` directive (`../intelligent-cluster-optimizer`) automatically.
