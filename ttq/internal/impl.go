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
	"encoding/hex"
	"io"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/ttq"
)

// Reminder reminds to enqueue a task.
//
// It is persisted transactionally with some other user logic to the database.
// Later, a task is actually scheduled and a reminder can be deleted
// non-transactionally.
type Reminder struct {
	// Id identifies a reminder.
	//
	// Id values are always in hex-encoded and are well distributed in keyspace.
	Id string
	// FreshUntil is the expected time by which the happy path should complete.
	//
	// If the sweeper encounters a Reminder before this time, the sweeper ignores
	// it to allow the happy path to complete.
	FreshUntil time.Time
	// Payload is a proto-serialized taskspb.Task.
	Payload []byte
}

// Impl implements abstract transaction enqueue semantics on top of Database
// interface.
type Impl struct {
	Options ttq.Options
	DB      Database
}

// Helpers.

const (
	// keySpaceBytes defines the space of the Reminder Ids.
	//
	// Because Reminder.Id is hex-encoded, actual len(Ids) == 2*keySpaceBytes.
	//
	// 16 is chosen is big enough to avoid collisions in practice yet small enough
	// for easier human-debugging of key ranges in queries.
	keySpaceBytes = 16

	// happyPathMaxDuration caps how long the happy path will be waited for.
	happyPathMaxDuration = time.Minute
)

// makeReminder creates a Reminder and a cloned CreateTaskRequest with a named
// task.
//
// The request is cloned to avoid bugs when user's transaction code is retried.
// The resulting request's task is then named to avoid avoid duplicate task
// creation later on.
// The resulting cloned request is returned to avoid needlessly deserializing it
// from the Reminder.Payload in the PostProcess callback.
func makeReminder(ctx context.Context, req *taskspb.CreateTaskRequest) (*Reminder, *taskspb.CreateTaskRequest, error) {
	if req.Task == nil {
		return nil, nil, errors.New("CreateTaskRequest.Task must be set")
	}
	if req.Task.Name != "" {
		return nil, nil, errors.New("CreateTaskRequest.Task.Name must not be set. Named tasks are not supported")
	}
	buf := make([]byte, keySpaceBytes)
	if _, err := io.ReadFull(cryptorand.Get(ctx), buf); err != nil {
		return nil, nil, errors.Annotate(err, "failed to get random bytes").Tag(transient.Tag).Err()
	}

	r := &Reminder{Id: hex.EncodeToString(buf)}
	// Bound FreshUntil to at most current context deadline.
	r.FreshUntil = clock.Now(ctx).Add(happyPathMaxDuration)
	if deadline, ok := ctx.Deadline(); ok && r.FreshUntil.After(deadline) {
		// TODO(tandrii): allow propagating custom deadline for the async happy
		// path which won't bind the context's deadline.
		r.FreshUntil = deadline
	}
	r.FreshUntil = r.FreshUntil.UTC()

	clone := proto.Clone(req).(*taskspb.CreateTaskRequest)
	clone.Task.Name = clone.Parent + "/tasks/" + r.Id
	var err error
	if r.Payload, err = proto.Marshal(clone); err != nil {
		return nil, nil, errors.Annotate(err, "failed to marshal the task").Err()
	}
	return r, clone, nil
}

func (r *Reminder) createTaskRequest() (*taskspb.CreateTaskRequest, error) {
	req := &taskspb.CreateTaskRequest{}
	if err := proto.Unmarshal(r.Payload, req); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal the task").Err()
	}
	return req, nil
}
