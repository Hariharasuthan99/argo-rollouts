package rolloutplugin

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/conditions"
)

type fakeCanaryPlugin struct {
	verifyWeightResult bool
	setWeightCalls     int
	abortCalls         int
	promoteCalls       int
}

func (f *fakeCanaryPlugin) Init() error {
	return nil
}

func (f *fakeCanaryPlugin) GetResourceStatus(_ context.Context, _ v1alpha1.WorkloadRef) (*ResourceStatus, error) {
	return &ResourceStatus{}, nil
}

func (f *fakeCanaryPlugin) SetWeight(_ context.Context, _ v1alpha1.WorkloadRef, _ int32) error {
	f.setWeightCalls++
	return nil
}

func (f *fakeCanaryPlugin) VerifyWeight(_ context.Context, _ v1alpha1.WorkloadRef, _ int32) (bool, error) {
	return f.verifyWeightResult, nil
}

func (f *fakeCanaryPlugin) Promote(_ context.Context, _ v1alpha1.WorkloadRef) error {
	f.promoteCalls++
	return nil
}

func (f *fakeCanaryPlugin) Abort(_ context.Context, _ v1alpha1.WorkloadRef) error {
	f.abortCalls++
	return nil
}

func (f *fakeCanaryPlugin) Restart(_ context.Context, _ v1alpha1.WorkloadRef) error {
	return nil
}

func TestFindRolloutPluginsForWorkload(t *testing.T) {
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "ns"}}
	matching := &v1alpha1.RolloutPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "rp-match", Namespace: "ns"},
		Spec: v1alpha1.RolloutPluginSpec{
			WorkloadRef: v1alpha1.WorkloadRef{
				Kind: "StatefulSet",
				Name: "workload",
			},
		},
	}
	nonMatching := &v1alpha1.RolloutPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "rp-no", Namespace: "ns"},
		Spec: v1alpha1.RolloutPluginSpec{
			WorkloadRef: v1alpha1.WorkloadRef{
				Kind: "StatefulSet",
				Name: "other",
			},
		},
	}
	otherNamespace := &v1alpha1.RolloutPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "rp-other-ns", Namespace: "other"},
		Spec: v1alpha1.RolloutPluginSpec{
			WorkloadRef: v1alpha1.WorkloadRef{
				Kind: "StatefulSet",
				Name: "workload",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	client := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(matching, nonMatching, otherNamespace).Build()
	reconciler := &RolloutPluginReconciler{Client: client}

	reqs := reconciler.findRolloutPluginsForWorkload(context.Background(), sts)
	assert.Len(t, reqs, 1)
	assert.Equal(t, ctrlclient.ObjectKey{Namespace: "ns", Name: "rp-match"}, reqs[0].NamespacedName)
}

func TestCheckPausedConditions_SetsPausedAndProgressingWhenPaused(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	logCtx := log.NewEntry(log.New())

	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{Paused: true},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{
		Abort:             true,
		RolloutInProgress: true,
		Conditions: []v1alpha1.RolloutPluginCondition{*conditions.NewRolloutPluginCondition(
			conditions.RolloutPluginProgressing,
			corev1.ConditionTrue,
			conditions.RolloutPluginProgressingReason,
			conditions.RolloutPluginProgressingMessage,
		)},
	}

	err := reconciler.checkPausedConditions(context.Background(), rolloutPlugin, newStatus, logCtx)
	assert.NoError(t, err)

	prog := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginProgressing)
	assert.NotNil(t, prog)
	assert.Equal(t, corev1.ConditionUnknown, prog.Status)
	assert.Equal(t, conditions.RolloutPluginPausedReason, prog.Reason)

	paused := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginPaused)
	assert.NotNil(t, paused)
	assert.Equal(t, corev1.ConditionTrue, paused.Status)
	assert.Equal(t, conditions.RolloutPluginPausedReason, paused.Reason)
}

