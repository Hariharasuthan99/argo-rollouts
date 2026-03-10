package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
)

func TestMetricsPortFlagCompatibility(t *testing.T) {
	cmd := newCommand()

	// Test that both flags are available
	metricsPortFlag := cmd.Flags().Lookup("metricsPort")
	assert.NotNil(t, metricsPortFlag, "metricsPort flag should exist")

	metricsportFlag := cmd.Flags().Lookup("metricsport")
	assert.NotNil(t, metricsportFlag, "metricsport flag should exist for backward compatibility")

	// Test that deprecated flag is marked as deprecated
	assert.True(t, metricsportFlag.Deprecated != "", "metricsport flag should be marked as deprecated")
}

func TestInitRegistersRolloutPluginTypesInScheme(t *testing.T) {
	obj, err := scheme.New(v1alpha1.SchemeGroupVersion.WithKind("RolloutPlugin"))
	assert.NoError(t, err)
	_, ok := obj.(*v1alpha1.RolloutPlugin)
	assert.True(t, ok)

	listObj, err := scheme.New(v1alpha1.SchemeGroupVersion.WithKind("RolloutPluginList"))
	assert.NoError(t, err)
	_, ok = listObj.(*v1alpha1.RolloutPluginList)
	assert.True(t, ok)
}

func TestInitRegistersCoreTypesInScheme(t *testing.T) {
	obj, err := scheme.New(metav1.SchemeGroupVersion.WithKind("Status"))
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestInitAddsRolloutPluginGVKsToKnownTypes(t *testing.T) {
	knownTypes := scheme.AllKnownTypes()

	rolloutPluginGVK := schema.GroupVersionKind{Group: v1alpha1.SchemeGroupVersion.Group, Version: v1alpha1.SchemeGroupVersion.Version, Kind: "RolloutPlugin"}
	_, hasRolloutPlugin := knownTypes[rolloutPluginGVK]
	assert.True(t, hasRolloutPlugin)

	rolloutPluginListGVK := schema.GroupVersionKind{Group: v1alpha1.SchemeGroupVersion.Group, Version: v1alpha1.SchemeGroupVersion.Version, Kind: "RolloutPluginList"}
	_, hasRolloutPluginList := knownTypes[rolloutPluginListGVK]
	assert.True(t, hasRolloutPluginList)
}
