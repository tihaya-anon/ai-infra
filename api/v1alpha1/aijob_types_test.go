package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/onsi/gomega"
)

func TestGivenAIJobArgsWhenRoundTrippingJSONThenOrderIsPreserved(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	original := &AIJob{Spec: AIJobSpec{
		Workers: 1, GPUPerWorker: 1,
		Args: []string{"--mode=complete", "--duration=2s"},
	}}

	// when
	data, err := json.Marshal(original)
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	decoded := &AIJob{}
	err = json.Unmarshal(data, decoded)

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(string(data)).To(gomega.ContainSubstring(
		`"args":["--mode=complete","--duration=2s"]`,
	))
	assert.Expect(decoded.Spec.Args).To(gomega.Equal(original.Spec.Args))
}

func TestGivenOptionalAIJobArgsWhenSerializingAndCopyingThenTheyRemainIndependent(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	withoutArgs := &AIJob{Spec: AIJobSpec{Workers: 1, GPUPerWorker: 1}}
	original := &AIJob{Spec: AIJobSpec{Args: []string{"first", "second"}}}

	// when
	data, err := json.Marshal(withoutArgs)
	copy := original.DeepCopy()
	copy.Spec.Args[0] = "changed"

	// then
	assert.Expect(err).NotTo(gomega.HaveOccurred())
	assert.Expect(string(data)).NotTo(gomega.ContainSubstring(`"args"`))
	assert.Expect(original.Spec.Args).To(gomega.Equal([]string{"first", "second"}))
	assert.Expect(copy.Spec.Args).To(gomega.Equal([]string{"changed", "second"}))
}
