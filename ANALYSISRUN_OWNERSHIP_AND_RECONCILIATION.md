# AnalysisRun Ownership and Reconciliation in RolloutPlugin

## Current Status ✅

**Good news!** The RolloutPlugin controller **already correctly sets owner references** on AnalysisRuns it creates. However, it's **missing the watch setup** to automatically reconcile when AnalysisRuns change.

## How Rollouts Controller Works

### 1. Owner Reference Setup
In `rollout/analysis.go` (line 485):
```go
run.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(c.rollout, controllerKind)}
```

### 2. Watch Configuration
In `rollout/controller.go` (lines 318-332):
```go
cfg.AnalysisRunInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj any) {
        controllerutil.EnqueueParentObject(obj, register.RolloutKind, controller.enqueueRollout)
    },
    UpdateFunc: func(old, new any) {
        oldAR := unstructuredutil.ObjectToAnalysisRun(old)
        newAR := unstructuredutil.ObjectToAnalysisRun(new)
        if oldAR == nil || newAR == nil {
            return
        }
        if newAR.Status.Phase == oldAR.Status.Phase {
            // Only enqueue rollout if the status changed
            return
        }
        controllerutil.EnqueueParentObject(new, register.RolloutKind, controller.enqueueRollout)
    },
    DeleteFunc: func(obj any) {
        controllerutil.EnqueueParentObject(obj, register.RolloutKind, controller.enqueueRollout)
    },
})
```

**Key mechanism**: `controllerutil.EnqueueParentObject` extracts the owner reference from the AnalysisRun and enqueues the parent Rollout for reconciliation.

## How RolloutPlugin Controller Currently Works

### 1. Owner Reference Setup ✅ CORRECT
In `rolloutplugin/analysis.go` (lines 308-310):
```go
// Set owner reference
ar.SetOwnerReferences([]metav1.OwnerReference{
    *metav1.NewControllerRef(rp, v1alpha1.SchemeGroupVersion.WithKind("RolloutPlugin")),
})
```

This is **correct** - AnalysisRuns created by RolloutPlugin have proper owner references pointing back to the RolloutPlugin.

### 2. Watch Configuration ❌ MISSING
In `rolloutplugin/controller.go` (lines 905-917), the `SetupWithManager` method currently only watches:
- **RolloutPlugin** resources (with custom predicates)
- **StatefulSet** resources (mapped via `findRolloutPluginsForWorkload`)

**Missing**: Watch on AnalysisRun resources!

## Solution: Add AnalysisRun Watch

You need to add an `Owns()` relationship in the `SetupWithManager` method to automatically trigger reconciliation when owned AnalysisRuns change.

### Option 1: Simple Owns() - Recommended ⭐
```go
func (r *RolloutPluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // ... existing predicates ...

    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.RolloutPlugin{}).
        Owns(&v1alpha1.AnalysisRun{}).  // 🆕 Add this line
        Watches(
            &appsv1.StatefulSet{},
            handler.EnqueueRequestsFromMapFunc(r.findRolloutPluginsForWorkload),
            builder.WithPredicates(statefulSetPredicate),
        ).
        Watches(
            &v1alpha1.RolloutPlugin{},
            &handler.EnqueueRequestForObject{},
            builder.WithPredicates(rolloutPluginPredicate),
        ).
        Complete(r)
}
```

**What `Owns()` does**:
1. Automatically watches AnalysisRun resources
2. When an AnalysisRun changes, controller-runtime extracts the owner reference
3. If the owner is a RolloutPlugin, it enqueues that RolloutPlugin for reconciliation
4. Your `Reconcile()` method is called automatically

