package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	crdPath       = "deploy/crd.yaml"
	schemaPath    = "deploy/schemas/aijob-v1alpha1.schema.json"
	schemaComment = "# yaml-language-server: $schema=../deploy/schemas/aijob-v1alpha1.schema.json"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	crd, err := os.ReadFile(crdPath)
	if err != nil {
		return err
	}
	schema, err := aiJobSchema(crd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(schemaPath, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return linkExamples()
}

func aiJobSchema(crd []byte) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(crd, &document); err != nil {
		return nil, err
	}
	versions, ok := nestedSlice(document, "spec", "versions")
	if !ok {
		return nil, fmt.Errorf("%s does not contain spec.versions", crdPath)
	}
	for _, version := range versions {
		versionMap, ok := version.(map[string]any)
		if !ok || versionMap["name"] != "v1alpha1" {
			continue
		}
		schema, ok := nestedMap(versionMap, "schema", "openAPIV3Schema")
		if !ok {
			return nil, fmt.Errorf("%s v1alpha1 does not contain openAPIV3Schema", crdPath)
		}
		result := map[string]any{"$schema": "http://json-schema.org/draft-07/schema#"}
		for key, value := range schema {
			result[key] = value
		}
		return result, nil
	}
	return nil, fmt.Errorf("%s does not contain AIJob v1alpha1 schema", crdPath)
}

func nestedMap(value map[string]any, path ...string) (map[string]any, bool) {
	current := value
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func nestedSlice(value map[string]any, path ...string) ([]any, bool) {
	parent, ok := nestedMap(value, path[:len(path)-1]...)
	if !ok {
		return nil, false
	}
	items, ok := parent[path[len(path)-1]].([]any)
	return items, ok
}

func linkExamples() error {
	matches, err := filepath.Glob("examples/*.yaml")
	if err != nil {
		return err
	}
	for _, path := range matches {
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !isAIJobExample(contents) {
			continue
		}
		linked := withSchemaComment(string(contents))
		if linked == string(contents) {
			continue
		}
		if err := os.WriteFile(path, []byte(linked), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func isAIJobExample(contents []byte) bool {
	text := string(contents)
	return strings.Contains(text, "apiVersion: infra.example.io/v1alpha1\n") &&
		strings.Contains(text, "kind: AIJob\n")
}

func withSchemaComment(contents string) string {
	if strings.HasPrefix(contents, schemaComment+"\n") {
		return contents
	}
	lines := strings.SplitAfter(contents, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "# yaml-language-server: $schema=") {
		lines[0] = schemaComment + "\n"
		return strings.Join(lines, "")
	}
	return schemaComment + "\n" + contents
}
