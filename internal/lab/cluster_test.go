package lab

import "testing"

func TestContextGuard(t *testing.T) {
	if err := validateContext("kind-ai-infra-lab", "kind-ai-infra-lab"); err != nil {
		t.Fatal(err)
	}
	if err := validateContext("production", "kind-ai-infra-lab"); err == nil {
		t.Fatal("context guard accepted an unrelated cluster")
	}
}
