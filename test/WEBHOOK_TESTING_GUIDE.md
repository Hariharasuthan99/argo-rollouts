# Testing RolloutPlugin Notifications with Webhook Receiver

This guide shows how to test RolloutPlugin notifications using a simple webhook receiver.

## Step 1: Deploy the Webhook Receiver

```bash
# Deploy to the argo-rollouts namespace (or change namespace in the yaml)
kubectl apply -f test/webhook-receiver.yaml

# Verify it's running
kubectl get pods -n argo-rollouts -l app=webhook-receiver

# Check the service
kubectl get svc -n argo-rollouts webhook-receiver
```

## Important: Local Development vs In-Cluster Controller

**If you're running the controller as a local process** (not as a pod in the cluster), you **CANNOT** use Kubernetes internal DNS names like `webhook-receiver.argo-rollouts.svc.cluster.local` because:

- Kubernetes DNS only works inside the cluster
- Your local process cannot resolve `.svc.cluster.local` domains

### Solution for Local Development:

1. **Port-forward the webhook receiver:**

   ```bash
   kubectl port-forward -n argo-rollouts svc/webhook-receiver 8080:8080
   ```

2. **Use `localhost` in the notification ConfigMap** (see Step 2 below):

   ```yaml
   service.webhook.test-webhook: |
     url: http://localhost:8080  # Use localhost for local controller
   ```

3. **Keep the port-forward running** while testing notifications.

**If the controller is running in-cluster** (as a pod), use the cluster DNS name:

```yaml
service.webhook.test-webhook: |
  url: http://webhook-receiver.argo-rollouts.svc.cluster.local:8080
```

## Step 2: Configure Notifications to Use Webhook

Update or create the notification ConfigMap with **all 10 event types**:

**For local controller (with port-forward):**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-notification-configmap
  namespace: argo-rollouts
data:
  # Define webhook service - USE LOCALHOST FOR LOCAL CONTROLLER
  service.webhook.test-webhook: |
    url: http://localhost:8080
    headers:
    - name: Content-Type
      value: application/json
  
  # ... templates and triggers below ...
