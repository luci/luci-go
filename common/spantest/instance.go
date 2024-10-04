// Copyright 2024 The LUCI Authors.
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

package spantest

import (
	"context"
	"fmt"
	"sync/atomic"

	spanins "cloud.google.com/go/spanner/admin/instance/apiv1"
	inspb "cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/spantest/emulator"
)

// instanceCounter counts instances created by the process.
var instanceCounter atomic.Int64

// EmulatedInstance is a Cloud Spanner instance inside the emulator.
type EmulatedInstance struct {
	// Name is a full instance name "projects/locally-emulated/instances/...".
	Name string
	// Emulator is the emulator that holds this instance.
	Emulator *emulator.Emulator
}

// NewEmulatedInstance creates a Cloud Spanner instance inside the emulator.
//
// It will have some randomly generated name.
func NewEmulatedInstance(ctx context.Context, e *emulator.Emulator) (*EmulatedInstance, error) {
	client, err := spanins.NewInstanceAdminClient(ctx, e.ClientOptions()...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	insOp, err := client.CreateInstance(ctx, &inspb.CreateInstanceRequest{
		Parent:     "projects/locally-emulated",
		InstanceId: fmt.Sprintf("test-instance-%d", instanceCounter.Add(1)),
		Instance:   &inspb.Instance{NodeCount: 1},
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create instance").Err()
	}

	switch ins, err := insOp.Wait(ctx); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to get instance state").Err()
	case ins.State != inspb.Instance_READY:
		return nil, errors.Reason("instance is not ready, got state %v", ins.State).Err()
	default:
		return &EmulatedInstance{
			Name:     ins.Name,
			Emulator: e,
		}, nil
	}
}
