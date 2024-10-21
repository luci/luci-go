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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

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

// NewGRPCMonitor creates a new Monitor that sends metric by gRPC with a given
// chunk size.
func NewGRPCMonitor(ctx context.Context, chunkSize int, conn *grpc.ClientConn) Monitor {
	return &grpcMonitor{
		client:     pb.NewMonitoringServiceClient(conn),
		chunkSize:  chunkSize,
		connection: conn,
	}
}

func (m *grpcMonitor) ChunkSize() int {
	return m.chunkSize
}

func (m *grpcMonitor) Send(ctx context.Context, cells []types.Cell, now time.Time) error {
	// Don't waste time on serialization if we are already too late.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Note `now` can actually be in the past. Here we use `startTime` exclusively
	// to measure how long it takes to call Insert. So get the freshest value.
	startTime := clock.Now(ctx)

	resp, err := m.client.Insert(ctx, &pb.MonitoringInsertRequest{
		Payload: &pb.MetricsPayload{
			MetricsCollection: SerializeCells(cells, now),
		},
	})
	if err != nil {
		logging.Warningf(ctx, "tsmon: failed to send %d cells - %s", len(cells), err)
		return err
	}

	if resp.ResponseStatus != nil {
		if err := status.FromProto(resp.ResponseStatus).Err(); err != nil {
			logging.Warningf(ctx, "tsmon: got non-OK response status - %s", err)
		}
	}

	logging.Debugf(ctx, "tsmon: sent %d cells in %s", len(cells), clock.Since(ctx, startTime))
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