```

**For in-cluster controller:**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-notification-configmap
  namespace: argo-rollouts
data:
  # Define webhook service - USE CLUSTER DNS FOR IN-CLUSTER CONTROLLER
  service.webhook.test-webhook: |
    url: http://192.168.49.2:30080
    headers:
    - name: Content-Type
      value: application/json
  
  # ============================================================================
  # Notification Templates - All 10 RolloutPlugin Event Types
  # ============================================================================
  
  # 1. Rollout Started/Updated (Progressing)
  template.rolloutplugin-progressing: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-progressing",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "updatedRevision": "{{.rolloutPlugin.status.updatedRevision}}",
            "currentRevision": "{{.rolloutPlugin.status.currentRevision}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # 2. Rollout Completed (Success)
  template.rolloutplugin-completed: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-completed",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "revision": "{{.rolloutPlugin.status.updatedRevision}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "timestamp": "{{call .time.Now}}",
            {{- if .analysisRuns}}
            "analysisRuns": [
              {{- range $index, $ar := .analysisRuns}}
              {{- if $index}},{{end}}
              {
                "name": "{{$ar.metadata.name}}",
                "phase": "{{$ar.status.phase}}",
                "message": "{{$ar.status.message}}"
              }
              {{- end}}
            ]
            {{- else}}
            "analysisRuns": []
            {{- end}}
          }
  
  # 3. Rollout Paused (Manual or Step)
  template.rolloutplugin-paused: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-paused",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            {{- if .rolloutPlugin.status.pauseConditions}}
            "pauseConditions": true,
            {{- end}}
            {{- if .rolloutPlugin.status.currentStepIndex}}
            "currentStep": {{.rolloutPlugin.status.currentStepIndex}},
            {{- end}}
            "message": "{{.rolloutPlugin.status.message}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # 4. Rollout Resumed (Manual or Step Completed)
  template.rolloutplugin-resumed: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-resumed",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            {{- if .rolloutPlugin.status.currentStepIndex}}
            "currentStep": {{.rolloutPlugin.status.currentStepIndex}},
            {{- end}}
            "timestamp": "{{call .time.Now}}"
          }
  
  # 5. Rollout Aborted (Manual or Timeout)
  template.rolloutplugin-aborted: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-aborted",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "abortedRevision": "{{.rolloutPlugin.status.abortedRevision}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "message": "{{.rolloutPlugin.status.message}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # 6. Rollout Timed Out (Progress Deadline Exceeded)
  template.rolloutplugin-timedout: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-timedout",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "message": "{{.rolloutPlugin.status.message}}",
            "progressDeadlineSeconds": {{.rolloutPlugin.spec.progressDeadlineSeconds}},
            "timestamp": "{{call .time.Now}}"
          }
  
  # 7. Invalid Spec (Validation Failed)
  template.rolloutplugin-invalid-spec: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-invalid-spec",
            "severity": "warning",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "message": "{{.rolloutPlugin.status.message}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # 8. RolloutPlugin Deleted
  template.rolloutplugin-deleted: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "event": "rolloutplugin-deleted",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # ============================================================================
  # Notification Triggers - Map Event Reasons to Templates
  # ============================================================================
  
  # Trigger on rollout started/updated
  trigger.on-rollout-plugin-progressing: |
    - when: rolloutPlugin.status.phase == 'Progressing'
      send: [rolloutplugin-progressing]
  
  # Trigger on successful completion
  trigger.on-rollout-plugin-completed: |
    - send: [rolloutplugin-completed]
  
  # Trigger on pause (manual or step)
  trigger.on-rollout-plugin-paused: |
    - send: [rolloutplugin-paused]
  
  # Trigger on resume (manual or step completed)
  trigger.on-rollout-plugin-resumed: |
    - send: [rolloutplugin-resumed]
  
  # Trigger on abort
  trigger.on-rollout-plugin-aborted: |
    - send: [rolloutplugin-aborted]
  
  # Trigger on timeout
  trigger.on-rollout-plugin-timedout: |
    - send: [rolloutplugin-timedout]
  
  # Trigger on invalid spec
  trigger.on-rollout-plugin-invalid-spec: |
    - send: [rolloutplugin-invalid-spec]
  
  # Trigger on deletion
  trigger.on-rollout-plugin-deleted: |
    - send: [rolloutplugin-deleted]
  
  # ============================================================================
  # Custom Trigger Examples - Advanced Scenarios
  # ============================================================================
  
  # Custom Template 1: Critical service completion alert
  template.custom-critical-service-completed: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "triggerType": "custom",
            "event": "critical-service-completed",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "revision": "{{.rolloutPlugin.status.updatedRevision}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # Custom Template 2: Stuck rollout alert
  template.custom-rollout-stuck: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "triggerType": "custom",
            "event": "rollout-stuck-at-step",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "currentStep": {{.rolloutPlugin.status.currentStepIndex}},
            "message": "Rollout is stuck/paused at step {{.rolloutPlugin.status.currentStepIndex}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # Custom Template 3: StatefulSet plugin rollout notification
  template.custom-statefulset-plugin-event: |
    webhook:
      test-webhook:
        method: POST
        body: |
          {
            "triggerType": "custom",
            "event": "statefulset-plugin-rollout",
            "name": "{{.rolloutPlugin.metadata.name}}",
            "namespace": "{{.rolloutPlugin.metadata.namespace}}",
            "plugin": "{{.rolloutPlugin.spec.plugin.name}}",
            "workloadKind": "{{.rolloutPlugin.spec.workloadRef.kind}}",
            "workloadName": "{{.rolloutPlugin.spec.workloadRef.name}}",
            "phase": "{{.rolloutPlugin.status.phase}}",
            "updatedRevision": "{{.rolloutPlugin.status.updatedRevision}}",
            "message": "StatefulSet plugin rollout event for {{.rolloutPlugin.spec.workloadRef.name}}",
            "timestamp": "{{call .time.Now}}"
          }
  
  # Custom: Notify only for specific RolloutPlugin names (e.g., critical services)
  trigger.on-critical-rollout-plugin-completed: |
    - when: rolloutPlugin.metadata.name == 'critical-app-rollout' || rolloutPlugin.metadata.name == 'payment-service-rollout'
      send: [custom-critical-service-completed]
  
  # Custom: Notify on stuck/paused rollout (paused at a step)
  trigger.on-rollout-plugin-stuck: |
    - when: rolloutPlugin.status.phase == 'Paused' && rolloutPlugin.status.currentStepIndex != nil
      oncePer: rolloutPlugin.metadata.name
      send: [custom-rollout-stuck]
  
  # Custom: Notify when plugin type is statefulset
  trigger.on-statefulset-plugin-rollout: |
    - when: rolloutPlugin.spec.plugin.name == 'statefulset'
      send: [custom-statefulset-plugin-event]
```

