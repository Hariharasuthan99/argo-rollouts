# RolloutPlugin Notification Implementation

## Overview
Implemented notification engine support for RolloutPlugin CR, following the same pattern as the existing Rollout CR notification system. This allows users to receive notifications for RolloutPlugin lifecycle events through various channels (Slack, email, webhooks, etc.).

## Implementation Details

### 1. EventRecorder Integration

#### Updated Files:
- `utils/record/record.go`
- `rolloutplugin/controller.go`
- `cmd/rollouts-controller/main.go`

#### Changes in `utils/record/record.go`:

**Added `NewAPIFactorySettingsForRolloutPlugin()` function:**
```go
func NewAPIFactorySettingsForRolloutPlugin(arInformer argoinformers.AnalysisRunInformer) api.Settings {
	return api.Settings{
		SecretName:    NotificationSecret,
		ConfigMapName: NotificationConfigMap,
		InitGetVars: func(cfg *api.Config, configMap *corev1.ConfigMap, secret *corev1.Secret) (api.GetVars, error) {
			return func(obj map[string]any, dest services.Destination) map[string]any {
				var vars = map[string]any{
					"rolloutPlugin": obj,
					"time":          timeExprs,
					"secrets":       secret.Data,
				}

				if arInformer == nil {
					log.Infof("Notification is not set for analysisRun Informer: %s", dest)
					return vars
				}

				var rp v1alpha1.RolloutPlugin
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &rp)

				if err != nil {
					log.Errorf("unable to send notification: bad rolloutPlugin object: %v", err)
					return vars
				}

				arsObj, err := getAnalysisRunsForRolloutPlugin(rp, arInformer)

				if err != nil {
					log.Errorf("Error calling getAnalysisRunsForRolloutPlugin for namespace: %s",
						rp.Namespace)
					return vars
				}

				vars = map[string]any{
					"rolloutPlugin": obj,
					"analysisRuns":  arsObj,
					"time":          timeExprs,
					"secrets":       secret.Data,
				}
				return vars
			}, nil
		},
	}
}
```

This creates a separate API factory for RolloutPlugin with:
- `rolloutPlugin` variable (the RolloutPlugin object)
- `analysisRuns` variable (analysis runs owned by the RolloutPlugin)
- `time` and `secrets` helper variables

**Added `getAnalysisRunsForRolloutPlugin()` helper function:**
```go
func getAnalysisRunsForRolloutPlugin(rp v1alpha1.RolloutPlugin, arInformer argoinformers.AnalysisRunInformer) (any, error) {
	// Get all analysis runs in the namespace
	ars, err := arInformer.Lister().AnalysisRuns(rp.Namespace).List(labels.Everything())
	// ...
	// Filter for analysis runs owned by this RolloutPlugin
	filteredArs := make([]*v1alpha1.AnalysisRun, 0)
	for _, ar := range ars {
		for _, ownerRef := range ar.OwnerReferences {
			if ownerRef.Kind == "RolloutPlugin" &&
				ownerRef.Name == rp.Name &&
				ownerRef.UID == rp.UID {
				filteredArs = append(filteredArs, ar)
				break
			}
		}
	}
	// Sort by creation timestamp (newest first)
	// Marshal to any type for template usage
	// ...
}
```

This retrieves analysis runs owned by the RolloutPlugin using owner references and sorts them by creation timestamp.

**Updated `defaultEventf()` to track RolloutPlugin events:**
```go
// Also increment for RolloutPlugin - reusing the same metric
if kind == "RolloutPlugin" {
    e.RolloutEventCounter.WithLabelValues(namespace, name, opts.EventType, opts.EventReason).Inc()
}
```

### 2. RolloutPluginReconciler Updates

**Added `Recorder` field:**
```go
type RolloutPluginReconciler struct {
	// ...existing fields...
	Recorder          record.EventRecorder
	// ...existing fields...
}
```

**Added event recording at key lifecycle points:**

1. **Deletion Event:**
   - EventReason: `RolloutPluginDeleted`
   - Message: "RolloutPlugin '%s' deleted"

2. **Invalid Spec Event:**
   - EventReason: `RolloutPluginInvalidSpecReason` (from conditions)
   - Message: Validation error message
   - Type: Warning

3. **Manual Pause Event:**
   - EventReason: `RolloutPluginPausedReason`
   - Message: `RolloutPluginPausedMessage` (from conditions)

4. **Manual Resume Event:**
   - EventReason: `RolloutPluginResumed`
   - Message: "RolloutPlugin resumed from manual pause"

