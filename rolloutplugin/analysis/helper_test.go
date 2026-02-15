package analysis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	informers "github.com/argoproj/argo-rollouts/pkg/client/informers/externalversions"
	"github.com/argoproj/argo-rollouts/utils/annotations"
)

func TestGetAnalysisRunsForOwnerIncludesStatusRefs(t *testing.T) {
	ctx := context.Background()
	ownerUID := types.UID("owner-uid")

	ar1 := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ar-1",
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{{
				UID:        ownerUID,
				Controller: ptr.To(true),
			}},
		},
	}
	ar2 := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ar-2",
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{{
				UID:        ownerUID,
				Controller: ptr.To(true),
			}},
		},
	}

	client := fake.NewSimpleClientset(ar1, ar2)
	factory := informers.NewSharedInformerFactory(client, 0)

	arInformer := factory.Argoproj().V1alpha1().AnalysisRuns().Informer()
	assert.NoError(t, arInformer.GetIndexer().Add(ar1))

	helper := NewHelper(
		client,
		factory.Argoproj().V1alpha1().AnalysisRuns().Lister(),
		factory.Argoproj().V1alpha1().AnalysisTemplates().Lister(),
		factory.Argoproj().V1alpha1().ClusterAnalysisTemplates().Lister(),
	)

	statusRefs := []v1alpha1.RolloutAnalysisRunStatus{{Name: "ar-2"}}
	ars, err := helper.GetAnalysisRunsForOwner(ctx, "rp", metav1.NamespaceDefault, ownerUID, statusRefs)
	assert.NoError(t, err)
	assert.Len(t, ars, 2)

	names := map[string]bool{}
	for _, ar := range ars {
		names[ar.Name] = true
	}
	assert.True(t, names["ar-1"])
	assert.True(t, names["ar-2"])
}

func TestCreateAnalysisRunMergesMetadataAndCreatesRun(t *testing.T) {
	ctx := context.Background()

	template := &v1alpha1.AnalysisTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tmpl",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1alpha1.AnalysisTemplateSpec{
			Metrics: []v1alpha1.Metric{{Name: "metric"}},
		},
	}

	client := fake.NewSimpleClientset(template)
	factory := informers.NewSharedInformerFactory(client, 0)
	tmplInformer := factory.Argoproj().V1alpha1().AnalysisTemplates().Informer()
	assert.NoError(t, tmplInformer.GetIndexer().Add(template))

	helper := NewHelper(
		client,
		factory.Argoproj().V1alpha1().AnalysisRuns().Lister(),
		factory.Argoproj().V1alpha1().AnalysisTemplates().Lister(),
		factory.Argoproj().V1alpha1().ClusterAnalysisTemplates().Lister(),
	)

	rolloutAnalysis := &v1alpha1.RolloutAnalysis{
		Templates: []v1alpha1.AnalysisTemplateRef{{TemplateName: "tmpl"}},
		AnalysisRunMetadata: &v1alpha1.AnalysisRunMetadata{
			Annotations: map[string]string{"from": "analysis"},
		},
	}
	labels := map[string]string{"l1": "v1"}
	annotations := map[string]string{"a1": "v1"}
	ownerRef := metav1.OwnerReference{UID: types.UID("owner"), Name: "rp", Controller: ptr.To(true)}

	created, err := helper.CreateAnalysisRun(ctx, rolloutAnalysis, nil, metav1.NamespaceDefault, "abcd", "step", labels, annotations, ownerRef)
	assert.NoError(t, err)
	assert.Equal(t, "rp-step-abcd", created.Name)
	assert.Len(t, created.OwnerReferences, 1)
	assert.Equal(t, ownerRef.UID, created.OwnerReferences[0].UID)
	assert.Equal(t, "v1", created.Labels["l1"])
	assert.Equal(t, "analysis", created.Annotations["from"])
	assert.Equal(t, "v1", created.Annotations["a1"])
}

func TestCreateAnalysisRunNoTemplatesReturnsError(t *testing.T) {
	ctx := context.Background()

	client := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(client, 0)
	helper := NewHelper(
		client,
		factory.Argoproj().V1alpha1().AnalysisRuns().Lister(),
		factory.Argoproj().V1alpha1().AnalysisTemplates().Lister(),
		factory.Argoproj().V1alpha1().ClusterAnalysisTemplates().Lister(),
	)

	rolloutAnalysis := &v1alpha1.RolloutAnalysis{}
	_, err := helper.CreateAnalysisRun(ctx, rolloutAnalysis, nil, metav1.NamespaceDefault, "abcd", "step", nil, nil, metav1.OwnerReference{Name: "rp"})
	assert.Error(t, err)
	assert.EqualError(t, err, "no templates found")
}