Apply the configuration:

```bash
kubectl apply -f <your-notification-config>.yaml
```

## Step 3: Subscribe RolloutPlugin to Notifications

Add annotations to your RolloutPlugin to subscribe to **all notification events**:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: my-rolloutplugin
  namespace: default
  annotations:
    # Subscribe to all standard events
    notifications.argoproj.io/subscribe.on-rollout-plugin-progressing.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-completed.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-paused.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-resumed.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-aborted.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-deleted.test-webhook: ""
    # NOTE: timeout and invalid-spec use their translated event reason names:
    notifications.argoproj.io/subscribe.on-progress-deadline-exceeded.test-webhook: ""
    notifications.argoproj.io/subscribe.on-invalid-spec.test-webhook: ""
    # Subscribe to standalone custom triggers (evaluated by NotificationController):
    notifications.argoproj.io/subscribe.on-rollout-plugin-stuck.test-webhook: ""
    notifications.argoproj.io/subscribe.on-statefulset-plugin-rollout.test-webhook: ""
    notifications.argoproj.io/subscribe.on-critical-rollout-plugin-completed.test-webhook: ""
spec:
  # ... your rolloutplugin spec ...
```

**Note:** You can subscribe to individual events by removing unwanted annotation lines.

### Using Custom Triggers

> ℹ️ **How standalone custom triggers work for RolloutPlugin**
>
> RolloutPlugin has **two separate notification paths**, identical to the standard Rollout CR:
> 1. **`NotificationController`** (notifications-engine `pkg/controller`) — watches the RolloutPlugin informer, and on every object change iterates over **all** subscribed trigger names from annotations, evaluates their `when` conditions, and sends if true. This is how standalone custom triggers like `trigger.on-rollout-plugin-stuck` and `trigger.on-statefulset-plugin-rollout` fire — they are evaluated against the current object state on every informer update.
> 2. **`sendNotifications`** in `EventRecorderAdapter` — fires on each `Eventf`/`Warnf` call, maps `EventReason → trigger name` via `translateReasonToTrigger`, and evaluates that specific trigger.
>
> **Both paths are active for RolloutPlugin.** A trigger like `trigger.on-statefulset-plugin-rollout` with only a `when: rolloutPlugin.spec.plugin.name == 'statefulset'` condition will be evaluated automatically by the `NotificationController` on every spec/status change — no EventReason mapping required.

Subscribe to the custom triggers using their standalone names:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: test-sts-canary-rollout
  namespace: argo-rollouts
  annotations:
    # Standard events
    notifications.argoproj.io/subscribe.on-rollout-plugin-progressing.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-completed.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-paused.test-webhook: ""

    # Standalone custom triggers — evaluated by NotificationController on every object change
    notifications.argoproj.io/subscribe.on-rollout-plugin-stuck.test-webhook: ""
    notifications.argoproj.io/subscribe.on-statefulset-plugin-rollout.test-webhook: ""
    notifications.argoproj.io/subscribe.on-critical-rollout-plugin-completed.test-webhook: ""
spec:
  plugin:
    name: statefulset   # ← the on-statefulset-plugin-rollout trigger matches this field
  # ... rest of spec ...
```

**How each custom trigger fires:**

| Custom Trigger | `when` condition | Evaluation path |
| --- | --- | --- |
| `on-rollout-plugin-stuck` | `rolloutPlugin.status.phase == 'Paused' && rolloutPlugin.status.currentStepIndex != nil` | `NotificationController` (on every object change) |
| `on-statefulset-plugin-rollout` | `rolloutPlugin.spec.plugin.name == 'statefulset'` | `NotificationController` (on every object change) |
| `on-critical-rollout-plugin-completed` | `rolloutPlugin.metadata.name == 'critical-app-rollout' \|\| ...` | `NotificationController` (on every object change) |

**Available fields in `when` conditions:**

