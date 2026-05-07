# RolloutPlugin User Guide

This guide explains how to use the `RolloutPlugin` custom resource to perform controlled canary rollouts of StatefulSets. Rather than updating all pods at once, RolloutPlugin lets you gradually shift to a new version with pause gates, automated health checks, and manual controls.

---

## What is a RolloutPlugin?

A **RolloutPlugin** manages the update process of a StatefulSet. It gives you:

- **Step-by-step canary rollouts** — update a percentage of pods at each step
- **Pause gates** — hold at a step until you manually approve or a timer expires
- **Automated analysis** — run health checks inline or in the background and abort on failure
- **Manual controls** — pause, resume, promote full, abort, or restart at any time
- **Progress deadlines** — detect stalled rollouts and optionally abort automatically

---

## How it Works with a StatefulSet

StatefulSets use a `partition` value in `updateStrategy.rollingUpdate` to control which pods receive updates. RolloutPlugin adjusts this partition — lowering it step by step as canary steps succeed, gradually updating more pods.

**Important:** Set `partition` equal to `replicas` on your StatefulSet before creating the RolloutPlugin. This prevents Kubernetes from updating pods on its own.

---

## Complete Example

A 5-replica StatefulSet with a canary rollout: update 20%, wait for approval, update to 60%, soak for 5 minutes, then complete.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app-svc
spec:
  clusterIP: None
  selector:
    app: my-app
  ports:
  - port: 8080
    name: web

---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: my-app
spec:
  serviceName: my-app-svc
  replicas: 5
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: my-app
        image: my-registry/my-app:v1.0.0
        ports:
        - containerPort: 8080
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      partition: 5    # Must equal replicas

---
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: my-app-rollout
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: my-app
  plugin:
    name: statefulset
  strategy:
    canary:
      steps:
      - setWeight: 20
      - pause: {}
      - setWeight: 60
      - pause: {duration: 5m}
      - setWeight: 100
```

To trigger a rollout, update the container image on the StatefulSet. The RolloutPlugin detects the change and starts executing canary steps automatically.

---

## Spec Reference

### `spec.workloadRef` *(required)*

Points to the StatefulSet being managed.

| Field        | Description                                                              |
| ------------ | ------------------------------------------------------------------------ |
| `apiVersion` | `apps/v1`                                                                |
| `kind`       | `StatefulSet`                                                            |
| `name`       | Name of the StatefulSet                                                  |
| `namespace`  | Optional. Defaults to the RolloutPlugin's namespace.                     |

```yaml
workloadRef:
  apiVersion: apps/v1
  kind: StatefulSet
  name: my-app
```

---

### `spec.plugin` *(required)*

Specifies which plugin handles this workload type. The plugin is configured in the `argo-rollouts-config` ConfigMap under the `rolloutPlugins` key.

| Field  | Description                                      |
| ------ | ------------------------------------------------ |
| `name` | Plugin name. Use `statefulset` for StatefulSets. |

```yaml
plugin:
  name: statefulset
```

---

### `spec.strategy.canary` *(required)*

Defines the canary rollout steps and optional background analysis.

```yaml
strategy:
  canary:
    steps:
    - setWeight: 20
    - pause: {duration: 10m}
    - setWeight: 100
```

#### Step Types

**`setWeight`** — Updates a percentage of pods to the new version.

```yaml
- setWeight: 50    # 50% of pods get the new version
```

For a 4-replica StatefulSet: `25%` = 1 pod, `50%` = 2 pods, `75%` = 3 pods, `100%` = all 4.

---

**`pause`** — Holds the rollout at the current step.

```yaml
- pause: {}                  # Indefinite — requires manual promote
- pause: {duration: 5m}      # Auto-resumes after 5 minutes
- pause: {duration: 30s}     # Auto-resumes after 30 seconds
```

An empty `pause: {}` waits indefinitely until you manually promote or promote full.

---

**`analysis`** — Runs an AnalysisTemplate as a step. The rollout waits for the analysis to complete. On success it advances; on failure it aborts; on inconclusive it pauses.

```yaml
- analysis:
    templates:
    - templateName: my-smoke-test
    args:
    - name: service-url
      value: "http://my-app:8080/healthz"
```

You can override template arguments with the `args` field.

---

#### Background Analysis

Runs an AnalysisTemplate continuously throughout the entire rollout. If it fails at any point, the rollout is aborted. If it becomes inconclusive, the rollout is paused.

```yaml
strategy:
  canary:
    analysis:
      templates:
      - templateName: error-rate-check
    steps:
    - setWeight: 20
    - pause: {}
    - setWeight: 100
```

---

### `spec.paused` *(optional)*

Manually pauses the rollout at its current position. The rollout will not advance while this is `true`.

```yaml
spec:
  paused: true
```

Set to `false` or remove the field to resume. Note: using **Promote Full** while manually paused will automatically clear this field and complete the rollout.

---

### `spec.progressDeadlineSeconds` *(optional)*

Maximum time in seconds for a rollout to make progress before it is considered stalled. Defaults to **600** (10 minutes). Time spent paused does not count toward this deadline.

```yaml
spec:
  progressDeadlineSeconds: 300
