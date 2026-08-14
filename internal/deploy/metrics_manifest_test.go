package deploy

import (
	"io"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

type manifest struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
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

func TestMetricServicesAndSchedulerBinding(t *testing.T) {
	controller := readManifests(t, "../../deploy/controller.yaml")
	scheduler := readManifests(t, "../../deploy/scheduler-config.yaml")

	assertServicePort(t, controller, "aijob-controller-metrics", 8080)
	assertServicePort(t, scheduler, "ai-scheduler-metrics", 10259)

	deployment := findManifest(t, scheduler, "Deployment", "ai-scheduler")
	args := deployment.Spec.Template.Spec.Containers[0].Args
	if !contains(args, "--bind-address=0.0.0.0") {
		t.Fatalf("Scheduler does not listen on its Pod interface: %#v", args)
	}
}

func readManifests(t *testing.T, path string) []manifest {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var manifests []manifest
	decoder := yaml.NewDecoder(file)
	for {
		var item manifest
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if item.Kind != "" {
			manifests = append(manifests, item)
		}
	}
	return manifests
}

func assertServicePort(t *testing.T, manifests []manifest, name string, port int) {
	t.Helper()
	service := findManifest(t, manifests, "Service", name)
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != port {
		t.Fatalf("Service %s does not expose port %d: %#v", name, port, service.Spec.Ports)
	}
}

func findManifest(t *testing.T, manifests []manifest, kind, name string) manifest {
	t.Helper()
	for _, item := range manifests {
		if item.Kind == kind && item.Metadata.Name == name {
			return item
		}
	}
	t.Fatalf("missing %s %s", kind, name)
	return manifest{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
