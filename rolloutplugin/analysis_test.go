package rolloutplugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	analysisutil "github.com/argoproj/argo-rollouts/utils/analysis"
)

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
