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
│   └── *_pipeline_test.go  # 28 test files
├── e2e/                    # E2E tests — requires kind cluster
│   ├── smoke_test.go
│   ├── optimizerconfig_test.go
│   ├── controller_recovery_test.go
│   ├── dryrun_test.go
│   └── namespace_isolation_test.go
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
# Unit tests only (fast, ~5s)
go test ./unit/... -v

# Integration tests
go test ./integration/... -v

# All tests with coverage
go test ./unit/... ./integration/... \
  -coverpkg=intelligent-cluster-optimizer/pkg/... \
  -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# E2E tests (requires kind cluster — see scripts/setup-kind.sh)
kind export kubeconfig --name optimizer-test --kubeconfig /tmp/kind-optimizer-test.yaml
KUBECONFIG=/tmp/kind-optimizer-test.yaml go test ./e2e/... -v -timeout 5m
```

## Current Test Status

### Coverage by Layer

| Layer       | Test Files | Test Cases | Coverage of `pkg/` |
|-------------|:----------:|:----------:|:------------------:|
| Unit        | 9          | 439        | ~45%               |
| Integration | 28         | 131        | ~81.8%             |
| E2E         | 5          | 11         | N/A (cluster)      |
| **Total**   | **42**     | **581**    | **81.8%**          |

> Target: **80%** ✅ — achieved.

### Coverage by Package

| Package               | Coverage | Status |
|-----------------------|:--------:|:------:|
| `pkg/pareto`          | high     | 🟢     |
| `pkg/leakdetector`    | high     | 🟢     |
| `pkg/recommendation`  | high     | 🟢     |
| `pkg/policy`          | high     | 🟢     |
| `pkg/prediction`      | high     | 🟢     |
| `pkg/safety`          | high     | 🟢     |
| `pkg/sla`             | high     | 🟢     |
| `pkg/trends`          | high     | 🟢     |
| `pkg/storage`         | high     | 🟢     |
| `pkg/cost`            | high     | 🟢     |
| `pkg/apis`            | E2E only | 🔵     |

> `pkg/apis` contains the Kubernetes CRD client — exercised by E2E tests only.

### What Is Tested

#### Unit & Integration (`pkg/`)

| Package | What is covered |
|---------|----------------|
| `pkg/safety` | CircuitBreaker open/close/half-open, HPA conflict detection, PDB safety check, OOM detection, percent-based disruption budgets |
| `pkg/pareto` | Solution set generation (≥6 solutions), Pareto frontier, crowding distance, dominance, Summary/ObjectiveSummary |
| `pkg/recommendation` | P95/P99 percentile accuracy (±1%), over/under-provisioning detection, confidence scoring, pricing model |
| `pkg/leakdetector` | Linear leak detection, GC sawtooth false-positive, severity escalation, FormatAnalysisSummary, ShouldPreventScaling |
| `pkg/policy` | Allow/deny/skip/require-approval actions, priority ordering, disabled rules, latency <50ms |
| `pkg/prediction` | Holt-Winters MAPE <15%, confidence intervals, seasonal forecasting, damped trend, decomposition |
| `pkg/sla` | Latency/error-rate/availability violations, add/remove SLA, GenerateChart, DetectOutliers |
| `pkg/trends` | Growth pattern detection (linear, stable, exponential, logarithmic, volatile), ExportHTML, ExportCSV |
| `pkg/storage` | Metric ingestion, retention cleanup, pipeline integration |
| `pkg/apis` | DeepCopy, AddToScheme, Resource — covered via unit tests and E2E CRD lifecycle |

#### E2E (real kind cluster)

| Test | What is verified |
|------|-----------------|
| `TestSmoke_ControllerDeploymentExists` | Controller Deployment exists in cluster |
| `TestSmoke_ControllerStartsWithin30s` | Controller reaches Ready within 30 seconds |
| `TestSmoke_ControllerPodIsRunning` | At least one pod in Running phase |
| `TestSmoke_MetricsEndpointReachable` | Metrics Service is present |
| `TestE2E_OptimizerConfigLifecycle` | Full CRD CRUD: Create → List → Get → Watch → Update → Delete |
| `TestE2E_ControllerRecovery` | Pod deletion triggers restart; new pod acquires leader lease |
| `TestE2E_DryRunMode` | `dryRun:true` — controller does not modify workload resources |
| `TestE2E_NamespaceIsolation` | Controller only acts on namespaces listed in `targetNamespaces` |
| `TestE2E_OverProvisionedDeploymentIsDetected` | Controller stays alive when processing over-provisioned workload |
| `TestE2E_RollbackRestoresWith60s` | Resource rollback completes within 60 seconds |

## CI Pipeline

```
push / pull_request / nightly
         │
         ├── unit-tests        go test ./unit/...        (~30s)
         ├── integration       go test ./integration/... (~2m)
         ├── e2e               kind cluster + all e2e tests (~5m)
         └── coverage-report   merged coverage ≥ 80%
```

CI checks out both repos as siblings under `$GITHUB_WORKSPACE`, which satisfies
the `replace` directive (`../intelligent-cluster-optimizer`) automatically.
