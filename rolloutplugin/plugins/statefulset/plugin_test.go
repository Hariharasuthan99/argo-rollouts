package statefulset

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
)

func int32Ptr(v int32) *int32 {
	return &v
}

func newPluginWithStatefulSet(sts *appsv1.StatefulSet) *Plugin {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	client := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	return &Plugin{
		logCtx: log.New().WithField("test", "statefulset"),
		client: client,
	}
}

func TestGetResourceStatusCalculations(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "ns"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(5),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: int32Ptr(2),
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   5,
			UpdatedReplicas: 3,
			CurrentRevision: "rev1",
			UpdateRevision:  "rev2",
		},
	}

	plugin := newPluginWithStatefulSet(sts)
	status, err := plugin.GetResourceStatus(context.Background(), v1alpha1.WorkloadRef{Name: "my-sts", Namespace: "ns"})
	assert.NoError(t, err)
	assert.Equal(t, int32(5), status.Replicas)
	assert.Equal(t, int32(3), status.UpdatedReplicas)
	assert.Equal(t, int32(5), status.ReadyReplicas)
	assert.Equal(t, "rev1", status.CurrentRevision)
	assert.Equal(t, "rev2", status.UpdatedRevision)
	assert.True(t, status.Ready)
}

func TestVerifyWeightPartitionMismatch(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "ns"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(5),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: int32Ptr(4),
				},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 5, UpdatedReplicas: 1},
	}

	plugin := newPluginWithStatefulSet(sts)
	verified, err := plugin.VerifyWeight(context.Background(), v1alpha1.WorkloadRef{Name: "my-sts", Namespace: "ns"}, 40)
	assert.NoError(t, err)
	assert.False(t, verified)
}

func TestVerifyWeightRequiresReadyAndUpdatedReplicas(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "ns"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(5),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: int32Ptr(3),
				},
			},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 5, UpdatedReplicas: 1},
	}

	plugin := newPluginWithStatefulSet(sts)
	verified, err := plugin.VerifyWeight(context.Background(), v1alpha1.WorkloadRef{Name: "my-sts", Namespace: "ns"}, 40)
	assert.NoError(t, err)
	assert.False(t, verified)

	// Update status to meet expectations
	sts.Status.UpdatedReplicas = 2
	sts.Status.ReadyReplicas = 5
	_ = plugin.client.Status().Update(context.Background(), sts)

	verified, err = plugin.VerifyWeight(context.Background(), v1alpha1.WorkloadRef{Name: "my-sts", Namespace: "ns"}, 40)
	assert.NoError(t, err)
	assert.True(t, verified)
}

func TestGetResourceStatusRequiresNamespace(t *testing.T) {
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "ns"}}
	plugin := newPluginWithStatefulSet(sts)

	_, err := plugin.GetResourceStatus(context.Background(), v1alpha1.WorkloadRef{Name: "my-sts"})
	assert.EqualError(t, err, "namespace is required in workloadRef")
}
