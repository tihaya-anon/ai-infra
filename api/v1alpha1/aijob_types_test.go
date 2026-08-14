package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAIJobArgsJSONRoundTrip(t *testing.T) {
	original := &AIJob{Spec: AIJobSpec{
		Workers: 1, GPUPerWorker: 1,
		Args: []string{"--mode=complete", "--duration=2s"},
	}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"args":["--mode=complete","--duration=2s"]`) {
		t.Fatalf("ordered args missing from JSON: %s", data)
	}

	decoded := &AIJob{}
	if err := json.Unmarshal(data, decoded); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(decoded.Spec.Args, ","); got != "--mode=complete,--duration=2s" {
		t.Fatalf("got args %q", got)
	}
}

func TestAIJobArgsAreOptionalAndDeepCopied(t *testing.T) {
	withoutArgs, err := json.Marshal(&AIJob{Spec: AIJobSpec{Workers: 1, GPUPerWorker: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutArgs), `"args"`) {
		t.Fatalf("omitted args must not be serialized: %s", withoutArgs)
	}

	original := &AIJob{Spec: AIJobSpec{Args: []string{"first", "second"}}}
	copy := original.DeepCopy()
	copy.Spec.Args[0] = "changed"
	if original.Spec.Args[0] != "first" {
		t.Fatal("DeepCopy shared the args backing array")
	}
}