func TestCheckPausedConditions_ResetsProgressingWhenResumed(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	logCtx := log.NewEntry(log.New())

	rolloutPlugin := &v1alpha1.RolloutPlugin{}
	newStatus := &v1alpha1.RolloutPluginStatus{
		RolloutInProgress: true,
		Conditions: []v1alpha1.RolloutPluginCondition{
			*conditions.NewRolloutPluginCondition(
				conditions.RolloutPluginProgressing,
				corev1.ConditionUnknown,
				conditions.RolloutPluginPausedReason,
				conditions.RolloutPluginPausedMessage,
			),
			*conditions.NewRolloutPluginCondition(
				conditions.RolloutPluginPaused,
				corev1.ConditionTrue,
				conditions.RolloutPluginPausedReason,
				conditions.RolloutPluginPausedMessage,
			),
		},
	}

	err := reconciler.checkPausedConditions(context.Background(), rolloutPlugin, newStatus, logCtx)
	assert.NoError(t, err)

	prog := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginProgressing)
	assert.NotNil(t, prog)
	assert.Equal(t, corev1.ConditionUnknown, prog.Status)
	assert.Equal(t, conditions.RolloutPluginProgressingReason, prog.Reason)
	assert.Equal(t, "RolloutPlugin resumed", prog.Message)

	paused := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginPaused)
	assert.NotNil(t, paused)
	assert.Equal(t, corev1.ConditionFalse, paused.Status)
	assert.Equal(t, conditions.RolloutPluginPausedReason, paused.Reason)
}

func TestCheckPausedConditions_DoesNotUpdateWhenAborted(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	logCtx := log.NewEntry(log.New())

	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{Paused: true},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{
		Abort:             true,
		RolloutInProgress: true,
		Conditions: []v1alpha1.RolloutPluginCondition{*conditions.NewRolloutPluginCondition(
			conditions.RolloutPluginProgressing,
			corev1.ConditionFalse,
			conditions.RolloutPluginAbortedReason,
			conditions.RolloutPluginAbortedMessage,
		)},
	}

	err := reconciler.checkPausedConditions(context.Background(), rolloutPlugin, newStatus, logCtx)
	assert.NoError(t, err)

	prog := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginProgressing)
	assert.NotNil(t, prog)
	assert.Equal(t, conditions.RolloutPluginAbortedReason, prog.Reason)

	paused := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginPaused)
	assert.Nil(t, paused)
}

func TestProcessCanaryRollout_SetWeightNotVerified(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{verifyWeightResult: false}
	logCtx := log.NewEntry(log.New())

	weight := int32(20)
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{{SetWeight: &weight}},
				},
			},
		},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{RolloutInProgress: true}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
	assert.False(t, result.Requeue)
	assert.Equal(t, "Waiting for weight 20 to be verified", newStatus.Message)
	if assert.NotNil(t, newStatus.CurrentStepIndex) {
		assert.Equal(t, int32(0), *newStatus.CurrentStepIndex)
	}
	assert.Equal(t, 1, plugin.setWeightCalls)
}

func TestProcessCanaryRollout_SetWeightVerifiedMovesToPause(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{verifyWeightResult: true}
	logCtx := log.NewEntry(log.New())

	weight := int32(30)
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{
						{SetWeight: &weight},
						{Pause: &v1alpha1.RolloutPause{Duration: v1alpha1.DurationFromString("10s")}},
					},
				},
			},
		},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{RolloutInProgress: true}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
	if assert.NotNil(t, newStatus.CurrentStepIndex) {
		assert.Equal(t, int32(1), *newStatus.CurrentStepIndex)
	}
	assert.True(t, newStatus.ControllerPause)
	assert.NotNil(t, newStatus.PauseStartTime)
	assert.Equal(t, "Paused", newStatus.Message)
	assert.Equal(t, 1, plugin.setWeightCalls)
}

func TestProcessCanaryRollout_PauseDurationElapsedAdvancesStep(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{}
	logCtx := log.NewEntry(log.New())

	stepIndex := int32(0)
	started := metav1.NewTime(time.Now().Add(-2 * time.Second))
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{{Pause: &v1alpha1.RolloutPause{Duration: v1alpha1.DurationFromString("1s")}}},
				},
			},
		},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{
		RolloutInProgress: true,
		CurrentStepIndex:  &stepIndex,
		ControllerPause:   true,
		PauseStartTime:    &started,
	}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
	if assert.NotNil(t, newStatus.CurrentStepIndex) {
		assert.Equal(t, int32(1), *newStatus.CurrentStepIndex)
	}
	assert.False(t, newStatus.ControllerPause)
	assert.Nil(t, newStatus.PauseStartTime)
}

