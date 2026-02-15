package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/plugin/types"
)

type fakeRpcResourcePlugin struct {
	initErr         types.RpcError
	getStatusErr    types.RpcError
	setWeightErr    types.RpcError
	verifyWeightErr types.RpcError
	promoteErr      types.RpcError
	abortErr        types.RpcError
	restartErr      types.RpcError
	status          *types.ResourceStatus
	verified        bool
}

func (f *fakeRpcResourcePlugin) InitPlugin() types.RpcError {
	return f.initErr
}

func (f *fakeRpcResourcePlugin) GetResourceStatus(workloadRef v1alpha1.WorkloadRef) (*types.ResourceStatus, types.RpcError) {
	return f.status, f.getStatusErr
}

func (f *fakeRpcResourcePlugin) SetWeight(workloadRef v1alpha1.WorkloadRef, weight int32) types.RpcError {
	return f.setWeightErr
}

func (f *fakeRpcResourcePlugin) VerifyWeight(workloadRef v1alpha1.WorkloadRef, weight int32) (bool, types.RpcError) {
	return f.verified, f.verifyWeightErr
}

func (f *fakeRpcResourcePlugin) Promote(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return f.promoteErr
}

func (f *fakeRpcResourcePlugin) Abort(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return f.abortErr
}

func (f *fakeRpcResourcePlugin) Restart(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return f.restartErr
}

func (f *fakeRpcResourcePlugin) Type() string {
	return "fake"
}

func TestRpcPluginWrapperSuccessPaths(t *testing.T) {
	status := &types.ResourceStatus{Replicas: 3}
	plugin := &fakeRpcResourcePlugin{status: status, verified: true}
	wrapper := RpcPluginWrapper{RpcResourcePlugin: plugin}

	err := wrapper.Init()
	assert.NoError(t, err)

	gotStatus, err := wrapper.GetResourceStatus(context.Background(), v1alpha1.WorkloadRef{})
	assert.NoError(t, err)
	assert.Equal(t, status, gotStatus)

	err = wrapper.SetWeight(context.Background(), v1alpha1.WorkloadRef{}, 50)
	assert.NoError(t, err)

	verified, err := wrapper.VerifyWeight(context.Background(), v1alpha1.WorkloadRef{}, 50)
	assert.NoError(t, err)
	assert.True(t, verified)

	err = wrapper.Promote(context.Background(), v1alpha1.WorkloadRef{})
	assert.NoError(t, err)

	err = wrapper.Abort(context.Background(), v1alpha1.WorkloadRef{})
	assert.NoError(t, err)

	err = wrapper.Restart(context.Background(), v1alpha1.WorkloadRef{})
	assert.NoError(t, err)
}

func TestRpcPluginWrapperErrorPaths(t *testing.T) {
	plugin := &fakeRpcResourcePlugin{
		initErr:         types.RpcError{ErrorString: "init"},
		getStatusErr:    types.RpcError{ErrorString: "status"},
		setWeightErr:    types.RpcError{ErrorString: "weight"},
		verifyWeightErr: types.RpcError{ErrorString: "verify"},
		promoteErr:      types.RpcError{ErrorString: "promote"},
		abortErr:        types.RpcError{ErrorString: "abort"},
		restartErr:      types.RpcError{ErrorString: "restart"},
	}
	wrapper := RpcPluginWrapper{RpcResourcePlugin: plugin}

	assert.EqualError(t, wrapper.Init(), "failed to initialize plugin: init")

	_, err := wrapper.GetResourceStatus(context.Background(), v1alpha1.WorkloadRef{})
	assert.EqualError(t, err, "failed to get resource status: status")

	err = wrapper.SetWeight(context.Background(), v1alpha1.WorkloadRef{}, 50)
	assert.EqualError(t, err, "failed to set weight: weight")

	verified, err := wrapper.VerifyWeight(context.Background(), v1alpha1.WorkloadRef{}, 50)
	assert.EqualError(t, err, "failed to verify weight: verify")
	assert.False(t, verified)

	err = wrapper.Promote(context.Background(), v1alpha1.WorkloadRef{})
	assert.EqualError(t, err, "failed to promote: promote")

	err = wrapper.Abort(context.Background(), v1alpha1.WorkloadRef{})
	assert.EqualError(t, err, "failed to abort: abort")

	err = wrapper.Restart(context.Background(), v1alpha1.WorkloadRef{})
	assert.EqualError(t, err, "failed to reset: restart")
}
