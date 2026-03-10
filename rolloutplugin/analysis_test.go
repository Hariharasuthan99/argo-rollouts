package rolloutplugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubetesting "k8s.io/client-go/testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	fakeclientset "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	analysisutil "github.com/argoproj/argo-rollouts/utils/analysis"
)

func TestConvertAnalysisRunArgsToArguments(t *testing.T) {
	args := []v1alpha1.AnalysisRunArgument{
		{Name: "plain", Value: "v1"},
		{
			Name:  "field-ref",
			Value: "",
			ValueFrom: &v1alpha1.ArgumentValueFrom{
				FieldRef: &v1alpha1.FieldRef{FieldPath: "metadata.name"},
			},
		},
	}

	converted := convertAnalysisRunArgsToArguments(args)
	assert.Len(t, converted, 2)
	assert.Equal(t, "plain", converted[0].Name)
	if assert.NotNil(t, converted[0].Value) {
		assert.Equal(t, "v1", *converted[0].Value)
	}
	assert.Nil(t, converted[0].ValueFrom)

	assert.Equal(t, "field-ref", converted[1].Name)
	assert.NotNil(t, converted[1].ValueFrom)
	if assert.NotNil(t, converted[1].ValueFrom.FieldRef) {
		assert.Equal(t, "metadata.name", converted[1].ValueFrom.FieldRef.FieldPath)
	}
}

func TestFilterCurrentAnalysisRuns(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}

	stepAR := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "step-ar"}}
	bgAR := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "bg-ar"}}
	otherAR := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "other-ar"}}

	rp := &v1alpha1.RolloutPlugin{}
	rp.Status.Canary.CurrentStepAnalysisRunStatus = &v1alpha1.RolloutAnalysisRunStatus{Name: "step-ar"}
	rp.Status.Canary.CurrentBackgroundAnalysisRunStatus = &v1alpha1.RolloutAnalysisRunStatus{Name: "bg-ar"}

	current, others := reconciler.filterCurrentAnalysisRuns([]*v1alpha1.AnalysisRun{stepAR, bgAR, otherAR, nil}, rp)
	assert.Equal(t, stepAR, current.CanaryStep)
	assert.Equal(t, bgAR, current.CanaryBackground)
	assert.Len(t, others, 1)
	assert.Equal(t, otherAR, others[0])
}

func TestSetCurrentAnalysisRuns(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}
	rp := &v1alpha1.RolloutPlugin{}

	stepAR := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "step-ar"}, Status: v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseRunning}}
	bgAR := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "bg-ar"}, Status: v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseSuccessful}}

	reconciler.setCurrentAnalysisRuns(rp, analysisutil.CurrentAnalysisRuns{CanaryStep: stepAR, CanaryBackground: bgAR})
	assert.Equal(t, "step-ar", rp.Status.Canary.CurrentStepAnalysisRunStatus.Name)
	assert.Equal(t, v1alpha1.AnalysisPhaseRunning, rp.Status.Canary.CurrentStepAnalysisRunStatus.Status)
	assert.Equal(t, "bg-ar", rp.Status.Canary.CurrentBackgroundAnalysisRunStatus.Name)
	assert.Equal(t, v1alpha1.AnalysisPhaseSuccessful, rp.Status.Canary.CurrentBackgroundAnalysisRunStatus.Status)

	reconciler.setCurrentAnalysisRuns(rp, analysisutil.CurrentAnalysisRuns{})
	assert.Nil(t, rp.Status.Canary.CurrentStepAnalysisRunStatus)
	assert.Nil(t, rp.Status.Canary.CurrentBackgroundAnalysisRunStatus)
}

func TestReconcileStepBasedAnalysisRunReturnsCurrentWhenNoRetryConditions(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}

	stepIndex := int32(0)
	rp := &v1alpha1.RolloutPlugin{}
	rp.Name = "rp"
	rp.Status.CurrentStepIndex = &stepIndex
	rp.Spec.Strategy.Canary = &v1alpha1.CanaryStrategy{Steps: []v1alpha1.CanaryStep{{Analysis: &v1alpha1.RolloutAnalysis{}}}}

	currentAR := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{Name: "ar-existing"},
		Status:     v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseRunning},
	}

	got, err := reconciler.reconcileStepBasedAnalysisRun(context.Background(), rp, currentAR)
	assert.NoError(t, err)
	assert.Equal(t, currentAR, got)
}

func TestReconcileBackgroundAnalysisRunReturnsCurrentWhenNoRetryConditions(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}

	rp := &v1alpha1.RolloutPlugin{}
	rp.Name = "rp"
	rp.Spec.Strategy.Canary = &v1alpha1.CanaryStrategy{
		Analysis: &v1alpha1.RolloutAnalysisBackground{RolloutAnalysis: v1alpha1.RolloutAnalysis{}},
	}

	currentAR := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{Name: "ar-bg-existing"},
		Status:     v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseRunning},
	}

	got, err := reconciler.reconcileBackgroundAnalysisRun(context.Background(), rp, currentAR)
	assert.NoError(t, err)
	assert.Equal(t, currentAR, got)
}

