package rolloutplugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
)

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

func TestMeetsMinReadySeconds(t *testing.T) {
	status := &ResourceStatus{ReadyReplicas: 3, AvailableReplicas: 3}
	assert.True(t, meetsMinReadySeconds(status, 0))
	assert.True(t, meetsMinReadySeconds(status, 10))

	status.AvailableReplicas = 2
	assert.False(t, meetsMinReadySeconds(status, 10))
}
