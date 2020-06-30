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
	"sync/atomic"
	"time"

	"github.com/googleapis/gax-go"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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
	Options     ttq.Options
	DB          Database
	TasksClient TasksClient
}

// TasksClient decouples Impl from the Cloud Tasks client.
type TasksClient interface {
	CreateTask(context.Context, *taskspb.CreateTaskRequest, ...gax.CallOption) (*taskspb.Task, error)
}

// AddTask persists a task during a transaction and returns PostProcess to be
// called after the transaction completes (aka happy path).
func (impl *Impl) AddTask(ctx context.Context, req *taskspb.CreateTaskRequest) (ttq.PostProcess, error) {
	// TODO(tandrii): this can't be instrumented in a naive way because the
	// transaction can be retried, leading to inflated counts. Ideally, we'd want
	// a callback as soon as transaction successfully finishes or annotating
	// metrics with try number s.t. all retries can be ignored.
	r, reqClone, err := makeReminder(ctx, req)
	if err != nil {
		return nil, err
	}
	if err = impl.DB.SaveReminder(ctx, r); err != nil {
		return nil, err
	}
	once := int32(0)
	return func(outCtx context.Context) {
		if count := atomic.AddInt32(&once, 1); count > 1 {
			logging.Warningf(outCtx, "calling PostProcess %d times is not necessary", count)
			return
		}
		if clock.Now(outCtx).After(r.FreshUntil) {
			logging.Warningf(outCtx, "ttq happy path PostProcess %s has no time to run", r.Id)
			return
		}
		if err := impl.postProcess(outCtx, r, reqClone); err != nil {
			logging.Warningf(outCtx, "ttq happy path PostProcess %s failed: %s", r.Id, err)
		}
	}, nil
}

// Helpers.

// postProcess enqueues a task and deletes a reminder.
// During happy path, the req is not nil.
func (impl *Impl) postProcess(ctx context.Context, r *Reminder, req *taskspb.CreateTaskRequest) (err error) {
	startedAt := clock.Now(ctx)
	when := "happy"
	status := "OK"
	defer func() {
		durMS := float64(clock.Now(ctx).Sub(startedAt).Milliseconds())
		metricPostProcessedAttempts.Add(ctx, 1, status, when, impl.DB.Kind())
		metricPostProcessedDurationsMS.Add(ctx, durMS, status, when, impl.DB.Kind())
	}()

	if req == nil {
		when = "sweep"
		if req, err = r.createTaskRequest(); err != nil {
			status = "fail-deserialize"
			return err
		}
	}

	switch err = impl.createCloudTask(ctx, req, when); {
	case err == nil:
	case transient.Tag.In(err):
		status = "fail-task"
		return err
	default:
		status = "ignored-bad-task"
		// Proceed deleting reminder, there is no point retrying later.
	}
	if err = impl.DB.DeleteReminder(ctx, r); err != nil {
		// This may override "ignored-bad-task" status and this is fine as:
		//   * createCloudTask keeps metrics, too
		//   * most likely the reminder wasn't deleted,
		//     so there will be a retry later anyway.
		status = "fail-db"
		return err
	}
	return nil
}

// createCloudTask tries to create a Cloud Task.
// On AlreadyExists error, returns nil.
// The returned error has transient.Tag applied if the error isn't permanent.
func (impl *Impl) createCloudTask(ctx context.Context, req *taskspb.CreateTaskRequest, when string) error {
	// WORKAROUND(https://github.com/googleapis/google-cloud-go/issues/1577): if
	// the passed context deadline is larger than 30s, the CreateTask call fails
	// with InvalidArgument "The request deadline is ... The deadline cannot be
	// more than 30s in the future." So, give it 20s.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	_, err := impl.TasksClient.CreateTask(ctx, req)
	code := status.Code(err)
	switch code {
	case codes.OK:
	case codes.AlreadyExists:
		err = nil

	case codes.InvalidArgument:
		err = errors.Annotate(err, "failed to create Cloud Task").Err()

	case codes.Unavailable:
		fallthrough
	default:
		// TODO(tandrii): refine with use which errors really should be fatal and
		// which should not.
		err = errors.Annotate(err, "failed to create Cloud Task").Tag(transient.Tag).Err()
	}
	if when != "ttq" {
		// Monitor only tasks created for the user.
		metricTasksCreated.Add(ctx, 1, code.String(), when, impl.DB.Kind())
	}
	return err
}

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