### Option 2: Owns() with Predicate - More Efficient ⚡
```go
func (r *RolloutPluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // ... existing predicates ...

    // Predicate to filter AnalysisRun events - only trigger on phase changes
    analysisRunPredicate := predicate.Funcs{
        CreateFunc: func(e event.CreateEvent) bool {
            return true  // Always trigger on creation
        },
        UpdateFunc: func(e event.UpdateEvent) bool {
            oldAR, ok1 := e.ObjectOld.(*v1alpha1.AnalysisRun)
            newAR, ok2 := e.ObjectNew.(*v1alpha1.AnalysisRun)
            if !ok1 || !ok2 {
                return false
            }

            // Skip if ResourceVersion is the same (periodic resync)
            if oldAR.ResourceVersion == newAR.ResourceVersion {
                return false
            }

            // Only trigger if phase changed (matches Rollouts controller behavior)
            if oldAR.Status.Phase != newAR.Status.Phase {
                return true
            }

            // Skip other updates
            return false
        },
        DeleteFunc: func(e event.DeleteEvent) bool {
            return true  // Always trigger on deletion
        },
        GenericFunc: func(e event.GenericEvent) bool {
            return false
        },
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.RolloutPlugin{}).
        Owns(&v1alpha1.AnalysisRun{}, builder.WithPredicates(analysisRunPredicate)).  // 🆕 Add this line
        Watches(
            &appsv1.StatefulSet{},
            handler.EnqueueRequestsFromMapFunc(r.findRolloutPluginsForWorkload),
            builder.WithPredicates(statefulSetPredicate),
        ).
        Watches(
            &v1alpha1.RolloutPlugin{},
            &handler.EnqueueRequestForObject{},
            builder.WithPredicates(rolloutPluginPredicate),
        ).
        Complete(r)
}
```

**Benefits of Option 2**:
- Reduces unnecessary reconciliations (only triggers on phase changes)
- Matches the Rollouts controller behavior exactly
- More efficient for clusters with many AnalysisRuns

## What Happens After Adding the Watch

### Before (Current State):
1. RolloutPlugin creates AnalysisRun with owner reference ✅
2. Analysis controller runs metrics and updates AnalysisRun status
3. RolloutPlugin controller **does not** automatically reconcile ❌
4. RolloutPlugin only discovers AnalysisRun changes during:
   - Next periodic resync
   - Manual trigger (kubectl edit)
   - WorkloadRef changes

### After (With Watch):
1. RolloutPlugin creates AnalysisRun with owner reference ✅
2. Analysis controller runs metrics and updates AnalysisRun status
3. Controller-runtime detects AnalysisRun change ✅
4. Controller-runtime enqueues parent RolloutPlugin for reconciliation ✅
5. Your `Reconcile()` method is called immediately ✅
6. You can react to AnalysisRun phase changes in real-time ✅

## Example Flow with Watch

```
Timeline:
---------
T0: RolloutPlugin creates AnalysisRun
    - AnalysisRun.Status.Phase = "Pending"
    - AnalysisRun.OwnerReferences[0].UID = RolloutPlugin.UID

T1: Analysis controller starts running metrics
    - AnalysisRun.Status.Phase = "Running"
    - 🔔 Watch triggers!
    - Controller-runtime sees owner reference
    - Enqueues RolloutPlugin for reconciliation
    - Your Reconcile() method is called

T2: Metric succeeds
    - AnalysisRun.Status.Phase = "Successful"
    - 🔔 Watch triggers again!
    - Controller-runtime enqueues RolloutPlugin
    - Your Reconcile() method is called
    - You can now proceed to next step immediately!

T3 (Alternative): Metric fails
    - AnalysisRun.Status.Phase = "Failed"
    - 🔔 Watch triggers!
    - Your Reconcile() method is called
    - You can abort the rollout immediately!
```

## Comparison with Informer-based Approach

### Traditional Informers (Rollouts Controller)
```go
cfg.AnalysisRunInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    UpdateFunc: func(old, new any) {
        oldAR := unstructuredutil.ObjectToAnalysisRun(old)
        newAR := unstructuredutil.ObjectToAnalysisRun(new)
        if newAR.Status.Phase == oldAR.Status.Phase {
            return  // Only enqueue if phase changed
        }
        controllerutil.EnqueueParentObject(new, register.RolloutKind, controller.enqueueRollout)
    },
})
```

### Controller-Runtime (RolloutPlugin Controller)
```go
ctrl.NewControllerManagedBy(mgr).
    For(&v1alpha1.RolloutPlugin{}).
    Owns(&v1alpha1.AnalysisRun{}, builder.WithPredicates(analysisRunPredicate)).
    Complete(r)
```

**Both approaches achieve the same result**:
- ✅ Watch AnalysisRun resources
- ✅ Filter to only owned resources (via owner reference)
- ✅ Enqueue parent for reconciliation
- ✅ Optional filtering (phase change only)

**Difference**:
- Traditional informers: Manual event handler setup, explicit enqueue calls
- Controller-runtime: Declarative `Owns()` relationship, automatic enqueuing

## Do You Need the AnalysisHelper?

**Short answer: No, but it's convenient!**