```

---

### `spec.progressDeadlineAbort` *(optional)*

When `true`, automatically aborts the rollout if `progressDeadlineSeconds` is exceeded. When `false` (default), only a warning condition is recorded.

```yaml
spec:
  progressDeadlineSeconds: 120
  progressDeadlineAbort: true
```

---

## Controlling a Rollout

Actions are performed by patching the RolloutPlugin resource. If you use ArgoCD, these are available as UI actions. You can also use `kubectl patch`.

### Promote (Approve a Pause)

When paused at a step, clear the pause conditions to advance:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge --subresource=status \
  -p '{"status":{"pauseConditions":null}}'
```

### Promote Full

Skip all remaining steps and complete the rollout immediately. This bypasses pending pauses and analysis, and also clears manual pause (`spec.paused`) if set:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge --subresource=status \
  -p '{"status":{"promoteFull":true}}'
```

### Pause

Freeze the rollout at its current position:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge \
  -p '{"spec":{"paused":true}}'
```

### Resume

Resume a manually paused rollout:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge \
  -p '{"spec":{"paused":false}}'
```

### Abort

Stop the rollout and revert all pods to the previous version:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge --subresource=status \
  -p '{"status":{"abort":true}}'
```

### Restart

After an abort, restart the rollout from step 0 to try the same version again:

```bash
kubectl patch rolloutplugin my-app-rollout \
  --type=merge --subresource=status \
  -p '{"status":{"restart":true}}'
```

---

## Checking Rollout Status

```bash
kubectl get rolloutplugin my-app-rollout
```

```text
NAME             STATUS       PLUGIN        AGE
my-app-rollout   Progressing  statefulset   3m
```

For details including conditions and step index:

```bash
kubectl describe rolloutplugin my-app-rollout
```

### Status Phases

| Phase         | Meaning                                                                |
| ------------- | ---------------------------------------------------------------------- |
| `Healthy`     | No rollout in progress. All pods are on the current stable version.    |
| `Progressing` | A rollout is actively executing canary steps.                          |
| `Paused`      | The rollout is paused — at a pause step, manually paused, or inconclusive analysis. |
| `Degraded`    | The rollout was aborted (manually, by analysis failure, or by deadline timeout). |

---

## Common Patterns

### Gradual Rollout with Manual Gates

```yaml
strategy:
  canary:
    steps:
    - setWeight: 10
    - pause: {}
    - setWeight: 30
    - pause: {}
    - setWeight: 100
```

### Timed Soak Period

```yaml
strategy:
  canary:
    steps:
    - setWeight: 20
    - pause: {duration: 15m}
    - setWeight: 60
    - pause: {duration: 30m}
    - setWeight: 100
```

### Inline Analysis Gating

Run a health check between weight changes — the rollout only advances if analysis succeeds:

```yaml
strategy:
  canary:
    steps:
    - setWeight: 25
    - analysis:
        templates:
        - templateName: http-benchmark
    - setWeight: 75
    - analysis:
        templates:
        - templateName: http-benchmark
    - setWeight: 100
```

### Continuous Background Monitoring

A background analysis runs for the entire rollout duration. Any failure aborts immediately:

```yaml
strategy:
  canary:
    analysis:
      templates:
      - templateName: error-rate-check
    steps:
    - setWeight: 20
    - pause: {duration: 10m}
    - setWeight: 60
    - pause: {duration: 10m}
    - setWeight: 100
```

### Auto-Abort on Timeout

```yaml
spec:
  progressDeadlineSeconds: 600
  progressDeadlineAbort: true
  strategy:
    canary:
      steps:
      - setWeight: 50
      - pause: {}
      - setWeight: 100
```

---

## Tips and Gotchas

- **Set `partition` = `replicas` on your StatefulSet.** If partition is 0, Kubernetes may have already updated all pods before RolloutPlugin can take control.

- **Changing the image triggers a rollout automatically.** No separate action is needed — just update the StatefulSet's container image.

- **A new image mid-rollout resets to step 0.** The in-progress rollout is cancelled and a new one starts from the beginning with the latest image.

- **`pause: {}` requires manual action.** An indefinite pause will not time out. You must promote, promote full, or abort.

- **Promote Full clears manual pause.** If you have `spec.paused: true` and then promote full, the controller automatically clears the manual pause and completes the rollout.

- **Abort reverts all pods.** The partition is reset to block all new-version pods, reverting traffic to the previous stable version.

- **After an abort, you must `restart` to retry.** The controller will not automatically retry the same revision. This gives you time to investigate before trying again.

- **Analysis templates must have a finite `count`.** The RolloutPlugin validation requires that every metric in a referenced AnalysisTemplate has a defined `count` (either directly or via `effectiveCount`). A metric with only `interval` and no `count` will be rejected as "runs indefinitely".
