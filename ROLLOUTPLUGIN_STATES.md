# RolloutPlugin CR - States and Transitions

## Overview
The RolloutPlugin CR manages progressive delivery rollouts for workloads (StatefulSets, Deployments, etc.). This document describes all possible states, transitions, and actions.

---

## State Diagram

```
                         ┌──────────────┐
                         │   Initial    │
                         │  (No Phase)  │
                         └──────┬───────┘
                                │
                                │ currentRevision != updatedRevision
                                │ (new revision detected)
                                ▼
                    ┌───────────────────────┐
         ┌─────────│     Progressing        │◄──────────────────┐
         │         │ (rolloutInProgress=T)  │                   │
         │         └───────────┬────────────┘                   │
         │                     │                                │
         │        ┌────────────┼──────────────┐                 │
         │        │            │              │                 │
         │        │ Step       │ BG/Step      │ All Steps       │
         │        │ Pause      │ Analysis     │ + Pods          │
         │        │            │ Inconclusive │ Converged       │
         │        ▼            ▼              ▼                 │
         │  ┌───────────┐ ┌──────────┐ ┌──────────────┐        │
         │  │  Paused   │ │  Paused  │ │  Successful  │        │
         │  │(Step/Man) │ │(Analy.)  │ │  (Healthy)   │        │
         │  └─────┬─────┘ └────┬─────┘ └──────┬───────┘        │
         │        │            │               │                │
         │        │ Duration   │               │ New Revision   │
         │        │ expires /  │               │                │
         │        │ User       │               └────────────────┘
         │        │ promotes   │
         │        │            │
         │        └─────┬──────┘
         │              │
         │              │ Resume / PromoteFull
         │              └────────────────────────────────┐
         │                                               │
         │  Manual Abort (status.abort=true)              │
         │  OR Timeout (progressDeadlineAbort=true)       │
         │  OR BG/Step Analysis Failed                    │
         │                                               │
         ▼                                               │
  ┌──────────────────────┐                               │
  │      Degraded        │                               │
  │   (Aborted)          │                               │
  │ aborted=true         │                               │
  │ rolloutInProgress=F  │                               │
  └──────────┬───────────┘                               │
             │                                           │
    ┌────────┼──────────┐                                │
    │        │          │                                │
    │Restart │ New Rev  │ (Same Rev)                     │
    │(status.│ deployed │ → stays Degraded               │
    │restart │          │                                │
    │=true)  │          │                                │
    ▼        ▼          │                                │
 Progressing Progressing│                                │
 (step 0,   (step 0,   │                                │
  same rev)  new rev,   │                                │
             abort      │                                │
             cleared)   │                                │
    │        │                                           │
    └────────┴───────────────────────────────────────────┘

 PromoteFull (from Paused or Progressing):
   → Clears pause state
   → Sets partition=0 (all pods)
   → stepIndex = stepCount (past last step)
   → Waits for convergence → Successful

 Manual Pause (spec.paused=true, from Progressing):
   → Paused (processing halted)
   → spec.paused=false → resumes from same step

 Plugin Error / Invalid Spec:
   → Failed (requires fix + new revision)
```

---

## States (Phases)

### 1. **Progressing**
**Description**: Rollout is actively executing canary steps (setWeight, pause, analysis)

**Characteristics**:
- `status.rolloutInProgress = true`
- `status.currentStepIndex` tracks progress through canary steps (0-based)
- `status.phase = "Progressing"`
- Active traffic shifting and pod updates happening

**Entry Conditions**:
- New revision detected (`currentRevision != updatedRevision`) when not in progress
- New revision detected mid-rollout (resets to step 0)
- Resume from Paused (step pause elapsed or user promoted)
- Restart from Degraded (`status.restart=true`)
- PromoteFull initiated (briefly Progressing while waiting for convergence)

**Exit Conditions**:
- Pause step encountered → **Paused**
- All steps complete + pods converged (`currentRevision == updatedRevision`) → **Successful** (Healthy)
- Manual abort (`status.abort=true`) → **Degraded**
- Timeout with `progressDeadlineAbort=true` → **Degraded**
- Background/step analysis failed → **Degraded** (auto-abort)
- Background/step analysis inconclusive → **Paused**
- Manual pause (`spec.paused=true`) → **Paused**
- Plugin error / invalid spec → **Failed**

---

### 2. **Paused**
**Description**: Rollout is temporarily halted, waiting for manual or automatic resume

