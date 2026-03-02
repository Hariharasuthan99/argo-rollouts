package conditions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	timeutil "github.com/argoproj/argo-rollouts/utils/time"
)

func validRolloutPlugin() *v1alpha1.RolloutPlugin {
	return &v1alpha1.RolloutPlugin{
		Spec: v1alpha1.RolloutPluginSpec{
			WorkloadRef: v1alpha1.WorkloadRef{
				Name:       "workload",
				Kind:       "StatefulSet",
				APIVersion: "apps/v1",
			},
			Plugin: v1alpha1.PluginConfig{Name: "plugin"},
			Strategy: v1alpha1.RolloutPluginStrategy{
				Type:   "Canary",
				Canary: &v1alpha1.CanaryStrategy{},
			},
		},
	}
}

func TestNewGetAndRemoveRolloutPluginCondition(t *testing.T) {
	condition := NewRolloutPluginCondition(RolloutPluginHealthy, corev1.ConditionTrue, RolloutPluginHealthyReason, RolloutPluginHealthyMessage)
	assert.Equal(t, RolloutPluginHealthy, condition.Type)
	assert.Equal(t, corev1.ConditionTrue, condition.Status)
	assert.Equal(t, RolloutPluginHealthyReason, condition.Reason)
	assert.Equal(t, RolloutPluginHealthyMessage, condition.Message)
	assert.False(t, condition.LastUpdateTime.IsZero())
	assert.False(t, condition.LastTransitionTime.IsZero())

	status := v1alpha1.RolloutPluginStatus{Conditions: []v1alpha1.RolloutPluginCondition{*condition}}
	fetched := GetRolloutPluginCondition(status, RolloutPluginHealthy)
	assert.NotNil(t, fetched)
	assert.Equal(t, RolloutPluginHealthy, fetched.Type)

	RemoveRolloutPluginCondition(&status, RolloutPluginHealthy)
	assert.Nil(t, GetRolloutPluginCondition(status, RolloutPluginHealthy))
}

func TestSetRolloutPluginCondition_NoUpdateWhenIdentical(t *testing.T) {
	now := metav1.Now()
	status := &v1alpha1.RolloutPluginStatus{
		Conditions: []v1alpha1.RolloutPluginCondition{{
			Type:               RolloutPluginProgressing,
			Status:             corev1.ConditionTrue,
			Reason:             RolloutPluginProgressingReason,
			Message:            RolloutPluginProgressingMessage,
			LastUpdateTime:     now,
			LastTransitionTime: now,
		}},
	}

	updated := SetRolloutPluginCondition(status, v1alpha1.RolloutPluginCondition{
		Type:               RolloutPluginProgressing,
		Status:             corev1.ConditionTrue,
		Reason:             RolloutPluginProgressingReason,
		Message:            RolloutPluginProgressingMessage,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	})

	assert.False(t, updated)
	assert.Len(t, status.Conditions, 1)
	assert.Equal(t, now, status.Conditions[0].LastTransitionTime)
}

func TestSetRolloutPluginCondition_PreservesTransitionWhenStatusUnchanged(t *testing.T) {
	oldTransition := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	status := &v1alpha1.RolloutPluginStatus{
		Conditions: []v1alpha1.RolloutPluginCondition{{
			Type:               RolloutPluginProgressing,
			Status:             corev1.ConditionTrue,
			Reason:             RolloutPluginProgressingReason,
			Message:            RolloutPluginProgressingMessage,
			LastTransitionTime: oldTransition,
		}},
	}

	updated := SetRolloutPluginCondition(status, v1alpha1.RolloutPluginCondition{
		Type:               RolloutPluginProgressing,
		Status:             corev1.ConditionTrue,
		Reason:             "NewReason",
		Message:            "New message",
		LastTransitionTime: metav1.Now(),
	})

	assert.True(t, updated)
	assert.Len(t, status.Conditions, 1)
	assert.Equal(t, oldTransition, status.Conditions[0].LastTransitionTime)
	assert.Equal(t, "NewReason", status.Conditions[0].Reason)
}

