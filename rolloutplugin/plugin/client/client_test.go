package client

import (
	"sync"
	"testing"

	goPlugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/config"
	"github.com/argoproj/argo-rollouts/utils/plugin/types"
)

type fakeRpcResourcePlugin struct{}

func (f *fakeRpcResourcePlugin) InitPlugin() types.RpcError {
	return types.RpcError{}
}

func (f *fakeRpcResourcePlugin) GetResourceStatus(workloadRef v1alpha1.WorkloadRef) (*types.ResourceStatus, types.RpcError) {
	return &types.ResourceStatus{}, types.RpcError{}
}

func (f *fakeRpcResourcePlugin) SetWeight(workloadRef v1alpha1.WorkloadRef, weight int32) types.RpcError {
	return types.RpcError{}
}

func (f *fakeRpcResourcePlugin) VerifyWeight(workloadRef v1alpha1.WorkloadRef, weight int32) (bool, types.RpcError) {
	return true, types.RpcError{}
}

func (f *fakeRpcResourcePlugin) Promote(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (f *fakeRpcResourcePlugin) Abort(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (f *fakeRpcResourcePlugin) Restart(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (f *fakeRpcResourcePlugin) Type() string {
	return "fake"
}

func resetClientStateForTest() {
	pluginClients = nil
	once = sync.Once{}
	config.UnInitializeConfig()
}

func TestGetResourcePlugin_InitializesSingletonAndReturnsWrappedError(t *testing.T) {
	resetClientStateForTest()
	t.Cleanup(resetClientStateForTest)

	plugin, err := GetResourcePlugin("argoproj-labs/resource")
	assert.Nil(t, plugin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to start plugin system")
	assert.Contains(t, err.Error(), "unable to find plugin (argoproj-labs/resource)")
	assert.NotNil(t, pluginClients)
	assert.NotNil(t, pluginClients.pluginClient)
	assert.NotNil(t, pluginClients.plugin)
}

func TestStartPlugin_ReturnsCachedPluginWithoutRestart(t *testing.T) {
	fakePlugin := &fakeRpcResourcePlugin{}
	r := &resourcePlugin{
		pluginClient: map[string]*goPlugin.Client{
			"argoproj-labs/resource": {},
		},
		plugin: map[string]types.RpcResourcePlugin{
			"argoproj-labs/resource": fakePlugin,
		},
	}

	plugin, err := r.startPlugin("argoproj-labs/resource")
	assert.NoError(t, err)
	assert.Equal(t, fakePlugin, plugin)
}

func TestStartPlugin_ReturnsWrappedErrorWhenPluginLookupFails(t *testing.T) {
	resetClientStateForTest()
	t.Cleanup(resetClientStateForTest)

	r := &resourcePlugin{
		pluginClient: map[string]*goPlugin.Client{},
		plugin:       map[string]types.RpcResourcePlugin{},
	}

	plugin, err := r.startPlugin("argoproj-labs/resource")
	assert.Nil(t, plugin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find plugin (argoproj-labs/resource)")
	assert.Contains(t, err.Error(), "failed to get config")
}