**Types of Pauses**:
- **Step Pause (timed)**: Automatic pause defined in canary steps with `duration`
  - Resumes automatically when duration expires
- **Step Pause (indefinite)**: Pause step with no duration
  - Requires user to clear `pauseConditions` (via ArgoCD Lua resume action or kubectl)
- **Manual Pause**: User sets `spec.paused=true`
  - Takes priority; requires `spec.paused=false` to resume
- **Inconclusive Analysis Pause**: Background or step analysis returns inconclusive
  - Sets `PauseConditions` with reason `InconclusiveAnalysis`

**Characteristics**:
- `status.phase = "Paused"`
- `status.pauseStartTime` is set
- `status.controllerPause = true` (for step/analysis pauses)
- `status.pauseConditions` contains the pause reason(s)
- Pause time doesn't count toward `progressDeadlineSeconds`

**Pause/Resume Pattern (PauseConditions + ControllerPause)**:
1. Controller sets `PauseConditions` + `ControllerPause=true` when pausing
2. User clears `PauseConditions` to resume (via ArgoCD Lua action or kubectl patch)
3. Controller detects `ControllerPause=true && PauseConditions=nil` → step is complete, advance

**Exit Conditions**:
- Timed pause duration expires → **Progressing** (next step)
- User clears pauseConditions (promoted) → **Progressing** (next step)
- `spec.paused` set to `false` (manual resume) → **Progressing** (same step)
- PromoteFull → **Progressing** (skips to end, partition=0)
- Abort (`status.abort=true`) → **Degraded**

**Actions Available**:
- `Resume`: Clear pauseConditions → continue to next step
- `PromoteFull`: Skip all remaining steps → promote immediately
- `Abort`: Cancel rollout → **Degraded**

---

### 3. **Degraded (Aborted)**
**Description**: Rollout has been aborted, pods rolled back to previous revision

**Characteristics**:
- `status.aborted = true`
- `status.abortedRevision` = the revision that was aborted
- `status.rolloutInProgress = false`
- `status.phase = "Degraded"`
- Pods with new revision are deleted (gracefully, one at a time via `plugin.Abort()`)
- StatefulSet partition reset to previous state

**Entry Conditions**:
- Manual abort (`status.abort=true`) from Progressing or Paused
- Timeout with `progressDeadlineAbort=true`
- Background analysis failed/errored
- Step analysis failed/errored

**Exit Conditions**:
- **Restart same revision**: User sets `status.restart=true` → `plugin.Restart()` called → **Progressing** (step 0, same revision, `restartCount` incremented)
- **New revision deployed**: Different revision detected → abort state cleared, **Progressing** (step 0)
- **Same revision detected again**: Stays **Degraded** (no auto-restart)

**Actions Available**:
- `Restart` (`status.restart=true`): Retry the aborted revision from step 0
- Deploy new revision: Automatically clears abort and starts new rollout from step 0

---

### 4. **Successful (Healthy)**
**Description**: Rollout completed successfully, all pods updated and healthy

**Characteristics**:
- `status.rolloutInProgress = false`
- `status.currentRevision == status.updatedRevision` (pods converged)
- `status.phase = "Successful"` (transitions to `"Healthy"` on next reconcile)
- All replicas available and ready
- Completed condition set

**Entry Conditions**:
- All canary steps completed AND `currentRevision == updatedRevision`
- PromoteFull completed AND pods converged

**Exit Conditions**:
- New revision deployed → **Progressing** (step 0)

---

### 5. **Failed**
**Description**: Rollout encountered an unrecoverable error

**Common Causes**:
- Plugin not found / invalid spec → `InvalidSpec` condition
- Plugin execution errors (`SetWeight`, `VerifyWeight`, `Promote` failures)
- Invalid pause duration
- No strategy defined

**Characteristics**:
- `status.phase = "Failed"`
- `status.message` contains error details
- Appropriate condition set (`InvalidSpec`, etc.)

**Recovery**:
- Fix underlying issue (e.g., register plugin, fix spec)
- Deploy new revision to trigger a new rollout

---

## Actions and Triggers

### User-Initiated Actions