func TestReconcileStepBasedAnalysisRunReturnsNilWhenNoCanaryOrInvalidStep(t *testing.T) {
	reconciler := &RolloutPluginReconciler{}

	rpNoCanary := &v1alpha1.RolloutPlugin{}
	got, err := reconciler.reconcileStepBasedAnalysisRun(context.Background(), rpNoCanary, nil)
	assert.NoError(t, err)
	assert.Nil(t, got)

	stepIndex := int32(5)
	rpOutOfRange := &v1alpha1.RolloutPlugin{}
	rpOutOfRange.Status.CurrentStepIndex = &stepIndex
	rpOutOfRange.Spec.Strategy.Canary = &v1alpha1.CanaryStrategy{Steps: []v1alpha1.CanaryStep{{}}}
	got, err = reconciler.reconcileStepBasedAnalysisRun(context.Background(), rpOutOfRange, nil)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestCreateAnalysisRunSetsOwnerAndLabels(t *testing.T) {
	template := &v1alpha1.AnalysisTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: metav1.NamespaceDefault},
		Spec: v1alpha1.AnalysisTemplateSpec{
			Metrics: []v1alpha1.Metric{{Name: "metric-a"}},
		},
	}

	client := fakeclientset.NewSimpleClientset(template)
	reconciler := &RolloutPluginReconciler{
		ArgoProjClientset: client,
		InstanceID:        "iid-1",
	}

	rp := &v1alpha1.RolloutPlugin{ObjectMeta: metav1.ObjectMeta{Name: "rp", Namespace: metav1.NamespaceDefault, UID: "rp-uid"}}
	analysisSpec := &v1alpha1.RolloutAnalysis{
		Templates: []v1alpha1.AnalysisTemplateRef{{TemplateName: "tmpl"}},
	}

	created, err := reconciler.createAnalysisRun(context.Background(), rp, analysisSpec, "rp-step-0", v1alpha1.RolloutTypeStepLabel)
	assert.NoError(t, err)
	assert.NotNil(t, created)
	assert.Equal(t, v1alpha1.RolloutTypeStepLabel, created.Labels[v1alpha1.RolloutTypeLabel])
	assert.Equal(t, "rp", created.Labels["rollout-plugin-name"])
	assert.Equal(t, "iid-1", created.Labels[v1alpha1.LabelKeyControllerInstanceID])
	if assert.Len(t, created.OwnerReferences, 1) {
		assert.Equal(t, "rp", created.OwnerReferences[0].Name)
		assert.Equal(t, "rp-uid", string(created.OwnerReferences[0].UID))
	}
}

func TestCancelAnalysisRunsSkipsTerminateAndIgnoresNotFound(t *testing.T) {
	ar1 := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-1", Namespace: metav1.NamespaceDefault}}
	arTerminated := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-term", Namespace: metav1.NamespaceDefault}, Spec: v1alpha1.AnalysisRunSpec{Terminate: true}}
	arMissing := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-missing", Namespace: metav1.NamespaceDefault}}

	client := fakeclientset.NewSimpleClientset(ar1)
	reconciler := &RolloutPluginReconciler{ArgoProjClientset: client}

	patched := map[string]int{}
	client.PrependReactor("patch", "analysisruns", func(action kubetesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(kubetesting.PatchAction)
		name := patchAction.GetName()
		patched[name]++
		if name == "ar-missing" {
			return true, nil, errors.NewNotFound(schema.GroupResource{Group: "argoproj.io", Resource: "analysisruns"}, name)
		}
		return true, ar1, nil
	})

	err := reconciler.cancelAnalysisRuns(context.Background(), &v1alpha1.RolloutPlugin{}, []*v1alpha1.AnalysisRun{ar1, arTerminated, arMissing})
	assert.NoError(t, err)
	assert.Equal(t, 1, patched["ar-1"])
	assert.Equal(t, 0, patched["ar-term"])
	assert.Equal(t, 1, patched["ar-missing"])
}

func TestDeleteAnalysisRunsIgnoresNotFound(t *testing.T) {
	ar1 := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-1", Namespace: metav1.NamespaceDefault}}
	arMissing := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-missing", Namespace: metav1.NamespaceDefault}}

	client := fakeclientset.NewSimpleClientset(ar1)
	reconciler := &RolloutPluginReconciler{ArgoProjClientset: client}

	deleted := map[string]int{}
	client.PrependReactor("delete", "analysisruns", func(action kubetesting.Action) (bool, runtime.Object, error) {
		deleteAction := action.(kubetesting.DeleteAction)
		name := deleteAction.GetName()
		deleted[name]++
		if name == "ar-missing" {
			return true, nil, errors.NewNotFound(schema.GroupResource{Group: "argoproj.io", Resource: "analysisruns"}, name)
		}
		return true, nil, nil
	})

	err := reconciler.deleteAnalysisRuns(context.Background(), []*v1alpha1.AnalysisRun{ar1, arMissing})
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted["ar-1"])
	assert.Equal(t, 1, deleted["ar-missing"])
}
