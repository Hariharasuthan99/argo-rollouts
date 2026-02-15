package rolloutplugin

import (
	"context"
	"testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/stretchr/testify/assert"
)

type fakeResourcePlugin struct {
	initCalls int
	initErr   error
}

func (f *fakeResourcePlugin) Init() error {
	f.initCalls++
	return f.initErr
}

func (f *fakeResourcePlugin) GetResourceStatus(ctx context.Context, workloadRef v1alpha1.WorkloadRef) (*ResourceStatus, error) {
	return &ResourceStatus{}, nil
}

func (f *fakeResourcePlugin) SetWeight(ctx context.Context, workloadRef v1alpha1.WorkloadRef, weight int32) error {
	return nil
}

func (f *fakeResourcePlugin) VerifyWeight(ctx context.Context, workloadRef v1alpha1.WorkloadRef, weight int32) (bool, error) {
	return true, nil
}

func (f *fakeResourcePlugin) Promote(ctx context.Context, workloadRef v1alpha1.WorkloadRef) error {
	return nil
}

func (f *fakeResourcePlugin) Abort(ctx context.Context, workloadRef v1alpha1.WorkloadRef) error {
	return nil
}

func (f *fakeResourcePlugin) Restart(ctx context.Context, workloadRef v1alpha1.WorkloadRef) error {
	return nil
}

func TestRegisterAndGetPlugin(t *testing.T) {
	pm := &DefaultPluginManager{plugins: map[string]ResourcePlugin{}}

	plugin := &fakeResourcePlugin{}
	err := pm.RegisterPlugin("test", plugin)
	assert.NoError(t, err)
	assert.Equal(t, 1, plugin.initCalls)

	got, err := pm.GetPlugin("test")
	assert.NoError(t, err)
	assert.Same(t, plugin, got)
}

func TestRegisterPluginDuplicateReturnsError(t *testing.T) {
	pm := &DefaultPluginManager{plugins: map[string]ResourcePlugin{}}

	plugin := &fakeResourcePlugin{}
	err := pm.RegisterPlugin("test", plugin)
	assert.NoError(t, err)

	err = pm.RegisterPlugin("test", plugin)
	assert.Error(t, err)
	assert.Equal(t, 1, plugin.initCalls)
}

func TestRegisterPluginInitFailure(t *testing.T) {
	pm := &DefaultPluginManager{plugins: map[string]ResourcePlugin{}}

	plugin := &fakeResourcePlugin{initErr: assert.AnError}
	err := pm.RegisterPlugin("test", plugin)
	assert.Error(t, err)

	_, getErr := pm.GetPlugin("test")
	assert.Error(t, getErr)
}

func TestGetPluginNotFound(t *testing.T) {
	pm := &DefaultPluginManager{plugins: map[string]ResourcePlugin{}}

	_, err := pm.GetPlugin("missing")
	assert.Error(t, err)
}