| Action | Field | Valid From State | Result |
|--------|-------|------------------|--------|
| **Abort** | `status.abort=true` | Progressing, Paused | → Degraded (calls `plugin.Abort()`, rolls back pods) |
| **Restart** | `status.restart=true` | Degraded (Aborted only) | → Progressing (calls `plugin.Restart()`, step 0, increments `restartCount`) |
| **Pause** | `spec.paused=true` | Progressing | → Paused (halts processing) |
| **Resume** | `spec.paused=false` | Paused (manual) | → Progressing (continues from current step) |
| **Promote** | Clear `pauseConditions` | Paused (step/analysis) | → Progressing (advances to next step) |
| **PromoteFull** | `status.promoteFull=true` | Paused, Progressing | → Progressing → Successful (sets partition=0, skips all steps) |

### Automatic Transitions

| Trigger | From State | To State | Condition |
|---------|-----------|----------|-----------|
| New Revision | Healthy/Successful | Progressing | `currentRevision != updatedRevision`, `!rolloutInProgress` |
| New Revision | Degraded | Progressing | Different revision than `abortedRevision` |
| New Revision (mid-rollout) | Progressing/Paused | Progressing (step 0) | `updatedRevision` changed while `rolloutInProgress=true` |
| Same Revision | Degraded | Degraded (no change) | `updatedRevision == abortedRevision` |
| Pause Step | Progressing | Paused | Step with `pause: {}` reached |
| Timed Pause Expires | Paused | Progressing | Pause duration elapsed |
| All Steps Done + Converged | Progressing | Successful | `currentStepIndex >= stepCount` AND `currentRevision == updatedRevision` |
| All Steps Done + Not Converged | Progressing | Progressing | `currentStepIndex >= stepCount` but pods still converging |
| Timeout + `progressDeadlineAbort` | Progressing/Paused | Degraded | `progressDeadlineSeconds` exceeded, auto-abort |
| Timeout (no abort) | Progressing | Progressing (with warning) | `progressDeadlineSeconds` exceeded, condition set but processing continues |
| BG/Step Analysis Failed | Progressing | Degraded | Analysis run failed/errored → auto-abort |
| BG/Step Analysis Inconclusive | Progressing | Paused | `PauseConditions` set with `InconclusiveAnalysis` |
| Plugin Error | Progressing | Failed | `SetWeight`/`VerifyWeight`/`Promote` error |

---

## State Fields in Status

### Key Status Fields

```yaml
status:
  # Phase tracking
  phase: "Progressing"                    # Current state: Progressing|Paused|Degraded|Successful|Healthy|Failed
  message: "Rollout is progressing"       # Human-readable details
  
  # Rollout progress
  rolloutInProgress: true                 # Is rollout active?
  currentStepIndex: 2                     # Current canary step (0-based)
  currentStepComplete: false              # Has current step finished?
  
  # Revision tracking
  currentRevision: "abc123"               # Current stable revision (from workload)
  updatedRevision: "def456"               # Target revision being rolled out (from workload)
  
  # Abort state
  aborted: false                          # Is rollout aborted?
  abortedRevision: ""                     # Which revision was aborted
  
  # Pause state (PauseConditions + ControllerPause pattern)
  controllerPause: false                  # Controller has paused the rollout
  pauseConditions:                        # List of pause reasons (cleared by user to resume)
  - reason: CanaryPauseStep
    startTime: "2026-03-08T12:00:00Z"
  pauseStartTime: "2026-03-08T12:00:00Z" # When pause started
  
  # Trigger fields (one-shot, cleared after processing)
  abort: false                            # Set to true to abort → cleared to false after processing
  restart: false                          # Set to true to restart aborted → cleared after processing
  promoteFull: false                      # Set to true to promote fully → cleared after processing
  
  # Restart tracking
  restartCount: 0                         # Number of restart attempts for current revision
  restartedAt: null                       # Timestamp of last restart
  
  # Resource status (from workload)
  replicas: 5
  availableReplicas: 5
  readyReplicas: 5
  updatedReplicas: 3
  
  # Analysis status (canary)
  canary:
    currentStepAnalysisRunStatus:          # Step-based analysis run status
      name: "..."
      status: "Running"
    currentBackgroundAnalysisRunStatus:    # Background analysis run status
      name: "..."
      status: "Running"
```

---

## State Transition Rules

### 1. Starting a Rollout
**Condition**: `currentRevision != updatedRevision AND !rolloutInProgress`

**Logic**:
```
IF status.aborted == true:
  IF updatedRevision == abortedRevision:
    → Stay in Degraded (don't auto-restart same aborted revision)
  ELSE:
    → Clear abort state, start new rollout from step 0 → Progressing
ELSE:
  → Start new rollout from step 0 → Progressing
  → Set rolloutInProgress=true, currentStepIndex=nil (initialized to 0 in processCanaryRollout)
  → Remove old Completed condition
  → Set Progressing condition
```

