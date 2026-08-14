package main

import (
	"os"

	"github.com/tihaya-anon/ai-infra-lab/internal/plugin/gputopology"
	"k8s.io/component-base/cli"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	metrics := gputopology.NewMetrics(legacyregistry.Registerer())
	plugin := app.WithPlugin(gputopology.Name, gputopology.NewFactory(metrics))
	command := app.NewSchedulerCommand(plugin)
	os.Exit(cli.Run(command))
}