### Without Helper (Pure controller-runtime)
You would need to implement these methods yourself:
```go
// Get AnalysisRuns owned by RolloutPlugin
func (r *RolloutPluginReconciler) getAnalysisRunsForRolloutPlugin(ctx context.Context, rp *v1alpha1.RolloutPlugin) ([]*v1alpha1.AnalysisRun, error) {
    var arList v1alpha1.AnalysisRunList
    if err := r.Client.List(ctx, &arList, 
        client.InNamespace(rp.Namespace),
        client.MatchingLabels{"rollout-plugin-name": rp.Name}); err != nil {
        return nil, err
    }
    // Filter by owner reference...
}

// Create AnalysisRun with owner reference
func (r *RolloutPluginReconciler) createAnalysisRun(...) (*v1alpha1.AnalysisRun, error) {
    ar := &v1alpha1.AnalysisRun{...}
    ar.SetOwnerReferences([]metav1.OwnerReference{
        *metav1.NewControllerRef(rp, v1alpha1.SchemeGroupVersion.WithKind("RolloutPlugin")),
    })
    if err := r.Client.Create(ctx, ar); err != nil {
        return nil, err
    }
    return ar, nil
}
```

### With Helper (Current Approach)
The helper provides:
- ✅ Convenient methods for common operations
- ✅ Shared code between Rollouts and RolloutPlugin controllers
- ✅ Tested utility functions (GetAnalysisRunsForOwner, CreateAnalysisRun, etc.)
- ✅ Less boilerplate code

**Recommendation**: Keep the helper! It's a clean abstraction that reduces code duplication.

## Implementation Summary

### What You Need to Do

1. **Add the watch** in `rolloutplugin/controller.go`:
   ```go
   return ctrl.NewControllerManagedBy(mgr).
       For(&v1alpha1.RolloutPlugin{}).
       Owns(&v1alpha1.AnalysisRun{}, builder.WithPredicates(analysisRunPredicate)).  // 🆕
       Watches(...).
       Complete(r)
   ```

2. **Keep the AnalysisHelper** - no changes needed!
   - It already sets owner references correctly ✅
   - Your existing analysis reconciliation logic works ✅

3. **Your Reconcile() method** will now be called automatically when:
   - AnalysisRuns are created
   - AnalysisRun phase changes (with predicate)
   - AnalysisRuns are deleted

### What You Don't Need to Do

- ❌ Change owner reference setup (already correct)
- ❌ Remove the helper (it's useful)
- ❌ Rewrite analysis reconciliation logic
- ❌ Manual polling or periodic checks

## Benefits

After adding the watch:

1. **Real-time reactivity**: RolloutPlugin reconciles immediately when AnalysisRun completes
2. **Faster rollouts**: No waiting for periodic resync to detect analysis completion
3. **Consistency**: Matches Rollouts controller behavior exactly
4. **Correctness**: Automatic reconciliation on analysis failures/successes
5. **Simplicity**: Let controller-runtime handle the plumbing

## Testing the Change

After implementing, test with:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: test-rp
spec:
  workloadRef:
    kind: StatefulSet
    name: test-sts
  strategy:
    canary:
      steps:
      - setWeight: 20
      - pause: {}
      - analysis:
          templates:
          - templateName: success-rate
```

**Expected behavior**:
1. Create RolloutPlugin
2. Watch logs: `kubectl logs -n argo-rollouts deployment/argo-rollouts -f`
3. You should see reconciliation triggered when:
   - AnalysisRun is created
   - AnalysisRun phase changes (Pending → Running → Successful)
   - AnalysisRun completes

**Log output should show**:
```
INFO Reconciling RolloutPlugin namespace=default rolloutplugin=test-rp
INFO Creating step-based analysis run step=1
INFO Created AnalysisRun analysisRun=test-rp-step-1-...
INFO Reconciling RolloutPlugin namespace=default rolloutplugin=test-rp  # Triggered by AR creation
INFO AnalysisRun phase changed to Running
INFO Reconciling RolloutPlugin namespace=default rolloutplugin=test-rp  # Triggered by phase change
INFO AnalysisRun phase changed to Successful
INFO Reconciling RolloutPlugin namespace=default rolloutplugin=test-rp  # Triggered by phase change
INFO Moving to next step
```

## Conclusion

The RolloutPlugin controller is **99% correct** - it already sets owner references properly. You just need to add the `Owns(&v1alpha1.AnalysisRun{})` relationship to enable automatic reconciliation when AnalysisRuns change, exactly like the Rollouts controller does with informers.

This is a **one-line change** that will make your controller fully reactive to AnalysisRun lifecycle events! 🚀