### 2. New Revision Mid-Rollout
**Condition**: `oldUpdatedRevision != "" AND oldUpdatedRevision != newUpdatedRevision`

**Logic**:
```
→ Reset currentStepIndex=nil (forces restart from step 0)
→ Clear pause state (pauseConditions, controllerPause, pauseStartTime)
→ Clear abort state if new revision differs from abortedRevision
→ Clear timeout condition
→ Reset restartCount=0
→ Set phase=Progressing
```

### 3. Abort Behavior
**Trigger**: `status.abort=true` (manual) OR timeout+progressDeadlineAbort OR analysis failure

**Immediate Effects**:
- Call `plugin.Abort()` to rollback workload (delete new pods gracefully)
- Set `aborted=true`, `abortedRevision=updatedRevision`
- Set `rolloutInProgress=false`
- Set `phase=Degraded`
- Clear `abort=false` (one-shot)

**Subsequent Reconciliations**:
- **Same revision**: Stays Degraded, blocks automatic restart
- **New revision**: Clears abort state, starts new rollout from step 0
- **Restart action**: User sets `status.restart=true` → calls `plugin.Restart()`, restarts from step 0

### 4. Restart Behavior
**Trigger**: `status.restart=true AND status.aborted=true`

**Effects**:
- Call `plugin.Restart()` to return workload to baseline
- Increment `restartCount`, set `restartedAt`
- Reset: `currentStepIndex=0`, `rolloutInProgress=true`, clear all pause state
- Clear: `aborted=false`, `abort=false`, `restart=false`
- Set `phase=Progressing`

### 5. Pause Behavior
**Types**:
- **Step Pause**: Automatic from canary strategy steps
  - Controller sets `PauseConditions` + `ControllerPause=true`
  - Timed: auto-resumes after `duration` expires
  - Indefinite: waits for user to clear `PauseConditions`
  
- **Manual Pause**: User sets `spec.paused=true`
  - Takes priority (checked before step processing)
  - Requires `spec.paused=false` to resume
  - PromoteFull bypasses manual pause

- **Inconclusive Analysis Pause**: Analysis returns inconclusive
  - Sets `PauseConditions` with `InconclusiveAnalysis` reason

### 6. Resume Behavior
**From Step Pause (user promote)**:
- User clears `PauseConditions` → controller detects `ControllerPause=true && PauseConditions=nil`
- Advances `currentStepIndex` to next step
- Clears all pause state

**From Timed Step Pause**:
- Duration elapsed → same as promote, advances to next step

**From Manual Pause**:
- User sets `spec.paused=false`
- Continues from current step (does not advance)

**PromoteFull**:
- Clears all pause state
- Calls `plugin.Promote()` (partition=0)
- Sets `currentStepIndex = stepCount` (past last step)
- Waits for pod convergence → Successful

### 7. Completion
**Condition**: `currentStepIndex >= stepCount`

**Logic**:
```
→ Call plugin.Promote() (set partition=0, idempotent)
→ IF currentRevision != updatedRevision:
    → Wait for pods to converge (requeue every 5s)
→ ELSE:
    → Set rolloutInProgress=false
    → Set phase=Successful
    → Remove Progressing condition, set Completed condition
```

---

## Examples

### Example 1: Normal Rollout Flow
```
Steps: [setWeight: 20, pause: 3s, setWeight: 40, pause: 30s, setWeight: 100]

1. Deploy new image → currentRevision != updatedRevision
   → Progressing (step 0: setWeight 20%, partition updated)
   
2. Weight verified → move to step 1
   → Paused (3s timed pause, PauseConditions set)
   
3. 3s elapses → pause duration expired
   → Progressing (step 2: setWeight 40%)
   
4. Weight verified → move to step 3
   → Paused (30s timed pause)
   
5. User promotes (clears pauseConditions) OR 30s elapses
   → Progressing (step 4: setWeight 100%)
   
6. All steps complete, partition=0, pods converge
   → Successful → Healthy
```

### Example 2: Abort and Restart
```
1. Rollout in progress
   → Progressing (step 2)
   
2. User sets status.abort=true
   → plugin.Abort() called (rollback pods)
   → Degraded (aborted=true, abortedRevision=v2)
   
3. Same revision detected
   → Stays Degraded (no auto-restart)
   
4. User sets status.restart=true
   → plugin.Restart() called, restartCount=1
   → Progressing (step 0, retry same revision)
   
5. Completes successfully
   → Successful → Healthy
```

