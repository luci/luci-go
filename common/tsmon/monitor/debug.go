// Copyright 2015 The LUCI Authors.
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
	"os"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

type debugMonitor struct {
	path string
}

// NewDebugMonitor returns a Monitor that outputs metrics to a log, and
// optionally a file on disk.
func NewDebugMonitor(path string) Monitor {
	return &debugMonitor{
		path: path,
	}
}

func (m *debugMonitor) ChunkSize() int {
	return 0
}

func (m *debugMonitor) Send(ctx context.Context, cells []types.Cell, now time.Time) error {
	str := proto.MarshalTextString(&pb.MetricsPayload{
		MetricsCollection: SerializeCells(cells, clock.Now(ctx)),
	})

	logging.Infof(ctx, "Sending ts_mon metrics:\n%s", str)

	if m.path != "" {
		file, err := os.OpenFile(m.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0664)
		if err != nil {
			logging.Errorf(ctx, "Failed to open file %s: %s", m.path, err)
			return err
		}

		defer func() {
			if err := file.Close(); err != nil {
				logging.Errorf(ctx, "Failed to close file %s: %s", m.path, err)
			}
		}()

		if _, err = file.WriteString(str); err != nil {
			logging.Errorf(ctx, "Failed to write to the file %s: %s", m.path, err)
			return err
		}
	}

	return nil
}

func (m *debugMonitor) Close() error {
	return nil
}
