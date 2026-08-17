package lab

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestGivenKubernetesContextsWhenValidatingThenOnlyExpectedKindContextIsAccepted(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	expectedContext := "kind-ai-infra-lab"

	// when
	matchingError := validateContext(expectedContext, expectedContext)
	unrelatedError := validateContext("production", expectedContext)

	// then
	assert.Expect(matchingError).NotTo(gomega.HaveOccurred())
	assert.Expect(unrelatedError).To(gomega.HaveOccurred())
}
