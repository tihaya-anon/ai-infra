package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tihaya-anon/ai-infra-lab/internal/deviceplugin"
)

func main() {
	devices := flag.Int("devices", deviceplugin.DefaultDeviceCount, "simulated GPUs per Node")
	socketDir := flag.String("socket-dir", "", "kubelet device plugin socket directory")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := deviceplugin.Run(ctx, deviceplugin.Options{
		DeviceCount: *devices, SocketDir: *socketDir,
	}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "simulated-device-plugin:", err)
		os.Exit(1)
	}
}
