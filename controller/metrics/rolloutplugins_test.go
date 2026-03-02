package metrics

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	logutil "github.com/argoproj/argo-rollouts/utils/log"
)

func TestCalculateRolloutPluginPhase(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		want  RolloutPluginPhase
	}{
		{name: "empty defaults to progressing", phase: "", want: RolloutPluginProgressing},
		{name: "progressing", phase: "Progressing", want: RolloutPluginProgressing},
		{name: "paused", phase: "Paused", want: RolloutPluginPaused},
		{name: "healthy", phase: "Healthy", want: RolloutPluginHealthy},
		{name: "successful maps healthy", phase: "Successful", want: RolloutPluginHealthy},
		{name: "completed maps healthy", phase: "Completed", want: RolloutPluginHealthy},
		{name: "degraded", phase: "Degraded", want: RolloutPluginDegraded},
		{name: "error", phase: "Error", want: RolloutPluginError},
		{name: "failed maps error", phase: "Failed", want: RolloutPluginError},
		{name: "timeout", phase: "Timeout", want: RolloutPluginTimeout},
		{name: "timedout maps timeout", phase: "TimedOut", want: RolloutPluginTimeout},
		{name: "aborted", phase: "Aborted", want: RolloutPluginAborted},
		{name: "unknown defaults progressing", phase: "Whatever", want: RolloutPluginProgressing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &v1alpha1.RolloutPlugin{Status: v1alpha1.RolloutPluginStatus{Phase: tt.phase}}
			assert.Equal(t, tt.want, calculateRolloutPluginPhase(rp))
		})
	}
}

func TestGetRolloutPluginStrategyType(t *testing.T) {
	canaryRP := &v1alpha1.RolloutPlugin{Spec: v1alpha1.RolloutPluginSpec{Strategy: v1alpha1.RolloutPluginStrategy{Canary: &v1alpha1.CanaryStrategy{}}}}
	noneRP := &v1alpha1.RolloutPlugin{Spec: v1alpha1.RolloutPluginSpec{Strategy: v1alpha1.RolloutPluginStrategy{}}}

	assert.Equal(t, "Canary", getRolloutPluginStrategyType(canaryRP))
	assert.Equal(t, "none", getRolloutPluginStrategyType(noneRP))
}

func TestCollectRolloutPlugins(t *testing.T) {
	rp := &v1alpha1.RolloutPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "rp-sample", Namespace: "argo-rollouts"},
		Spec: v1alpha1.RolloutPluginSpec{
			Plugin:      v1alpha1.PluginConfig{Name: "statefulset"},
			WorkloadRef: v1alpha1.WorkloadRef{Kind: "StatefulSet"},
			Strategy:    v1alpha1.RolloutPluginStrategy{Canary: &v1alpha1.CanaryStrategy{}},
		},
		Status: v1alpha1.RolloutPluginStatus{
			Phase:           "Progressing",
			Replicas:        5,
			UpdatedReplicas: 2,
			ReadyReplicas:   4,
		},
	}

	expected := `
# HELP rolloutplugin_info Information about rolloutplugin.
# TYPE rolloutplugin_info gauge
rolloutplugin_info{name="rp-sample",namespace="argo-rollouts",phase="Progressing",plugin_name="statefulset",strategy="Canary",workload_kind="StatefulSet"} 1
# HELP rolloutplugin_workload_replicas_desired The number of desired replicas for the workload managed by rolloutplugin.
# TYPE rolloutplugin_workload_replicas_desired gauge
rolloutplugin_workload_replicas_desired{name="rp-sample",namespace="argo-rollouts"} 5
# HELP rolloutplugin_workload_replicas_ready The number of ready replicas for the workload managed by rolloutplugin.
# TYPE rolloutplugin_workload_replicas_ready gauge
rolloutplugin_workload_replicas_ready{name="rp-sample",namespace="argo-rollouts"} 4
# HELP rolloutplugin_workload_replicas_updated The number of updated replicas for the workload managed by rolloutplugin.
# TYPE rolloutplugin_workload_replicas_updated gauge
rolloutplugin_workload_replicas_updated{name="rp-sample",namespace="argo-rollouts"} 2`

	registry := prometheus.NewRegistry()
	config := newFakeServerConfig(rp)
	registry.MustRegister(NewRolloutPluginCollector(config.RolloutPluginLister))
	mux := http.NewServeMux()
	mux.Handle(MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	testHttpResponse(t, mux, expected, assert.Contains)
}

func TestIncRolloutPluginReconcile(t *testing.T) {
	expected := `
# HELP rolloutplugin_reconcile RolloutPlugin reconciliation performance.
# TYPE rolloutplugin_reconcile histogram
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="0.01"} 1
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="0.15"} 1
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="0.25"} 1
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="0.5"} 1
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="1"} 1
rolloutplugin_reconcile_bucket{name="rp-test",namespace="rp-ns",le="+Inf"} 1
rolloutplugin_reconcile_sum{name="rp-test",namespace="rp-ns"} 0.001
rolloutplugin_reconcile_count{name="rp-test",namespace="rp-ns"} 1`

	metricsServ := NewMetricsServer(newFakeServerConfig())
	rp := &v1alpha1.RolloutPlugin{ObjectMeta: metav1.ObjectMeta{Name: "rp-test", Namespace: "rp-ns"}}
	metricsServ.IncRolloutPluginReconcile(rp, time.Millisecond)
	testHttpResponse(t, metricsServ.Handler, expected, assert.Contains)
}

func TestIncErrorAndRemoveRolloutPlugin(t *testing.T) {
	expected := `rolloutplugin_reconcile_error{name="rp-name",namespace="rp-ns"} 1`

	metricsServ := NewMetricsServer(newFakeServerConfig())
	metricsServ.IncError("rp-ns", "rp-name", logutil.RolloutPluginKey)
	testHttpResponse(t, metricsServ.Handler, expected, assert.Contains)

	metricsServ.Remove("rp-ns", "rp-name", logutil.RolloutPluginKey)
	time.Sleep(2 * time.Second)
	testHttpResponse(t, metricsServ.Handler, expected, assert.NotContains)
}
