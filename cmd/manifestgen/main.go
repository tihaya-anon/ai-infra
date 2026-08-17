package main

import (
	"fmt"
	"os"

	"github.com/tihaya-anon/ai-infra-lab/internal/manifests"
)

const outputPath = "deploy/kueue-resources.yaml"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	manifest, err := manifests.RenderKueueResources()
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, manifest, 0o644)
}