func TestProcessCanaryRollout_AnalysisFailedAborts(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{}
	logCtx := log.NewEntry(log.New())

	stepIndex := int32(0)
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{{Analysis: &v1alpha1.RolloutAnalysis{}}},
				},
			},
		},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{
		RolloutInProgress: true,
		CurrentStepIndex:  &stepIndex,
		UpdatedRevision:   "rev-2",
		Canary: v1alpha1.CanaryStatus{
			CurrentStepAnalysisRunStatus: &v1alpha1.RolloutAnalysisRunStatus{
				Name:   "analysis-1",
				Status: v1alpha1.AnalysisPhaseFailed,
			},
		},
	}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, "Failed", newStatus.Phase)
	assert.True(t, newStatus.Aborted)
	assert.False(t, newStatus.RolloutInProgress)
	assert.Equal(t, "rev-2", newStatus.AbortedRevision)
	assert.Equal(t, 1, plugin.abortCalls)
}

func TestProcessCanaryRollout_AnalysisSuccessfulMovesToNextStep(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{}
	logCtx := log.NewEntry(log.New())

	stepIndex := int32(0)
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{
						{Analysis: &v1alpha1.RolloutAnalysis{}},
						{},
					},
				},
			},
		},
	}
	newStatus := &v1alpha1.RolloutPluginStatus{
		RolloutInProgress: true,
		CurrentStepIndex:  &stepIndex,
		Canary: v1alpha1.CanaryStatus{
			CurrentStepAnalysisRunStatus: &v1alpha1.RolloutAnalysisRunStatus{
				Name:   "analysis-2",
				Status: v1alpha1.AnalysisPhaseSuccessful,
			},
		},
	}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
	if assert.NotNil(t, newStatus.CurrentStepIndex) {
		assert.Equal(t, int32(1), *newStatus.CurrentStepIndex)
	}
	assert.Equal(t, "Analysis successful", newStatus.Message)
	assert.False(t, newStatus.ControllerPause)
}

func TestProcessCanaryRollout_PromoteFullCompletesRollout(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	plugin := &fakeCanaryPlugin{}
	logCtx := log.NewEntry(log.New())

	weight := int32(10)
	rolloutPlugin := &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			Strategy: v1alpha1.RolloutPluginStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					Steps: []v1alpha1.CanaryStep{{SetWeight: &weight}},
				},
			},
		},
	}
	pausedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	newStatus := &v1alpha1.RolloutPluginStatus{
		RolloutInProgress: true,
		PromoteFull:       true,
		ControllerPause:   true,
		PauseStartTime:    &pausedAt,
		Conditions: []v1alpha1.RolloutPluginCondition{*conditions.NewRolloutPluginCondition(
			conditions.RolloutPluginProgressing,
			corev1.ConditionTrue,
			conditions.RolloutPluginProgressingReason,
			conditions.RolloutPluginProgressingMessage,
		)},
	}

	result, err := reconciler.processCanaryRollout(context.Background(), rolloutPlugin, newStatus, plugin, v1alpha1.WorkloadRef{}, logCtx)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
	assert.Equal(t, 1, plugin.promoteCalls)
	assert.Equal(t, "Progressing", newStatus.Phase)
	assert.Equal(t, "Full promotion in progress, waiting for pods to converge", newStatus.Message)
	assert.True(t, newStatus.RolloutInProgress)
	assert.False(t, newStatus.PromoteFull)
	assert.False(t, newStatus.ControllerPause)
	assert.Nil(t, newStatus.PauseStartTime)
	if assert.NotNil(t, newStatus.CurrentStepIndex) {
		assert.Equal(t, int32(len(rolloutPlugin.Spec.Strategy.Canary.Steps)), *newStatus.CurrentStepIndex)
	}
	assert.NotNil(t, conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginProgressing))
	completed := conditions.GetRolloutPluginCondition(*newStatus, conditions.RolloutPluginCompleted)
	assert.Nil(t, completed)
}