- `rolloutPlugin.metadata.name` - RolloutPlugin name
- `rolloutPlugin.metadata.namespace` - Namespace
- `rolloutPlugin.status.phase` - Current phase (Progressing, Successful, Failed, Degraded, Paused)
- `rolloutPlugin.status.aborted` - Boolean, true if aborted
- `rolloutPlugin.status.pauseConditions` - Array, non-empty when controller-paused
- `rolloutPlugin.status.controllerPause` - Boolean, true if controller paused
- `rolloutPlugin.status.currentStepIndex` - Current step number
- `rolloutPlugin.spec.plugin.name` - Plugin type (e.g. `statefulset`)
- `analysisRuns` - Array of analysis runs (if any)

## Step 4: Watch Webhook Receiver Logs

Open a terminal and watch the webhook receiver logs to see incoming notifications:

```bash
kubectl logs -f -n argo-rollouts -l app=webhook-receiver
```

You should see output like:

```
Starting webhook receiver on port 8080...
Ready to receive notifications!

================================================================================
[2026-02-24 10:30:45] Received webhook POST request
================================================================================

--- Headers ---
Content-Type: application/json
User-Agent: ArgoRollouts/1.0
Content-Length: 234

--- Body ---
{
  "event": "rolloutplugin-completed",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "revision": "abc123",
  "phase": "Successful",
  "timestamp": "2026-02-24T10:30:45Z",
  "analysisRuns": [
    {
      "name": "my-rolloutplugin-analysis-abc123",
      "phase": "Successful"
    }
  ]
}

================================================================================
```

## Step 5: Trigger Events

Trigger various events to test all notification scenarios:

### 1. Test Rollout Started (Progressing)

Create or update a RolloutPlugin to start a new rollout:

```bash
# Apply a RolloutPlugin or update the workload to trigger a new revision
kubectl apply -f my-rolloutplugin.yaml
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-progressing",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "updatedRevision": "abc123",
  "currentRevision": "xyz789",
  "phase": "Progressing",
  "timestamp": "2026-02-24T10:30:00Z"
}
```

### 2. Test Completion (Success)

Let the rollout complete naturally or use full promotion:

```bash
# Option 1: Let it complete through all steps naturally
# (wait for rollout to finish)

# Option 2: Use full promotion to complete immediately
kubectl patch rolloutplugin my-rolloutplugin -n default --type=merge -p '{"status":{"promoteFull":true}}'
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-completed",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "revision": "abc123",
  "phase": "Successful",
  "timestamp": "2026-02-24T10:35:00Z",
  "analysisRuns": [
    {
      "name": "my-rolloutplugin-analysis-abc123",
      "phase": "Successful",
      "message": "Run Completed"
    }
  ]
}
```

### 3. Test Manual Pause

Pause the rollout manually:

```bash
kubectl patch rolloutplugin my-rolloutplugin -n default --type=merge -p '{"spec":{"paused":true}}'
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-paused",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "phase": "Paused",
  "paused": true,
  "message": "manually paused",
  "timestamp": "2026-02-24T10:32:00Z"
}
```

### 4. Test Manual Resume

Resume from manual pause:

```bash
kubectl patch rolloutplugin my-rolloutplugin -n default --type=merge -p '{"spec":{"paused":false}}'
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-resumed",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "phase": "Progressing",
  "timestamp": "2026-02-24T10:33:00Z"
}
```

### 5. Test Abort

Abort the rollout:

```bash
kubectl patch rolloutplugin my-rolloutplugin -n default --type=merge -p '{"status":{"abort":true}}'
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-aborted",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "abortedRevision": "abc123",
  "phase": "Degraded",
  "message": "Rollout aborted by user",
  "timestamp": "2026-02-24T10:34:00Z"
}
```

### 6. Test Timeout

Set a short progress deadline to trigger timeout:

```bash
# Create RolloutPlugin with short timeout (e.g., 10 seconds)
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: my-rolloutplugin
  namespace: default
  annotations:
    notifications.argoproj.io/subscribe.on-rollout-plugin-timedout.test-webhook: ""
spec:
  progressDeadlineSeconds: 10
  # ... rest of spec that takes longer than 10s ...
EOF

# Wait for timeout to occur
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-timedout",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "phase": "Progressing",
  "message": "RolloutPlugin my-rolloutplugin has timed out progressing after 10 seconds",
  "progressDeadlineSeconds": 10,
  "timestamp": "2026-02-24T10:30:20Z"
}
```

