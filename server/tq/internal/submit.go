// Copyright 2020 The LUCI Authors.
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

package internal

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/reminder"
)

// TODO(vadimsh): Add metrics.

// Submitter is used by the dispatcher and the sweeper to submit Cloud Tasks.
type Submitter interface {
	// CreateTask creates a task, returning a gRPC status.
	CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error
}

// Submit submits the prepared request through the given submitter.
//
// Recognizes AlreadyExists as success. Annotates retriable errors with
// transient.Tag.
func Submit(ctx context.Context, s Submitter, req *taskspb.CreateTaskRequest) error {
	// Each individual RPC should be pretty quick. Also Cloud Tasks client bugs
	// out if the context has a large deadline.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()

	switch err := s.CreateTask(ctx, req); status.Code(err) {
	case codes.OK, codes.AlreadyExists:
		return nil
	case codes.Internal,
		codes.Unknown,
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.Canceled:
		return transient.Tag.Apply(err)
	default:
		return err
	}
}

// SubmitFromReminder submits the request and deletes the reminder on success
// or a fatal error.
//
// If `req` is nil, the request will be deserialized from the reminder payload.
func SubmitFromReminder(ctx context.Context, s Submitter, db databases.Database, r *reminder.Reminder, req *taskspb.CreateTaskRequest) error {
	var err error
	if req == nil {
		req = &taskspb.CreateTaskRequest{}
		err = proto.Unmarshal(r.Payload, req)
	}

	if err == nil {
		err = Submit(ctx, s, req)
	}

	// Delete the reminder if the task was successfully enqueued or it is
	// a non-retriable failure.
	if !transient.Tag.In(err) {
		if rerr := db.DeleteReminder(ctx, r); rerr != nil {
			if err == nil {
				err = rerr
			}
		}
	}

	return err
}
