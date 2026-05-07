# Argo Rollouts — Agent Guidelines

Kubernetes controller and CRDs for progressive delivery (canary, blue-green, analysis, experiments).

## Build and Test

```bash
# Build the controller binary
make controller

# Run unit tests (requires kustomize installed locally)
make test

# Run a specific unit test
go test ./rollout/... -run TestCanaryRollout -count=1

# Lint
make lint

# Code generation after changing types or APIs
make codegen

# Run the controller locally (connects to current kubeconfig cluster)
go run ./cmd/rollouts-controller/main.go
```

### E2E Tests

```bash
# Prepare cluster
k3d cluster create
kubectl create ns argo-rollouts
kubectl apply -k manifests/crds
kubectl apply -f test/e2e/crds

# Start controller for E2E
make start-e2e

# Run all E2E tests
make test-e2e

# Run a specific E2E suite/test
E2E_TEST_OPTIONS="-run 'TestCanarySuite' -testify.m 'TestCanaryScaleDownOnAbortNoTrafficRouting'" make test-e2e
```

## Architecture

The project follows the Kubernetes controller pattern with workqueue-based reconciliation.

### Key directories

| Directory | Purpose |
|-----------|---------|
| `pkg/apis/rollouts/v1alpha1/` | CRD type definitions (`types.go`, `analysis_types.go`, `experiment_types.go`) — run `make codegen` after changes |
| `rollout/` | Rollout controller reconciliation logic (canary, bluegreen, sync, traffic routing) |
| `analysis/` | AnalysisRun controller — queries metric providers, manages analysis lifecycle |
| `experiments/` | Experiment controller — manages temporary ReplicaSets for A/B testing |
| `controller/` | Top-level `Manager` that wires and starts all sub-controllers |
| `cmd/rollouts-controller/` | Controller binary entrypoint |
| `cmd/kubectl-argo-rollouts/` | CLI plugin (`kubectl argo rollouts`) entrypoint |
| `server/` | gRPC + HTTP API server for the dashboard |
| `pkg/client/` | Generated clientset, informers, listers (do not edit manually) |
| `utils/` | Shared utility packages (replicaset, analysis, conditions, logging, etc.) |
| `metricproviders/` | Metric provider implementations (Prometheus, Datadog, Wavefront, etc.) |
| `rollout/trafficrouting/` | Traffic routing integrations (Istio, ALB, Nginx, SMI, Ambassador, etc.) |
| `rolloutplugin/` | RolloutPlugin CRD controller for plugin-based rollout strategies |
| `ingress/` | Ingress controller for managing ingress resources during rollouts |
| `service/` | Service controller for managing active/preview services |
| `test/e2e/` | End-to-end test suites (use build tags `e2e`) |
| `ui/` | React dashboard (yarn-based) |
| `manifests/` | Kubernetes manifests and CRD definitions |
| `hack/` | Code generation and installer scripts |

### Core CRD types (in `pkg/apis/rollouts/v1alpha1/`)

- **Rollout** — primary resource, replaces Deployment for progressive delivery
- **AnalysisTemplate / ClusterAnalysisTemplate** — defines metric queries for analysis
- **AnalysisRun** — instance of an analysis execution attached to a rollout
- **Experiment** — manages temporary ReplicaSets for traffic experiments

### Controller pattern

Each sub-controller (`rollout/`, `analysis/`, `experiments/`) follows the same pattern:
1. `Controller` struct with informers, listers, and workqueue
2. `NewController()` constructor wiring event handlers
3. `Run()` launching worker goroutines calling `controllerutil.RunWorker`
4. `syncHandler()` doing the actual reconciliation for a single resource key

The top-level `controller.Manager` creates and starts all sub-controllers.

## Conventions

- **Go modules with vendoring**: dependencies are vendored (`vendor/`). Run `go mod tidy && go mod vendor` when changing dependencies.
- **Code generation**: CRD types, protobuf, clientset, informers, listers, deepcopy, and OpenAPI specs are all generated. After changing `pkg/apis/rollouts/v1alpha1/types.go`, always run `make codegen`.
- **Logging**: use `logutil.WithRollout(ro)`, `logutil.WithAnalysisRun(ar)`, etc. from `utils/log` for structured context logging with logrus.
- **Event recording**: use the `record.EventRecorder` interface (not the raw k8s recorder) for Kubernetes events.
- **Test style**: unit tests use `testify/assert`, E2E tests use `testify/suite` with build tag `e2e`. Mocks are generated via `hack/update-mocks.sh` (mockery/gomock).
- **Instance ID**: controllers use `LabelKeyControllerInstanceID` for segregation between multiple controller instances.
- **Time utilities**: use `timeutil.Now()` from `utils/time` instead of `time.Now()` to enable deterministic testing.

## Documentation

- [Architecture overview](docs/architecture.md)
- [Contributing guide](docs/CONTRIBUTING.md)
- [Full documentation](https://argo-rollouts.readthedocs.io/en/stable/)