### Example 3: Abort and New Revision
```
1. Rollout in progress (image v2)
   → Progressing (step 2)
   
2. User aborts
   → Degraded (abortedRevision=v2)
   
3. User deploys new image v3
   → updatedRevision changed, different from abortedRevision
   → Abort state cleared, currentStepIndex reset to 0
   → Progressing (step 0 of new rollout)
```

### Example 4: Manual Pause and Resume
```
1. Rollout in progress
   → Progressing (step 2)
   
2. User sets spec.paused=true
   → Paused (processing halted, pauseStartTime set)
   
3. User sets spec.paused=false
   → Progressing (continues from step 2, same position)
```

### Example 5: PromoteFull During Pause
```
1. Rollout paused at step 1
   → Paused
   
2. User sets status.promoteFull=true
   → Pause state cleared (takes priority over spec.paused)
   → plugin.Promote() called (partition=0)
   → currentStepIndex = stepCount
   → Progressing (waiting for convergence)
   
3. Pods converge
   → Successful → Healthy
```

### Example 6: New Revision Mid-Rollout
```
1. Rollout at step 3 (setWeight 60%)
   → Progressing
   
2. User updates StatefulSet spec (new image)
   → updatedRevision changes
   → currentStepIndex reset to nil (→ 0)
   → Pause state cleared
   → Progressing (restarts from step 0 with new revision)
```

### Example 7: Analysis Failure
```
1. Rollout at step 2 (analysis step)
   → Progressing (analysis running)
   
2. AnalysisRun returns Failed
   → plugin.Abort() called (rollback)
   → Degraded (aborted=true, auto-aborted by analysis)
```

### Example 8: Timeout with Auto-Abort
```
1. Rollout in progress, spec.progressDeadlineAbort=true
   → Progressing
   
2. progressDeadlineSeconds exceeded
   → plugin.Abort() called
   → Degraded (aborted=true, message="aborted due to timeout")
```

---

## Best Practices

1. **Aborting**: Always check `status.phase == "Degraded"` and `status.aborted == true` after abort to confirm
2. **Restarting**: Use `status.restart=true` only when `status.aborted == true`; rejected otherwise
3. **Pausing**: Prefer step-level pauses in strategy over `spec.paused` for predictable behavior
4. **PromoteFull**: Bypasses `spec.paused`; use for emergency full promotion
5. **Monitoring**: Watch `status.message` for detailed state information; check `status.conditions` for structured state
6. **New Revisions**: Deploying a new revision automatically clears abort state and resets to step 0
7. **Convergence**: Rollout waits for `currentRevision == updatedRevision` before declaring Successful

---

## Conditions

The RolloutPlugin maintains Kubernetes conditions that provide additional state information:

| Condition Type | Status | Reason | When Set |
|----------------|--------|--------|----------|
| Progressing | True | RolloutPluginProgressing | Rollout is actively progressing |
| Progressing | True | RolloutRestarted | Rollout was restarted from abort |
| Progressing | False | RolloutPluginAborted | Rollout was aborted (manual, timeout, or analysis) |
| Progressing | False | RolloutPluginPaused | Rollout is paused |
| Progressing | False | ProgressDeadlineExceeded | Rollout exceeded progressDeadlineSeconds |
| InvalidSpec | True | InvalidSpec | Plugin not found or configuration error |
| Completed | True | RolloutPluginCompleted | Rollout finished successfully |

---

## Summary

The RolloutPlugin CR provides a robust state machine for progressive delivery with clear states, transitions, and user actions. Key principles:

- **Progressing**: Active rollout execution (step processing, weight changes)
- **Paused**: Temporary halt (step pause, manual pause, inconclusive analysis)
- **Degraded**: Aborted state, requires explicit restart or new revision
- **Successful/Healthy**: Rollout completed, all pods converged
- **Failed**: Error state requiring intervention

Users control rollouts through:
- **Status fields** (one-shot triggers): `abort`, `restart`, `promoteFull`
- **Spec fields** (persistent): `paused`
- **PauseConditions** (cleared to promote): via ArgoCD Lua actions or kubectl patch

The controller manages automatic transitions based on step completion, pause durations, analysis results, timeouts, and health checks. New revisions always reset the rollout to step 0.