5. **Abort Event:**
   - EventReason: `RolloutPluginAbortedReason`
   - Message: `RolloutPluginAbortedMessage` (from conditions)
   - Type: Warning

6. **Timeout Event:**
   - EventReason: `RolloutPluginTimedOutReason`
   - Message: "RolloutPlugin %q has timed out progressing."
   - Type: Warning

7. **Rollout Started/Updated Event:**
   - EventReason: `RolloutPluginProgressingReason`
   - Message: "RolloutPlugin updated to revision %s"

8. **Step Pause Event:**
   - EventReason: `RolloutPluginPausedReason`
   - Message: "RolloutPlugin paused at step %d"

9. **Step Resume Event:**
   - EventReason: `RolloutPluginResumed`
   - Message: "RolloutPlugin resumed from pause at step %d"

10. **Completion Event:**
    - EventReason: `RolloutPluginCompletedReason`
    - Message: "RolloutPlugin completed update to revision %s"
    - Also sent for full promotion with message: "...revision %s (full promotion)"

### 3. Main Controller Wiring

**In `cmd/rollouts-controller/main.go`:**

Added notification infrastructure setup for RolloutPlugin:

```go
// Create EventRecorder for RolloutPlugin notifications
// Pass AnalysisRun informer to enable analysisRuns variable in notification templates
rolloutPluginApiFactory := notificationapi.NewFactory(
    record.NewAPIFactorySettingsForRolloutPlugin(tolerantinformer.NewTolerantAnalysisRunInformer(dynamicInformerFactory)),
    defaults.Namespace(),
    notificationSecretInformerFactory.Core().V1().Secrets().Informer(),
    notificationConfigMapInformerFactory.Core().V1().ConfigMaps().Informer(),
)
rolloutPluginRecorder := record.NewEventRecorder(
    kubeClient,
    metrics.MetricRolloutEventsTotal,
    metrics.MetricNotificationFailedTotal,
    metrics.MetricNotificationSuccessTotal,
    metrics.MetricNotificationSend,
    rolloutPluginApiFactory,
)

// Pass recorder to RolloutPluginReconciler
if err = (&rolloutplugin.RolloutPluginReconciler{
    // ...existing fields...
    Recorder: rolloutPluginRecorder,
}).SetupWithManager(mgr); err != nil {
    log.Fatalf("Failed to setup RolloutPlugin controller: %s", err.Error())
}
```

## Notification Triggers

Based on the event reasons, the following notification triggers can be used in notification templates:

1. `on-rollout-plugin-deleted` - When RolloutPlugin is deleted
2. `on-rollout-plugin-invalid-spec` - When spec validation fails
3. `on-rollout-plugin-paused` - When RolloutPlugin is paused (manual or step)
4. `on-rollout-plugin-resumed` - When RolloutPlugin is resumed
5. `on-rollout-plugin-aborted` - When RolloutPlugin is aborted
6. `on-progress-deadline-exceeded` - When RolloutPlugin times out
7. `on-rollout-plugin-progressing` - When new rollout starts
8. `on-rollout-plugin-completed` - When rollout completes successfully

## Configuration Example

### ConfigMap: `argo-rollouts-notification-configmap`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-notification-configmap
  namespace: argo-rollouts
data:
  service.slack: |
    token: $slack-token
  
  template.rolloutplugin-completed: |
    message: |
      RolloutPlugin {{.rolloutPlugin.metadata.name}} completed successfully!
      Revision: {{.rolloutPlugin.status.updatedRevision}}
      Namespace: {{.rolloutPlugin.metadata.namespace}}
      {{- if .analysisRuns}}
      Analysis Runs: {{len .analysisRuns}}
      {{- range .analysisRuns}}
      - {{.metadata.name}}: {{.status.phase}}
      {{- end}}
      {{- end}}
  
  template.rolloutplugin-aborted: |
    message: |
      ⚠️ RolloutPlugin {{.rolloutPlugin.metadata.name}} was aborted!
      Revision: {{.rolloutPlugin.status.abortedRevision}}
      Namespace: {{.rolloutPlugin.metadata.namespace}}
      {{- if .analysisRuns}}
      {{- $failedArs := 0}}
      {{- range .analysisRuns}}
      {{- if or (eq .status.phase "Failed") (eq .status.phase "Error")}}
      {{- $failedArs = add $failedArs 1}}
      - Failed Analysis: {{.metadata.name}} ({{.status.phase}})
      {{- end}}
      {{- end}}
      {{- if gt $failedArs 0}}
      Total Failed Analysis Runs: {{$failedArs}}
      {{- end}}
      {{- end}}
  
  trigger.on-rollout-plugin-completed: |
    - send: [rolloutplugin-completed]
  
  trigger.on-rollout-plugin-aborted: |
    - send: [rolloutplugin-aborted]
