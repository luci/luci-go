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
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/metrics"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/tqpb"
)

// ErrStaleReminder is returned by ProcessReminderPostTxn.
var ErrStaleReminder = errors.New("the reminder is stale already")

// TraceContextHeader is name of a header that contains the trace context of
// a span that produced the task.
//
// It is always set regardless of InheritTraceContext setting. This header
// is read only by tq.Dispatcher itself and exists mostly for FYI purposes.
const TraceContextHeader = "X-Luci-Tq-Trace-Context"

// TxnPath indicates a code path a task can take.
type TxnPath string

const (
	TxnPathNone  TxnPath = "none"  // not a transactional task
	TxnPathHappy TxnPath = "happy" // the reminder was processed right after txn landed
	TxnPathSweep TxnPath = "sweep" // the reminder was processed during a sweep
)

// Submitter is used by the dispatcher and the sweeper to submit Cloud Tasks.
type Submitter interface {
	// CreateTask creates a task, returning a gRPC status.
	CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error
}

// Submit submits the prepared request through the given submitter.
//
// Mutates `req` by adding tracing headers to it.
//
// Recognizes AlreadyExists as success. Annotates retriable errors with
// transient.Tag.
func Submit(ctx context.Context, s Submitter, req *taskspb.CreateTaskRequest, extra *tqpb.Extra, t TxnPath) error {
	// Each individual RPC should be pretty quick. Also Cloud Tasks client bugs
	// out if the context has a large deadline.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()

	// Add a reference to the original trace context so that the trace of the
	// task handler can be matched to the span where AddTask was called.
	if extra.SpanContext != "" {
		var headers *map[string]string
		switch r := req.Task.MessageType.(type) {
		case *taskspb.Task_HttpRequest:
			headers = &r.HttpRequest.Headers
		case *taskspb.Task_AppEngineHttpRequest:
			headers = &r.AppEngineHttpRequest.Headers
		}
		if *headers == nil {
			*headers = make(map[string]string)
		}
		(*headers)[TraceContextHeader] = extra.SpanContext
		if extra.InheritTraceContext {
			(*headers)["X-Cloud-Trace-Context"] = extra.SpanContext
		}
	}

	start := clock.Now(ctx)
	err := s.CreateTask(ctx, req)
	code := status.Code(err)
	dur := clock.Now(ctx).Sub(start)

	metrics.SubmitCount.Add(ctx, 1, extra.TaskClass, string(t), code.String())
	metrics.SubmitDurationMS.Add(ctx, float64(dur.Milliseconds()), extra.TaskClass, string(t), code.String())

	switch code {
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
// If `req` and/or `extra` are nil, they will be deserialized from the reminder.
//
// Mutates `req` by adding tracing headers to it.
func SubmitFromReminder(ctx context.Context, s Submitter, db db.DB, r *reminder.Reminder, req *taskspb.CreateTaskRequest, extra *tqpb.Extra, t TxnPath) error {
	var err error
	if req == nil {
		req = &taskspb.CreateTaskRequest{}
		err = proto.Unmarshal(r.Payload, req)
	}
	if err == nil && extra == nil && len(r.Extra) != 0 {
		extra = &tqpb.Extra{}
		err = proto.Unmarshal(r.Extra, extra)
	}
	if extra == nil {
		extra = &tqpb.Extra{}
	}

	if err == nil {
		err = Submit(ctx, s, req, extra, t)
	}

	// Delete the reminder if the task was successfully enqueued or it is
	// a non-retriable failure.
	if !transient.Tag.In(err) {
		if rerr := db.DeleteReminder(ctx, r); rerr != nil {
			if err == nil {
				err = rerr
			}
		} else {
			metrics.RemindersDeleted.Add(ctx, 1, extra.TaskClass, string(t), db.Kind())
			if extra.Created.IsValid() {
				lat := clock.Now(ctx).Sub(extra.Created.AsTime())
				metrics.RemindersLatencyMS.Add(ctx, float64(lat.Milliseconds()), extra.TaskClass, string(t), db.Kind())
			}
		}
	}

	return err
}

// SubmitBatch process a batch of reminders by submitting corresponding
// Cloud Tasks and deleting reminders.
//
// Reminders batch will be modified to fetch Reminders' Payload. RAM usage is
// equivalent to O(total Payload size of each Reminder in batch).
//
// Logs errors inside. Returns the total number of successfully processed
// reminders.
func SubmitBatch(ctx context.Context, sub Submitter, db db.DB, batch []*reminder.Reminder) (int, error) {
	payloaded, err := db.FetchReminderPayloads(ctx, batch)
	switch missing := len(batch) - len(payloaded); {
	case missing < 0:
		panic(errors.Reason("%s.FetchReminderPayloads returned %d but asked for %d Reminders",
			db.Kind(), len(payloaded), len(batch)).Err())
	case err != nil:
		logging.Warningf(ctx, "Failed to fetch %d/%d Reminders: %s", missing, len(batch), err)
		// Continue processing whatever was fetched anyway.
	case missing > 0:
		logging.Warningf(ctx, "%d stale Reminders were unexpectedly deleted by something else. "+
			"If this persists, check for a misconfiguration of the sweeping or the happy path timeout",
			missing)
	}

	var success int32

	// Note: this can be optimized further by batching deletion of Reminders,
	// but the current version was good enough in load tests already.
	merr := parallel.WorkPool(16, func(work chan<- func() error) {
		for _, r := range payloaded {
			r := r
			work <- func() error {
				err := SubmitFromReminder(ctx, sub, db, r, nil, nil, TxnPathSweep)
				if err != nil {
					logging.Errorf(ctx, "Failed to process reminder %q: %s", r.ID, err)
				} else {
					atomic.AddInt32(&success, 1)
				}
				return err
			}
		}
	})

	count := int(atomic.LoadInt32(&success))

	switch {
	case err == nil:
		return count, merr
	case merr == nil:
		return count, err
	default:
		e := merr.(errors.MultiError)
		return count, append(e, err)
	}
}

// ProcessReminderPostTxn is called right after the transaction that saves the
// reminder.
//
// If the reminder is fresh enough, it means the sweeper hasn't picked it
// up yet and we can submit the task right now. This is the "happy" path. If
// the reminder is sufficiently old, or if SubmitFromReminder fails, we'll let
// the sweeper try to submit the task. It is the "sweep" path.
//
// Returns ErrStaleReminder if the reminder is stale and should be handled by
// the sweeper.
func ProcessReminderPostTxn(ctx context.Context, s Submitter, db db.DB, r *reminder.Reminder, req *taskspb.CreateTaskRequest, extra *tqpb.Extra) error {
	if clock.Now(ctx).After(r.FreshUntil) {
		metrics.RemindersCreated.Add(ctx, 1, extra.TaskClass, "stale", db.Kind())
		return ErrStaleReminder
	}
	metrics.RemindersCreated.Add(ctx, 1, extra.TaskClass, "fresh", db.Kind())
	return SubmitFromReminder(ctx, s, db, r, req, extra, TxnPathHappy)
}
