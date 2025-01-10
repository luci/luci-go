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

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/taskspb"
)

var changeTC = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "process-change-task",
	Prototype: &taskspb.ProcessChangeTask{},
	Queue:     "changelog-generation",
	Kind:      tq.Transactional,
})

var replicationTC = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "replication-task",
	Prototype: &taskspb.ReplicationTask{},
	Queue:     "auth-db-replication",
	Kind:      tq.Transactional,
})

// RegisterChangeHandler registers the handler for changelog generation tasks.
func RegisterChangeHandler() {
	handler := func(ctx context.Context, payload protoreflect.ProtoMessage) error {
		task := payload.(*taskspb.ProcessChangeTask)
		logging.Infof(ctx, "change queue got revision %d", task.AuthDbRev)
		return handleProcessChangeTask(ctx, payload.(*taskspb.ProcessChangeTask))
	}
	changeTC.AttachHandler(handler)
}

// RegisterReplicationHandler registers the handler for replication tasks.
func RegisterReplicationHandler() {
	handler := func(ctx context.Context, payload protoreflect.ProtoMessage) error {
		task := payload.(*taskspb.ReplicationTask)
		logging.Infof(ctx, "replication queue got revision %d", task.AuthDbRev)
		return handleReplicationTask(ctx, payload.(*taskspb.ReplicationTask))
	}
	replicationTC.AttachHandler(handler)
}
