package deviceplugin

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestGivenDeviceCountWhenCreatingPluginThenHealthyDevicesAreStable(t *testing.T) {
	assert := gomega.NewWithT(t)

	// when
	devicePlugin, err := newPlugin(4)

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(devicePlugin.devices).To(gomega.HaveLen(4))
	assert.Expect(devicePlugin.devices[0]).To(gomega.Equal(&pluginapi.Device{
		ID: "simulated-gpu-0", Health: pluginapi.Healthy,
	}))
}

func TestGivenKnownDevicesWhenAllocatingThenNoHostDevicesAreInjected(t *testing.T) {
	assert := gomega.NewWithT(t)
	devicePlugin, err := newPlugin(2)
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	response, err := devicePlugin.Allocate(context.Background(), &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{{
			DevicesIds: []string{"simulated-gpu-0", "simulated-gpu-1"},
		}},
	})

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(response.ContainerResponses).To(gomega.HaveLen(1))
	assert.Expect(response.ContainerResponses[0].Devices).To(gomega.BeEmpty())
}

func TestGivenUnknownDeviceWhenAllocatingThenRequestIsRejected(t *testing.T) {
	assert := gomega.NewWithT(t)
	devicePlugin, err := newPlugin(1)
	assert.Expect(err).NotTo(gomega.HaveOccurred())

	// when
	_, err = devicePlugin.Allocate(context.Background(), &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{{
			DevicesIds: []string{"missing"},
		}},
	})

	// then
	assert.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("unknown simulated GPU")))
}
