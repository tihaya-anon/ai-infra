package deploy

import (
	"io"
	"os"
	"testing"

	"github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

type manifest struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Strategy struct {
			Type string `yaml:"type"`
		} `yaml:"strategy"`
		Ports []struct {
			Port int `yaml:"port"`
		} `yaml:"ports"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name string   `yaml:"name"`
					Args []string `yaml:"args"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func TestGivenManifestsWhenReadingMetricsThenServicesAndBindingAreExposed(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	controller := readManifests(t, "../../deploy/controller.yaml")
	scheduler := readManifests(t, "../../deploy/scheduler-config.yaml")

	// when
	assertServicePort(t, controller, "aijob-controller-metrics", 8080)
	assertServicePort(t, scheduler, "ai-scheduler-metrics", 10259)
	deployment := findManifest(t, scheduler, "Deployment", "ai-scheduler")
	args := deployment.Spec.Template.Spec.Containers[0].Args

	// then
	assert.Expect(args).To(gomega.ContainElement("--bind-address=0.0.0.0"))
	assert.Expect(deployment.Spec.Strategy.Type).To(gomega.Equal("Recreate"))
}

func readManifests(t *testing.T, path string) []manifest {
	t.Helper()
	assert := gomega.NewWithT(t)
	file, err := os.Open(path)
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	defer file.Close()

	var manifests []manifest
	decoder := yaml.NewDecoder(file)
	for {
		var item manifest
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			assert.Expect(err).NotTo(gomega.HaveOccurred())
		}
		if item.Kind != "" {
			manifests = append(manifests, item)
		}
	}
	return manifests
}

func assertServicePort(t *testing.T, manifests []manifest, name string, port int) {
	t.Helper()
	assert := gomega.NewWithT(t)
	service := findManifest(t, manifests, "Service", name)
	assert.Expect(service.Spec.Ports).To(gomega.HaveLen(1))
	assert.Expect(service.Spec.Ports[0].Port).To(gomega.Equal(port))
}

func findManifest(t *testing.T, manifests []manifest, kind, name string) manifest {
	t.Helper()
	assert := gomega.NewWithT(t)
	for _, item := range manifests {
		if item.Kind == kind && item.Metadata.Name == name {
			return item
		}
	}
	assert.Expect(manifests).To(gomega.ContainElement(gomega.And(
		gomega.HaveField("Kind", kind),
		gomega.HaveField("Metadata.Name", name),
	)))
	return manifest{}
}
