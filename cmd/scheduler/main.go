package main

import (
	"os"

	"github.com/tihaya-anon/ai-infra-lab/internal/plugin/gputopology"
	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	command := app.NewSchedulerCommand(app.WithPlugin(gputopology.Name, gputopology.New))
	os.Exit(cli.Run(command))
}
