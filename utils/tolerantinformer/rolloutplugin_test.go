package tolerantinformer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	testutil "github.com/argoproj/argo-rollouts/test/util"
)

func newFakeRolloutPluginDynamicInformer(objs ...runtime.Object) dynamicinformer.DynamicSharedInformerFactory {
	scheme := runtime.NewScheme()
	listMapping := map[schema.GroupVersionResource]string{
		v1alpha1.RolloutPluginGVR: "RolloutPluginList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listMapping, objs...)
	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	dynamicInformerFactory.ForResource(v1alpha1.RolloutPluginGVR)

	stopCh := make(chan struct{})
	dynamicInformerFactory.Start(stopCh)
	synced := dynamicInformerFactory.WaitForCacheSync(stopCh)
	close(stopCh)

	if len(synced) != 1 {
		panic("could not sync fake rolloutplugin informer")
	}
	for gvr, isSynced := range synced {
		if !isSynced {
			panic(fmt.Sprintf("could not sync %v", gvr))
		}
	}
	return dynamicInformerFactory
}

func TestTolerantRolloutPluginInformer(t *testing.T) {
	good := testutil.ObjectFromYAML(`
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: rolloutplugin-good
  namespace: default
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: test-sts
  plugin:
    name: statefulset
  strategy:
    canary:
      steps:
      - setWeight: 50
`)

	bad := testutil.ObjectFromYAML(`
apiVersion: argoproj.io/v1alpha1
kind: RolloutPlugin
metadata:
  name: rolloutplugin-malformed
  namespace: dummy-namespace
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: test-sts
  plugin:
    name: statefulset
  progressDeadlineSeconds: "not-a-number"
  strategy:
    canary:
      steps:
      - setWeight: 20
`)

	dynInformerFactory := newFakeRolloutPluginDynamicInformer(good, bad)
	informer := NewTolerantRolloutPluginInformer(dynInformerFactory)

	verify := func(rp *v1alpha1.RolloutPlugin) {
		assert.Equal(t, "statefulset", rp.Spec.Plugin.Name)
		assert.Equal(t, "StatefulSet", rp.Spec.WorkloadRef.Kind)
		assert.Equal(t, "test-sts", rp.Spec.WorkloadRef.Name)
		assert.NotNil(t, rp.Spec.Strategy.Canary)
	}

	list, err := informer.Lister().List(labels.NewSelector())
	assert.NoError(t, err)
	assert.Len(t, list, 2)
	for _, obj := range list {
		if obj.Name == "rolloutplugin-malformed" {
			verify(obj)
		}
	}

	obj, err := informer.Lister().RolloutPlugins("dummy-namespace").Get("rolloutplugin-malformed")
	assert.NoError(t, err)
	verify(obj)

	list, err = informer.Lister().RolloutPlugins("dummy-namespace").List(labels.NewSelector())
	assert.NoError(t, err)
	assert.Len(t, list, 1)
	verify(list[0])
}