func TestRolloutPluginTimedOut(t *testing.T) {
	now := time.Date(2026, 3, 2, 0, 0, 30, 0, time.UTC)
	timeutil.SetNowTimeFunc(func() time.Time {
		return now
	})
	t.Cleanup(func() {
		timeutil.SetNowTimeFunc(time.Now)
	})

	progressDeadline := int32(10)
	basePlugin := validRolloutPlugin()
	basePlugin.Spec.ProgressDeadlineSeconds = &progressDeadline

	oldUpdate := metav1.NewTime(now.Add(-20 * time.Second))
	recentUpdate := metav1.NewTime(now.Add(-5 * time.Second))

	tests := []struct {
		name   string
		plugin *v1alpha1.RolloutPlugin
		status v1alpha1.RolloutPluginStatus
		want   bool
	}{
		{
			name:   "no progressing condition",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{},
			want:   false,
		},
		{
			name:   "already timed out condition",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{Conditions: []v1alpha1.RolloutPluginCondition{{
				Type:   RolloutPluginProgressing,
				Reason: RolloutPluginTimedOutReason,
			}}},
			want: true,
		},
		{
			name: "paused in spec does not timeout",
			plugin: &v1alpha1.RolloutPlugin{Spec: v1alpha1.RolloutPluginSpec{
				Paused:                  true,
				ProgressDeadlineSeconds: &progressDeadline,
			}},
			status: v1alpha1.RolloutPluginStatus{Conditions: []v1alpha1.RolloutPluginCondition{{
				Type:           RolloutPluginProgressing,
				LastUpdateTime: oldUpdate,
			}}},
			want: false,
		},
		{
			name:   "paused in status does not timeout",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{
				Paused: true,
				Conditions: []v1alpha1.RolloutPluginCondition{{
					Type:           RolloutPluginProgressing,
					LastUpdateTime: oldUpdate,
				}},
			},
			want: false,
		},
		{
			name:   "aborted does not timeout",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{
				Aborted: true,
				Conditions: []v1alpha1.RolloutPluginCondition{{
					Type:           RolloutPluginProgressing,
					LastUpdateTime: oldUpdate,
				}},
			},
			want: false,
		},
		{
			name:   "exceeded deadline",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{Conditions: []v1alpha1.RolloutPluginCondition{{
				Type:           RolloutPluginProgressing,
				LastUpdateTime: oldUpdate,
			}}},
			want: true,
		},
		{
			name:   "within deadline",
			plugin: basePlugin,
			status: v1alpha1.RolloutPluginStatus{Conditions: []v1alpha1.RolloutPluginCondition{{
				Type:           RolloutPluginProgressing,
				LastUpdateTime: recentUpdate,
			}}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RolloutPluginTimedOut(tt.plugin, &tt.status))
		})
	}
}

func TestRolloutPluginIsHealthy(t *testing.T) {
	plugin := validRolloutPlugin()

	tests := []struct {
		name   string
		status v1alpha1.RolloutPluginStatus
		want   bool
	}{
		{
			name:   "healthy when not in progress and not failed",
			status: v1alpha1.RolloutPluginStatus{RolloutInProgress: false, Phase: "Successful"},
			want:   true,
		},
		{
			name:   "not healthy when in progress",
			status: v1alpha1.RolloutPluginStatus{RolloutInProgress: true, Phase: "Successful"},
			want:   false,
		},
		{
			name:   "not healthy when failed",
			status: v1alpha1.RolloutPluginStatus{RolloutInProgress: false, Phase: "Failed"},
			want:   false,
		},
		{
			name:   "not healthy when degraded",
			status: v1alpha1.RolloutPluginStatus{RolloutInProgress: false, Phase: "Degraded"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RolloutPluginIsHealthy(plugin, &tt.status))
		})
	}
}