func TestCancelAnalysisRunsSkipsCompleted(t *testing.T) {
	ctx := context.Background()

	running := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: metav1.NamespaceDefault},
		Status:     v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseRunning},
	}
	completed := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: metav1.NamespaceDefault},
		Status:     v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseSuccessful},
	}

	client := fake.NewSimpleClientset(running, completed)
	factory := informers.NewSharedInformerFactory(client, 0)
	helper := NewHelper(
		client,
		factory.Argoproj().V1alpha1().AnalysisRuns().Lister(),
		factory.Argoproj().V1alpha1().AnalysisTemplates().Lister(),
		factory.Argoproj().V1alpha1().ClusterAnalysisTemplates().Lister(),
	)

	patchCount := 0
	client.PrependReactor("patch", "analysisruns", func(action kubetesting.Action) (bool, runtime.Object, error) {
		patchCount++
		return true, running, nil
	})

	err := helper.CancelAnalysisRuns(ctx, []*v1alpha1.AnalysisRun{running, completed})
	assert.NoError(t, err)
	assert.Equal(t, 1, patchCount)
}

func TestDeleteAnalysisRunsHonorsLimit(t *testing.T) {
	ctx := context.Background()

	ar1 := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-1", Namespace: metav1.NamespaceDefault}}
	ar2 := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-2", Namespace: metav1.NamespaceDefault}}
	ar3 := &v1alpha1.AnalysisRun{ObjectMeta: metav1.ObjectMeta{Name: "ar-3", Namespace: metav1.NamespaceDefault}}

	client := fake.NewSimpleClientset(ar1, ar2, ar3)
	factory := informers.NewSharedInformerFactory(client, 0)
	arInformer := factory.Argoproj().V1alpha1().AnalysisRuns().Informer()
	assert.NoError(t, arInformer.GetIndexer().Add(ar1))
	assert.NoError(t, arInformer.GetIndexer().Add(ar2))
	assert.NoError(t, arInformer.GetIndexer().Add(ar3))

	helper := NewHelper(
		client,
		factory.Argoproj().V1alpha1().AnalysisRuns().Lister(),
		factory.Argoproj().V1alpha1().AnalysisTemplates().Lister(),
		factory.Argoproj().V1alpha1().ClusterAnalysisTemplates().Lister(),
	)

	deleted := []string{}
	client.PrependReactor("delete", "analysisruns", func(action kubetesting.Action) (bool, runtime.Object, error) {
		deleteAction, ok := action.(kubetesting.DeleteAction)
		if ok {
			deleted = append(deleted, deleteAction.GetName())
		}
		return true, nil, nil
	})

	err := helper.DeleteAnalysisRuns(ctx, metav1.NamespaceDefault, labels.Everything(), 1)
	assert.NoError(t, err)
	assert.Len(t, deleted, 2)
	assert.Contains(t, deleted, "ar-1")
	assert.Contains(t, deleted, "ar-2")
}

func TestNeedsNewAnalysisRun(t *testing.T) {
	assert.True(t, NeedsNewAnalysisRun(nil, 1))

	completed := &v1alpha1.AnalysisRun{Status: v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseSuccessful}}
	assert.True(t, NeedsNewAnalysisRun(completed, 1))

	running := &v1alpha1.AnalysisRun{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.RevisionAnnotation: "1"}},
		Status:     v1alpha1.AnalysisRunStatus{Phase: v1alpha1.AnalysisPhaseRunning},
	}
	assert.False(t, NeedsNewAnalysisRun(running, 1))
	assert.True(t, NeedsNewAnalysisRun(running, 2))
}

func TestGetHistoryLimits(t *testing.T) {
	success, failure := GetHistoryLimits(nil)
	assert.Equal(t, int32(5), success)
	assert.Equal(t, int32(5), failure)

	successLimit := int32(2)
	failureLimit := int32(7)
	analysis := &v1alpha1.AnalysisRunStrategy{
		SuccessfulRunHistoryLimit:   &successLimit,
		UnsuccessfulRunHistoryLimit: &failureLimit,
	}
	success, failure = GetHistoryLimits(analysis)
	assert.Equal(t, int32(2), success)
	assert.Equal(t, int32(7), failure)
}
