// Package deviceplugin provides stable simulated GPU capacity for the kind lab.
package deviceplugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	DefaultDeviceCount = 4
	ResourceName       = "example.com/gpu"
	endpoint           = "simulated-gpu.sock"
)

// Options configures one Node-local simulated device plugin.
type Options struct {
	DeviceCount int
	SocketDir   string
}

type plugin struct {
	pluginapi.UnimplementedDevicePluginServer
	devices   []*pluginapi.Device
	deviceIDs map[string]struct{}
}

func newPlugin(deviceCount int) (*plugin, error) {
	if deviceCount < 1 {
		return nil, errors.New("device count must be positive")
	}
	result := &plugin{
		devices:   make([]*pluginapi.Device, 0, deviceCount),
		deviceIDs: make(map[string]struct{}, deviceCount),
	}
	for index := range deviceCount {
		id := fmt.Sprintf("simulated-gpu-%d", index)
		result.devices = append(result.devices, &pluginapi.Device{ID: id, Health: pluginapi.Healthy})
		result.deviceIDs[id] = struct{}{}
	}
	return result, nil
}

// Run serves and registers the plugin until the context is cancelled.
func Run(ctx context.Context, options Options) error {
	if options.DeviceCount == 0 {
		options.DeviceCount = DefaultDeviceCount
	}
	if options.SocketDir == "" {
		options.SocketDir = pluginapi.DevicePluginPath
	}
	devicePlugin, err := newPlugin(options.DeviceCount)
	if err != nil {
		return err
	}
	socket := filepath.Join(options.SocketDir, endpoint)
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socket, err)
	}
	defer listener.Close()
	defer os.Remove(socket)

	server := grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(server, devicePlugin)
	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve(listener) }()

	if err := register(ctx, options.SocketDir); err != nil {
		server.Stop()
		return err
	}
	select {
	case <-ctx.Done():
		server.Stop()
		return nil
	case err := <-serveError:
		return fmt.Errorf("serve device plugin: %w", err)
	}
}

func register(ctx context.Context, socketDir string) error {
	registerContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	kubeletSocket := filepath.Join(socketDir, filepath.Base(pluginapi.KubeletSocket))
	connection, err := grpc.DialContext(
		registerContext,
		"passthrough:///kubelet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", kubeletSocket)
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("connect to kubelet socket: %w", err)
	}
	defer connection.Close()
	client := pluginapi.NewRegistrationClient(connection)
	_, err = client.Register(registerContext, &pluginapi.RegisterRequest{
		Version: pluginapi.Version, Endpoint: endpoint, ResourceName: ResourceName,
		Options: &pluginapi.DevicePluginOptions{},
	})
	if err != nil {
		return fmt.Errorf("register simulated GPU resource: %w", err)
	}
	return nil
}

func (p *plugin) GetDevicePluginOptions(
	context.Context,
	*pluginapi.Empty,
) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (p *plugin) ListAndWatch(
	_ *pluginapi.Empty,
	stream grpc.ServerStreamingServer[pluginapi.ListAndWatchResponse],
) error {
	if err := stream.Send(&pluginapi.ListAndWatchResponse{Devices: p.devices}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func (p *plugin) Allocate(
	_ context.Context,
	request *pluginapi.AllocateRequest,
) (*pluginapi.AllocateResponse, error) {
	containerCount := len(request.ContainerRequests)
	response := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, 0, containerCount),
	}
	for _, container := range request.ContainerRequests {
		for _, id := range container.DevicesIds {
			if _, exists := p.deviceIDs[id]; !exists {
				return nil, status.Errorf(codes.InvalidArgument, "unknown simulated GPU %q", id)
			}
		}
		response.ContainerResponses = append(
			response.ContainerResponses, &pluginapi.ContainerAllocateResponse{},
		)
	}
	return response, nil
}
