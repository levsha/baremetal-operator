package ironic

import (
	"testing"
	"time"

	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"
)

func TestInspectHardware(t *testing.T) {

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name   string
		ironic *testserver.IronicMock

		expectedDirty        bool
		expectedRequestAfter int
		expectedResultError  string
		expectedDetailsHost  string

		expectedPublish string
		expectedError   string
	}{
		{
			name: "introspection-status-start-new-hardware-inspection",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "active",
			}),

			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "introspection-status-failed-404-retry-on-wait",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspect wait",
			}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "introspection-status-failed-404-retry",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspecting",
			}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "inspection-in-progress (not yet finished)",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.Inspecting),
			}),
			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "inspection-in-progress (but node still in InspectWait)",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectWait),
			}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "inspection-complete",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.Manageable),
				Properties:     map[string]interface{}{"memory_mb": "42"},
			}),

			expectedDirty:       false,
			expectedDetailsHost: "fixme.levsha.does.not.exist",
			expectedPublish:     "InspectionComplete Hardware inspection completed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			host := makeHost()
			publishedMsg := ""
			publisher := func(reason, message string) {
				publishedMsg = reason + " " + message
			}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
				tc.ironic.Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			prov.status.ID = nodeUUID
			result, details, err := prov.InspectHardware(false)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			assert.Equal(t, tc.expectedResultError, result.ErrorMessage)

			if details != nil {
				assert.Equal(t, tc.expectedDetailsHost, details.Hostname)
			}
			assert.Equal(t, tc.expectedPublish, publishedMsg)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Regexp(t, tc.expectedError, err.Error())
			}
		})
	}
}