func TestVerifyRolloutPluginSpec(t *testing.T) {
	positive := int32(10)
	nonPositive := int32(0)

	tests := []struct {
		name    string
		mutate  func(rp *v1alpha1.RolloutPlugin)
		wantMsg string
	}{
		{
			name: "missing workloadRef name",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.WorkloadRef.Name = ""
			},
			wantMsg: "RolloutPlugin spec.workloadRef.name is required",
		},
		{
			name: "missing workloadRef kind",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.WorkloadRef.Kind = ""
			},
			wantMsg: "RolloutPlugin spec.workloadRef.kind is required",
		},
		{
			name: "missing workloadRef apiVersion",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.WorkloadRef.APIVersion = ""
			},
			wantMsg: "RolloutPlugin spec.workloadRef.apiVersion is required",
		},
		{
			name: "missing plugin name",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Plugin.Name = ""
			},
			wantMsg: "RolloutPlugin spec.plugin.name is required",
		},
		{
			name: "invalid strategy type",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Strategy.Type = "Invalid"
			},
			wantMsg: "RolloutPlugin spec.strategy.type must be 'Canary' or 'BlueGreen'",
		},
		{
			name: "canary type missing canary config",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Strategy.Type = "Canary"
				rp.Spec.Strategy.Canary = nil
			},
			wantMsg: "RolloutPlugin spec.strategy.canary is required when strategy type is 'Canary'",
		},
		{
			name: "bluegreen type missing bluegreen config",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Strategy.Type = "BlueGreen"
				rp.Spec.Strategy.BlueGreen = nil
				rp.Spec.Strategy.Canary = nil
			},
			wantMsg: "RolloutPlugin spec.strategy.blueGreen is required when strategy type is 'BlueGreen'",
		},
		{
			name: "both strategies set",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Strategy.BlueGreen = &v1alpha1.BlueGreenStrategy{}
			},
			wantMsg: "RolloutPlugin cannot have both canary and blueGreen strategies specified",
		},
		{
			name: "no strategy specified when type empty",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.Strategy.Type = ""
				rp.Spec.Strategy.Canary = nil
				rp.Spec.Strategy.BlueGreen = nil
			},
			wantMsg: "RolloutPlugin must have either canary or blueGreen strategy specified",
		},
		{
			name: "negative minReadySeconds",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.MinReadySeconds = -1
			},
			wantMsg: "RolloutPlugin spec.minReadySeconds cannot be negative",
		},
		{
			name: "non positive progressDeadlineSeconds",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.ProgressDeadlineSeconds = &nonPositive
			},
			wantMsg: "RolloutPlugin spec.progressDeadlineSeconds must be greater than 0",
		},
		{
			name: "same invalid message reuses previous condition",
			mutate: func(rp *v1alpha1.RolloutPlugin) {
				rp.Spec.WorkloadRef.Name = ""
				rp.Spec.ProgressDeadlineSeconds = &positive
			},
			wantMsg: "RolloutPlugin spec.workloadRef.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := validRolloutPlugin()
			tt.mutate(rp)

			var prevCond *v1alpha1.RolloutPluginCondition
			if tt.name == "same invalid message reuses previous condition" {
				oldUpdate := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				prevCond = &v1alpha1.RolloutPluginCondition{
					Type:           RolloutPluginInvalidSpec,
					Status:         corev1.ConditionTrue,
					Reason:         RolloutPluginInvalidSpecReason,
					Message:        tt.wantMsg,
					LastUpdateTime: oldUpdate,
				}
			}

			cond := VerifyRolloutPluginSpec(rp, prevCond)
			assert.NotNil(t, cond)
			assert.Equal(t, RolloutPluginInvalidSpec, cond.Type)
			assert.Equal(t, corev1.ConditionTrue, cond.Status)
			assert.Equal(t, RolloutPluginInvalidSpecReason, cond.Reason)
			assert.Equal(t, tt.wantMsg, cond.Message)

			if tt.name == "same invalid message reuses previous condition" {
				assert.Same(t, prevCond, cond)
			}
		})
	}

	t.Run("valid spec returns nil", func(t *testing.T) {
		rp := validRolloutPlugin()
		cond := VerifyRolloutPluginSpec(rp, nil)
		assert.Nil(t, cond)
	})
}
