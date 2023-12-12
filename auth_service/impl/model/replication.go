// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/taskspb"
)

// EnqueueReplicationTask adds a ReplicationTask task to the cloud task
// queue.
func EnqueueReplicationTask(ctx context.Context, authdbrev int64) error {
	if authdbrev < 0 {
		return errors.New("negative revision numbers are not allowed")
	}
	logging.Infof(ctx, "enqueuing %d", authdbrev)
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskspb.ReplicationTask{AuthDbRev: authdbrev},
		Title:   fmt.Sprintf("authdb-rev-%d", authdbrev),
	})
}

func handleReplicationTask(ctx context.Context, task *taskspb.ReplicationTask) error {
	// TODO(crbug/1376603): Implement.
	return nil
}