### 7. Test Invalid Spec

Create a RolloutPlugin with invalid configuration:

```bash
# Example: Missing required plugin name
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: invalid-rolloutplugin
  namespace: default
  annotations:
    notifications.argoproj.io/subscribe.on-rollout-plugin-invalid-spec.test-webhook: ""
spec:
  plugin:
    name: ""  # Invalid: empty plugin name
  workloadRef:
    kind: StatefulSet
    name: my-app
EOF
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-invalid-spec",
  "severity": "warning",
  "name": "invalid-rolloutplugin",
  "namespace": "default",
  "phase": "Failed",
  "message": "Plugin name is required",
  "timestamp": "2026-02-24T10:36:00Z"
}
```

### 8. Test Deletion

Delete a RolloutPlugin:

```bash
kubectl delete rolloutplugin my-rolloutplugin -n default
```

**Expected Webhook Payload:**

```json
{
  "event": "rolloutplugin-deleted",
  "name": "my-rolloutplugin",
  "namespace": "default",
  "timestamp": "2026-02-24T10:37:00Z"
}
```

### 9. Test Step Pause (Automatic)

Create a RolloutPlugin with pause steps:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: step-pause-test
  namespace: default
  annotations:
    notifications.argoproj.io/subscribe.on-rollout-plugin-paused.test-webhook: ""
    notifications.argoproj.io/subscribe.on-rollout-plugin-resumed.test-webhook: ""
spec:
  plugin:
    name: statefulset
  workloadRef:
    kind: StatefulSet
    name: my-app
  strategy:
    canary:
      steps:
      - setWeight: 20
      - pause:
          duration: 30s  # Will trigger pause and resume events
      - setWeight: 50
EOF

# Step pause will automatically trigger:
# 1. Pause event when step starts
# 2. Resume event when duration elapses
```

**Expected Webhook Payloads:**

Pause (when step starts):

```json
{
  "event": "rolloutplugin-paused",
  "name": "step-pause-test",
  "namespace": "default",
  "phase": "Paused",
  "paused": true,
  "currentStep": 1,
  "message": "Paused",
  "timestamp": "2026-02-24T10:40:00Z"
}
```

Resume (when duration elapses):

```json
{
  "event": "rolloutplugin-resumed",
  "name": "step-pause-test",
  "namespace": "default",
  "phase": "Progressing",
  "currentStep": 2,
  "timestamp": "2026-02-24T10:40:30Z"
}
```

## Health Check

You can also check if the webhook receiver is running:

```bash
kubectl port-forward -n argo-rollouts svc/webhook-receiver 8080:8080

# In another terminal
curl http://localhost:8080
# Should return: Webhook receiver is running
```

## Cleanup

When you're done testing:

```bash
kubectl delete -f test/webhook-receiver.yaml
```

## Troubleshooting

### No notifications received?

1. **Check controller logs:**
   ```bash
   kubectl logs -n argo-rollouts -l app.kubernetes.io/name=argo-rollouts
   ```

2. **Verify ConfigMap is loaded:**
   ```bash
   kubectl get cm -n argo-rollouts argo-rollouts-notification-configmap -o yaml
   ```

3. **Check webhook receiver is deployed and running:**
   ```bash
   # Check if pod is running
   kubectl get pods -n argo-rollouts -l app=webhook-receiver
   
   # Check service exists
   kubectl get svc -n argo-rollouts webhook-receiver
   
   # Verify service has endpoints
   kubectl get endpoints -n argo-rollouts webhook-receiver
   ```

4. **Test DNS resolution from controller pod:**
   ```bash
   # Get controller pod name
   CONTROLLER_POD=$(kubectl get pods -n argo-rollouts -l app.kubernetes.io/name=argo-rollouts -o jsonpath='{.items[0].metadata.name}')
   
   # Test DNS lookup
   kubectl exec -n argo-rollouts webhook-receiver-5bb64ffb47-tcfxx -- nslookup webhook-receiver.argo-rollouts.svc.cluster.local
   ```

5. **Check webhook receiver is reachable:**
   ```bash
   kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
     curl -v http://webhook-receiver.argo-rollouts.svc.cluster.local:8080
   ```

6. **Verify annotations on RolloutPlugin:**
   ```bash
   kubectl get rolloutplugin my-rolloutplugin -n default -o jsonpath='{.metadata.annotations}'
   ```

### Webhook receiver DNS errors?

If you see errors like `no such host` for `webhook-receiver.argo-rollouts.svc.cluster.local`:

1. **Ensure webhook receiver is deployed:**
   ```bash
   kubectl apply -f test/webhook-receiver.yaml
   ```

2. **Verify namespace matches:**
   - Webhook receiver YAML uses namespace: `argo-rollouts`
   - Notification ConfigMap service URL should match: `http://webhook-receiver.argo-rollouts.svc.cluster.local:8080`
   - If your rollouts controller is in a different namespace, update the webhook-receiver.yaml

