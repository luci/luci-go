// Copyright 2021 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package monitor

import (
	"context"
	"sync"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

type grpcMonitor struct {
	lck        sync.Mutex
	client     pb.MonitoringServiceClient
	chunkSize  int
	connection *grpc.ClientConn
}

// NewGRPCMonitor creates a new Monitor object that sends metric by gRPC.
func NewGRPCMonitor(ctx context.Context, conn *grpc.ClientConn) Monitor {
	return &grpcMonitor{
		client:     pb.NewMonitoringServiceClient(conn),
		chunkSize:  500,
		connection: conn,
	}
}

// NewGRPCMonitorWithChunkSize creates a new GRPCMonitor with a given chunk size.
func NewGRPCMonitorWithChunkSize(ctx context.Context, chunkSize int, conn *grpc.ClientConn) Monitor {
	return &grpcMonitor{
		client:    pb.NewMonitoringServiceClient(conn),
		chunkSize: chunkSize,
	}
}

func (m *grpcMonitor) ChunkSize() int {
	return m.chunkSize
}

func (m *grpcMonitor) Send(ctx context.Context, cells []types.Cell) error {
	// Don't waste time on serialization if we are already too late.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	startTime := clock.Now(ctx)
	_, err := m.client.Insert(ctx, &pb.MonitoringInsertRequest{
		Payload: &pb.MetricsPayload{
			MetricsCollection: SerializeCells(cells, startTime),
		},
	})
	if err != nil {
		logging.Warningf(ctx, "tsmon: failed to send %d cells - %s", len(cells), err)
		return err
	}
	logging.Debugf(ctx, "tsmon: sent %d cells in %s", len(cells), clock.Now(ctx).Sub(startTime))
	return nil
}

func (m *grpcMonitor) Close() error {
	m.lck.Lock()
	defer m.lck.Unlock()
	if m.connection == nil {
		return nil
	}
	if err := m.connection.Close(); err != nil {
		return err
	}
	m.connection = nil
	return nil
}