```

### Secret: `argo-rollouts-notification-secret`

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: argo-rollouts-notification-secret
  namespace: argo-rollouts
stringData:
  slack-token: xoxb-your-slack-token
```

### Subscription via Annotations

On the RolloutPlugin CR:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: my-rolloutplugin
  annotations:
    notifications.argoproj.io/subscribe.on-rollout-plugin-completed.slack: my-channel
    notifications.argoproj.io/subscribe.on-rollout-plugin-aborted.slack: my-channel
spec:
  # ... rolloutplugin spec ...
```

## Variables Available in Templates

The following variables are available in notification templates for RolloutPlugin:

- `{{.rolloutPlugin}}` - The entire RolloutPlugin object
  - `{{.rolloutPlugin.metadata.name}}` - RolloutPlugin name
  - `{{.rolloutPlugin.metadata.namespace}}` - RolloutPlugin namespace
  - `{{.rolloutPlugin.spec}}` - RolloutPlugin spec
  - `{{.rolloutPlugin.status}}` - RolloutPlugin status
  - `{{.rolloutPlugin.status.updatedRevision}}` - Current updated revision
  - `{{.rolloutPlugin.status.currentRevision}}` - Current stable revision
  - `{{.rolloutPlugin.status.phase}}` - Current phase
  - `{{.rolloutPlugin.status.abortedRevision}}` - Aborted revision (if aborted)
  - `{{.rolloutPlugin.status.currentStepIndex}}` - Current canary step index
- `{{.analysisRuns}}` - Array of AnalysisRun objects owned by this RolloutPlugin (sorted by creation time, newest first)
  - `{{range .analysisRuns}}{{.metadata.name}}{{end}}` - Iterate through analysis runs
  - `{{(index .analysisRuns 0).status.phase}}` - Phase of the most recent analysis run
  - Can access any AnalysisRun field (name, namespace, status, metrics, etc.)
- `{{.time}}` - Time helper functions
  - `{{.time.Now}}` - Current time
  - `{{.time.Parse "RFC3339-timestamp"}}` - Parse timestamp
- `{{.secrets}}` - Access to secret data from notification secret

## Metrics

The implementation reuses existing rollout event metrics:

- `rollout_events_total` - Counter for all events (including RolloutPlugin events)
  - Labels: `namespace`, `name`, `type`, `reason`
- `notification_send_success` - Counter for successful notifications
- `notification_send_error` - Counter for failed notifications
- `notification_send` - Histogram for notification send duration

## Differences from Rollout Notifications

1. **Variable Name**: Uses `rolloutPlugin` instead of `rollout` in templates
2. **AnalysisRun Filtering**: 
   - **Rollout**: Filters analysisRuns by pod hash label and revision annotation
   - **RolloutPlugin**: Filters analysisRuns by owner reference (Kind=RolloutPlugin, Name, UID)
   - Both sorted by creation timestamp (newest first)
   - RolloutPlugin's approach is cleaner as it uses proper Kubernetes ownership
3. **Event Reasons**: Uses RolloutPlugin-specific event reasons from conditions package

## Testing

To test notifications:

1. Deploy notification ConfigMap and Secret with desired service (Slack, email, etc.)
2. Add notification subscription annotations to RolloutPlugin CR
3. Perform operations that trigger events (update, pause, abort, etc.)
4. Verify notifications are received via configured channels

## Future Enhancements

Potential improvements for the future:

1. **Step Information**: Add current step details to notification context (e.g., step name, weight, duration)
2. **WorkloadRef Status**: Include referenced workload status in notifications
3. **Custom Triggers**: Support for user-defined custom triggers based on conditions
4. **Notification Controller**: Similar to Rollout, could add a separate notification controller for RolloutPlugin if needed
5. **Analysis Metrics**: Include detailed metric results from AnalysisRuns in notifications

## Benefits

1. **Observability**: Teams get real-time notifications about RolloutPlugin lifecycle events
2. **Alerting**: Can set up alerts for failures, timeouts, and aborts
3. **Audit Trail**: Event history in notification channels provides audit trail
4. **Integration**: Works with existing notification infrastructure (Slack, PagerDuty, email, webhooks, etc.)
5. **Flexibility**: Users can customize triggers and templates per their needs
6. **Consistency**: Follows same pattern as Rollout notifications for familiar UX