3. **Check if pod is ready:**
   ```bash
   kubectl get pods -n argo-rollouts -l app=webhook-receiver -o wide
   ```

4. **Verify service has valid selector:**
   ```bash
   # Check service selector matches pod labels
   kubectl describe svc -n argo-rollouts webhook-receiver
   ```

### Template errors (like "can't print {{.time.Now}}")?

The templates use `{{call .time.Now}}` not `{{.time.Now}}` - ensure all templates use the `call` keyword for functions.

### See notification errors in controller logs

Search for notification-related errors:
```bash
kubectl logs -n argo-rollouts -l app.kubernetes.io/name=argo-rollouts | grep -i notification
```

## Advanced: Test with Different Services

You can also test with other notification services by adding them to the ConfigMap:

### Slack (for real testing)

```yaml
data:
  service.slack: |
    token: $slack-token
```

### Email

```yaml
data:
  service.email.gmail: |
    host: smtp.gmail.com
    port: 587
    username: $email-username
    password: $email-password
    from: $email-username
```

But for quick testing, the webhook receiver is the easiest!

---

## Event Coverage Summary

This guide covers **all 10 RolloutPlugin notification event types**:

| # | Event Type | Event Reason | Trigger | Severity |
|---|------------|--------------|---------|----------|
| 1 | **Progressing** | `RolloutPluginProgressingReason` | Rollout started/updated | Info |
| 2 | **Completed** | `RolloutPluginCompletedReason` | Rollout successfully finished | Info |
| 3 | **Paused** | `RolloutPluginPausedReason` | Manual pause or step pause | Info |
| 4 | **Resumed** | `RolloutPluginResumed` | Manual resume or step completed | Info |
| 5 | **Aborted** | `RolloutPluginAbortedReason` | Manual abort or timeout abort | Warning |
| 6 | **Timed Out** | `RolloutPluginTimedOutReason` | Progress deadline exceeded | Warning |
| 7 | **Invalid Spec** | `RolloutPluginInvalidSpecReason` | Spec validation failed | Warning |
| 8 | **Deleted** | `RolloutPluginDeleted` | RolloutPlugin deleted | Info |
| 9 | **Step Pause** | `RolloutPluginPausedReason` | Pause step started | Info |
| 10 | **Step Resume** | `RolloutPluginResumed` | Pause step completed | Info |

### Quick Test Checklist

- [ ] 1. Deploy webhook receiver
- [ ] 2. Configure notification ConfigMap with all templates
- [ ] 3. Subscribe RolloutPlugin with annotations
- [ ] 4. Watch webhook logs
- [ ] 5. Test Progressing (create/update RolloutPlugin)
- [ ] 6. Test Completion (full promotion or wait)
- [ ] 7. Test Manual Pause (set spec.paused=true)
- [ ] 8. Test Manual Resume (set spec.paused=false)
- [ ] 9. Test Abort (set status.abort=true)
- [ ] 10. Test Timeout (short progressDeadlineSeconds)
- [ ] 11. Test Invalid Spec (empty plugin name)
- [ ] 12. Test Deletion (kubectl delete)
- [ ] 13. Test Step Pause (pause step in strategy)
- [ ] 14. Test Step Resume (wait for pause duration)

### Testing Tips

1. **Start Simple**: Test one or two events first before subscribing to all
2. **Use Filters**: Add conditions in triggers to reduce noise (e.g., only on failures)
3. **Check Logs**: Always watch webhook receiver logs to verify payloads
4. **Verify Templates**: Use `analysisRuns` variable to include analysis results
5. **Test Timing**: Step pauses are great for testing pause/resume without manual intervention


