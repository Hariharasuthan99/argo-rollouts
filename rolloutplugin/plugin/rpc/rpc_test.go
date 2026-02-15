package rpc

import (
	"context"
	"testing"
	"time"

	goPlugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/argoproj/argo-rollouts/utils/plugin/types"
)

var testHandshake = goPlugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ARGO_ROLLOUTS_RPC_PLUGIN",
	MagicCookieValue: "resourceplugin",
}

type testResourcePlugin struct {
	status   *types.ResourceStatus
	verified bool
}

func (t *testResourcePlugin) InitPlugin() types.RpcError {
	return types.RpcError{}
}

func (t *testResourcePlugin) GetResourceStatus(workloadRef v1alpha1.WorkloadRef) (*types.ResourceStatus, types.RpcError) {
	return t.status, types.RpcError{}
}

func (t *testResourcePlugin) SetWeight(workloadRef v1alpha1.WorkloadRef, weight int32) types.RpcError {
	return types.RpcError{}
}

func (t *testResourcePlugin) VerifyWeight(workloadRef v1alpha1.WorkloadRef, weight int32) (bool, types.RpcError) {
	return t.verified, types.RpcError{}
}

func (t *testResourcePlugin) Promote(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (t *testResourcePlugin) Abort(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (t *testResourcePlugin) Restart(workloadRef v1alpha1.WorkloadRef) types.RpcError {
	return types.RpcError{}
}

func (t *testResourcePlugin) Type() string {
	return "TestResourcePlugin"
}

func pluginClient(t *testing.T) (ResourcePlugin, goPlugin.ClientProtocol, func(), chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())

	rpcPluginImp := &testResourcePlugin{
		status:   &types.ResourceStatus{Replicas: 3},
		verified: true,
	}

	pluginMap := map[string]goPlugin.Plugin{
		"RpcResourcePlugin": &RpcResourcePlugin{Impl: rpcPluginImp},
	}

	ch := make(chan *goPlugin.ReattachConfig, 1)
	closeCh := make(chan struct{})
	go goPlugin.Serve(&goPlugin.ServeConfig{
		HandshakeConfig: testHandshake,
		Plugins:         pluginMap,
		Test: &goPlugin.ServeTestConfig{
			Context:          ctx,
			ReattachConfigCh: ch,
			CloseCh:          closeCh,
		},
	})

	var config *goPlugin.ReattachConfig
	select {
	case config = <-ch:
	case <-time.After(2000 * time.Millisecond):
		t.Fatal("should've received reattach")
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}

	c := goPlugin.NewClient(&goPlugin.ClientConfig{
		Cmd:             nil,
		HandshakeConfig: testHandshake,
		Plugins:         pluginMap,
		Reattach:        config,
	})
	client, err := c.Client()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	raw, err := client.Dispense("RpcResourcePlugin")
	if err != nil {
		t.Fail()
	}

	plugin, ok := raw.(ResourcePlugin)
	if !ok {
		t.Fail()
	}

	return plugin, client, cancel, closeCh
}

func TestPlugin(t *testing.T) {
	plugin, _, cancel, closeCh := pluginClient(t)
	defer cancel()

	err := plugin.InitPlugin()
	assert.Equal(t, "", err.Error())

	status, err := plugin.GetResourceStatus(v1alpha1.WorkloadRef{})
	assert.Equal(t, "", err.Error())
	assert.Equal(t, int32(3), status.Replicas)

	err = plugin.SetWeight(v1alpha1.WorkloadRef{}, 50)
	assert.Equal(t, "", err.Error())

	verified, err := plugin.VerifyWeight(v1alpha1.WorkloadRef{}, 50)
	assert.Equal(t, "", err.Error())
	assert.True(t, verified)

	err = plugin.Promote(v1alpha1.WorkloadRef{})
	assert.Equal(t, "", err.Error())

	err = plugin.Abort(v1alpha1.WorkloadRef{})
	assert.Equal(t, "", err.Error())

	err = plugin.Restart(v1alpha1.WorkloadRef{})
	assert.Equal(t, "", err.Error())

	assert.Equal(t, "TestResourcePlugin", plugin.Type())

	cancel()
	<-closeCh
}

func TestPluginClosedConnection(t *testing.T) {
	plugin, client, cancel, closeCh := pluginClient(t)
	defer cancel()

	client.Close()
	time.Sleep(100 * time.Millisecond)

	const expectedError = "connection is shut down"

	err := plugin.InitPlugin()
	assert.Contains(t, err.Error(), expectedError)

	_, err = plugin.GetResourceStatus(v1alpha1.WorkloadRef{})
	assert.Contains(t, err.Error(), expectedError)

	err = plugin.SetWeight(v1alpha1.WorkloadRef{}, 0)
	assert.Contains(t, err.Error(), expectedError)

	_, err = plugin.VerifyWeight(v1alpha1.WorkloadRef{}, 0)
	assert.Contains(t, err.Error(), expectedError)

	err = plugin.Promote(v1alpha1.WorkloadRef{})
	assert.Contains(t, err.Error(), expectedError)

	err = plugin.Abort(v1alpha1.WorkloadRef{})
	assert.Contains(t, err.Error(), expectedError)

	err = plugin.Restart(v1alpha1.WorkloadRef{})
	assert.Contains(t, err.Error(), expectedError)

	cancel()
	<-closeCh
}

func TestInvalidArgs(t *testing.T) {
	server := ResourcePluginRPCServer{Impl: &testResourcePlugin{}}
	badtype := struct {
		Args string
	}{
		Args: "bad",
	}

	var errRpc types.RpcError
	var statusResp GetResourceStatusResponse
	var verifyResp VerifyWeightResponse

	err := server.GetResourceStatus(badtype, &statusResp)
	assert.NoError(t, err)
	assert.Contains(t, statusResp.Error.ErrorString, "invalid args")

	err = server.SetWeight(badtype, &errRpc)
	assert.NoError(t, err)
	assert.Contains(t, errRpc.ErrorString, "invalid args")

	err = server.VerifyWeight(badtype, &verifyResp)
	assert.NoError(t, err)
	assert.Contains(t, verifyResp.Error.ErrorString, "invalid args")

	err = server.Promote(badtype, &errRpc)
	assert.NoError(t, err)
	assert.Contains(t, errRpc.ErrorString, "invalid args")

	err = server.Abort(badtype, &errRpc)
	assert.NoError(t, err)
	assert.Contains(t, errRpc.ErrorString, "invalid args")

	err = server.Restart(badtype, &errRpc)
	assert.NoError(t, err)
	assert.Contains(t, errRpc.ErrorString, "invalid args")
}
