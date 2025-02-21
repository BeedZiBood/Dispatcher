package grpcDevice

import (
	"context"
	"fmt"
	device "github.com/BeedZiBood/protos/gen/go/DeviceService"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"log/slog"
)

type Client struct {
	api device.DeviceServiceClient
	log *slog.Logger
}

func New(
	log *slog.Logger,
	addr string,
) (*Client, error) {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("Can't create grpc client: %w", err)
	}

	return &Client{
		api: device.NewDeviceServiceClient(cc),
		log: log,
	}, nil
}

func (c *Client) GetDeviceList(ctx context.Context) ([]int32, error) {
	resp, err := c.api.GetDeviceList(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("Can't get device list: %w", err)
	}
	return resp.Devices, nil
}

func (c *Client) SendTest(ctx context.Context, deviceId, sourceId, testNumber uint) error {
	_, err := c.api.SendTest(ctx, &device.TestRequest{
		DeviceId:   uint32(deviceId),
		SourceId:   uint32(sourceId),
		TestNumber: uint32(testNumber),
	})
	if err != nil {
		return fmt.Errorf("Can't send test: %w", err)
	}
	return nil
}
