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

// Package tq provides a task queue implementation on top of Cloud Tasks.
//
// It exposes a high-level API that operates with proto messages and hides
// gory details such as serialization, routing, authentication, etc.
package tq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
)

// Dispatcher submits and handles Cloud Tasks tasks.
type Dispatcher struct {
	// Submitter is be used to submit Cloud Tasks tasks.
	//
	// Required. Use CloudTaskSubmitter to create a submitter that sends requests
	// to real Cloud Tasks.
	Submitter Submitter

	// GAE is true when running on Appengine.
	//
	// It alters how tasks are submitted and how incoming HTTP requests are
	// authenticated.
	GAE bool

	// CloudProject is ID of a Google Cloud project that hosts all queues.
	//
	// Required.
	CloudProject string

	// CloudRegion is a ID of a Google Cloud region that hosts all queues.
	//
	// Required.
	CloudRegion string

	// InternalRoutingPrefix is a relative URI prefix that will be used for all
	// exposed HTTP routes.
	//
	// This is a prefix in the server's router, usually "/internal/tasks/".
	//
	// Required.
	InternalRoutingPrefix string

	// ExternalRoutingPrefix is an absolute URL prefix that matches routes
	// exposed via InternalRoutingPrefix.
	//
	// It is used to construct a push HTTP URL for Cloud Tasks. Since Cloud Tasks
	// call the service from "outside", they need to know externally accessible
	// server's URL.
	//
	// This should usually be "https://<domain>/<InternalRoutingPrefix>", but may
	// be different if your load balancing layer does URL rewrites.
	//
	// Ignored on GAE. Required on non-GAE.
	ExternalRoutingPrefix string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for `PushAs` account.
	//
	// Ignored on GAE. Required on non-GAE.
	PushAs string

	mu       sync.RWMutex
	clsByID  map[string]*taskClassImpl
	clsByTyp map[reflect.Type]*taskClassImpl
}

// Submitter is used by Dispatcher to submit Cloud Tasks.
//
// Use CloudTaskSubmitter to create a submitter based on real Cloud Tasks API.
type Submitter interface {
	// CreateTask creates a task, returning a gRPC status.
	//
	// AlreadyExists status indicates the task with request name already exists.
	// Other statuses are handled using their usual semantics.
	//
	// Will be called from multiple goroutines at once.
	CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error
}

// TaskClass defines how to handles tasks of a specific proto message type.
type TaskClass struct {
	// ID is unique identifier of this class of tasks.
	//
	// It is used to decide how to deserialize and route the task. Changing IDs of
	// existing task classes is a disruptive operation, make sure the queue is
	// drained first. The dispatcher will permanently fail all Cloud Tasks with
	// unrecognized class IDs.
	ID string

	// Prototype identifies a proto message type of a task payload.
	//
	// Used for its type information only. In particular it is used by AddTask
	// to discover what TaskClass matches the added task. There should be
	// one-to-one correspondence between proto message types and task classes.
	//
	// It is safe to arbitrarily change this type as long as JSONPB encoding of
	// the previous type can be decoded using the new type. The dispatcher will
	// permanently fail Cloud Tasks with bodies it can't deserialize.
	Prototype proto.Message

	// Queue is a name of Cloud Tasks queue to use for the tasks.
	//
	// It is a short queue name within the region. The full queue name would be
	// "projects/<CloudProject>/locations/<CloudRegion>/queues/<Queue>", where
	// <CloudProject> and <CloudRegion> come from the Dispatcher configuration.
	//
	// This queue must exist already.
	Queue string

	// Handler will be called by the dispatcher to execute the tasks.
	//
	// The handler will receive the task's payload as a proto message of the exact
	// same type as type of Prototype.
	//
	// See Handler doc for more info.
	Handler Handler
}

// Task contains task body and metadata.
type Task struct {
	// Payload is task's payload as well as indicator of its class.
	//
	// Its type will be used to find a matching registered TaskClass which defines
	// how to route and handle the task.
	Payload proto.Message

	// DeduplicationKey is optional unique key used to derive name of the task.
	//
	// If a task of a given class with a given key has already been enqueued
	// recently (within ~1h), this task will be silently ignored.
	//
	// Because there is an extra lookup cost to identify duplicate task names,
	// enqueues of named tasks have significantly increased latency.
	//
	// Named tasks can only be used outside of transactions.
	DeduplicationKey string

	// Title is optional string that identifies the task in server logs.
	//
	// It will show up as a suffix in task handler URL. It exists exclusively to
	// simplify reading server logs. It serves no other purpose! In particular,
	// it is *not* a task name.
	//
	// Handlers won't ever see it. Pass all information through the payload.
	Title string

	// Delay specifies the duration the Cloud Tasks service must wait before
	// attempting to execute the task.
	//
	// Ignored if ETA is set.
	Delay time.Duration

	// ETA specifies the earliest time a task may be executed.
	//
	// If both Delay and ETA are unset, the task is executed as soon as possible.
	ETA time.Time
}

// Retry is an error tag used to indicate that the handler wants the task to
// be redelivered later.
//
// See Handler doc for more details.
var Retry = errors.BoolTag{Key: errors.NewTagKey("the task should be retried")}

// Handler is called to handle one enqueued task.
//
// If the returned error is tagged with Retry tag, the request finishes with
// HTTP status 409, indicating to the Cloud Tasks that it should attempt to
// execute the task later (which it may or may not do, depending on queue's
// retry config). Same happens if the error is transient (i.e. tagged with
// the transient.Tag), except the request finishes with HTTP status 500. This
// difference allows to distinguish "expected" retry requests (errors tagged
// with Retry) from "unexpected" ones (errors tagged with transient.Tag). Retry
// tag should be used **only** if the handler is fully aware of Cloud Tasks
// retry semantics and it **explicitly** wants the task to be retried because it
// can't be processed right now and the handler expects that the retry may help.
//
// For a contrived example, if the handler can process the task only after 2 PM,
// but it is 01:55 PM now, the handler should return an error tagged with Retry
// to indicate this. On the other hand, if the handler failed to process the
// task due to an RPC timeout or some other exceptional transient situation, it
// should return an error tagged with transient.Tag.
//
// Note that it is OK (and often desirable) to tag an error with both Retry and
// transient.Tag. Such errors propagate through the call stack as transient,
// until they reach Dispatcher, which treats them as retriable.
//
// An untagged error (or success) marks the task as "done", it won't be retried.
type Handler func(ctx context.Context, payload proto.Message) error

// RegisterTaskClass tells the dispatcher how to route and handle tasks of some
// particular type.
//
// Intended to be called during process startup. Panics if there's already
// a registered task class with the same ID or Prototype.
func (d *Dispatcher) RegisterTaskClass(cls TaskClass) {
	if cls.ID == "" {
		panic("TaskClass ID must be set")
	}
	if cls.Prototype == nil {
		panic("TaskClass Prototype must be set")
	}
	if cls.Handler == nil {
		panic("TaskClass Handler must be set")
	}
	if cls.Queue == "" {
		panic("TaskClass Queue must be set")
	}

	typ := reflect.TypeOf(cls.Prototype)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.clsByID == nil {
		d.clsByID = make(map[string]*taskClassImpl, 1)
	}
	if d.clsByTyp == nil {
		d.clsByTyp = make(map[reflect.Type]*taskClassImpl, 1)
	}

	if _, ok := d.clsByID[cls.ID]; ok {
		panic(fmt.Sprintf("TaskClass with ID %q is already registered", cls.ID))
	}
	if _, ok := d.clsByTyp[typ]; ok {
		panic(fmt.Sprintf("TaskClass with Prototype %T is already registered", cls.Prototype))
	}

	impl := &taskClassImpl{TaskClass: cls}
	d.clsByID[cls.ID] = impl
	d.clsByTyp[typ] = impl
}

// InstallRoutes installs HTTP routes under the given prefix.
//
// Panics if routes have already been installed.
//
// The exposed HTTP endpoints are called by Cloud Tasks service when it is time
// to execute a task.
func (d *Dispatcher) InstallRoutes(r *router.Router) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// TODO(vadimsh): Actually install routes.
}

// AddTask submits one or more tasks for later execution.
//
// The task payload type should match some registered TaskClass. Its ID will
// be used to identify the task class in the serialized Cloud Tasks task body.
//
// At some later time, in some other process, the dispatcher will invoke
// a handler specified in the corresponding TaskClass, based on its ID extracted
// from the task body.
//
// If the given context is transactional, inherits the transaction. It means
// the task will eventually be executed if and only if the transaction
// successfully commits.
//
// If the task has a DeduplicationKey and there already was a recent task with
// the same TaskClass ID and DeduplicationKey, silently ignores the task being
// added now. This works only outside of transactions. Using DeduplicationKey
// with transactional tasks results in an error.
//
// If given multiple tasks and running outside of a transaction, the operation
// is *not* atomic: if AddTask returns an error, it means it may have submitted
// some (but not all) tasks. There's no way to figure out which ones.
//
// Tags errors related to RPCs with transient.Tag. Logs individual RPC errors
// inside.
func (d *Dispatcher) AddTask(ctx context.Context, tasks ...*Task) (err error) {
	if err := d.checkConfigIsSane(); err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	preped, err := d.prepTasks(ctx, tasks)
	if err != nil {
		return err
	}

	// Submits the task and handles the error status and logging.
	submit := func(t *prepedTask) (err error) {
		ctx, span := trace.StartSpan(ctx, "go.chromium.org/luci/server/tq.AddTask")
		span.Attribute("cr.dev/class", t.cls.ID)
		span.Attribute("cr.dev/title", t.task.Title)
		defer func() { span.End(err) }()

		ctx = logging.SetFields(ctx, logging.Fields{
			"cr.dev/class": t.cls.ID,
			"cr.dev/title": t.task.Title,
		})

		// Each individual RPC should be pretty quick. Also Cloud Tasks client bugs
		// out if the context has large deadline.
		ctx, cancel := context.WithTimeout(ctx, time.Second*20)
		defer cancel()

		err = d.Submitter.CreateTask(ctx, &t.req)

		code := status.Code(err)
		span.Attribute("cr.dev/code", code)

		switch {
		case code == codes.OK, code == codes.AlreadyExists:
			return nil
		case transient.Tag.In(err) || grpcutil.IsTransientCode(code):
			logging.Warningf(ctx, "Transient error when creating the task: %s", err)
			return transient.Tag.Apply(err)
		default:
			logging.Errorf(ctx, "Fatal error when creating the task: %s", err)
			return err
		}
	}

	// One task is by far the most common case. Avoid launching parallel.WorkPool
	// machinery for it.
	if len(preped) == 1 {
		return submit(preped[0])
	}

	merr := parallel.WorkPool(32, func(work chan<- func() error) {
		for _, p := range preped {
			p := p
			work <- func() error { return submit(p) }
		}
	})
	if merr == nil {
		return nil
	}

	// If at least one error is fatal, return the overall error as fatal. If
	// all errors are transient, return the overall error as transient too.
	// We specifically do not want to return a multi error, it is tedious to
	// handle correctly. If callers care so much about status of each individual
	// task they should parallelize calls to AddTask manually themselves.
	var firstTransient error
	for _, err := range merr.(errors.MultiError) {
		if err != nil {
			if !transient.Tag.In(err) {
				return err
			}
			if firstTransient == nil {
				firstTransient = err
			}
		}
	}
	return firstTransient
}

////////////////////////////////////////////////////////////////////////////////

// defaultHeaders are added to all submitted Cloud Tasks.
var defaultHeaders = map[string]string{"Content-Type": "application/json"}

const (
	taskPrefix = "t/" // to get e.g. "/internal/tasks/t/..."
	cronPrefix = "c/" // unused for now
)

// prepedTask is a populated CreateTaskRequest along with information used
// to populate it.
type prepedTask struct {
	task *Task
	cls  *taskClassImpl
	req  taskspb.CreateTaskRequest
}

// checkConfigIsSane returns an error if Dispatcher is misconfigured.
func (d *Dispatcher) checkConfigIsSane() error {
	if d.Submitter == nil {
		return errors.New("bad Dispatcher: needs a Submitter")
	}
	if d.CloudProject == "" {
		return errors.New("bad Dispatcher: CloudProject must be set")
	}
	if d.CloudRegion == "" {
		return errors.New("bad Dispatcher: CloudRegion must be configured")
	}
	if !strings.HasPrefix(d.InternalRoutingPrefix, "/") {
		return errors.New("bad Dispatcher: InternalRoutingPrefix must begin with /")
	}

	if !d.GAE {
		if !strings.HasPrefix(d.ExternalRoutingPrefix, "https://") {
			return errors.New("bad Dispatcher: ExternalRoutingPrefix must begin with https://")
		}
		if d.PushAs == "" {
			return errors.New("bad Dispatcher: PushAs must be set")
		}
	}

	return nil
}

// internalURL joins d.InternalRoutingPrefix with `u`.
func (d *Dispatcher) internalURL(u string) string {
	out := d.InternalRoutingPrefix
	if !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out + u
}

// externalURL joins d.ExternalRoutingPrefix with `u`.
//
// Panics if d.ExternalRoutingPrefix is not set or invalid.
func (d *Dispatcher) externalURL(u string) string {
	out := d.ExternalRoutingPrefix
	if !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out + u
}

// prepTasks converts tasks into Cloud Tasks requests.
func (d *Dispatcher) prepTasks(ctx context.Context, tasks []*Task) ([]*prepedTask, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]*prepedTask, len(tasks))
	for i, tsk := range tasks {
		var err error
		if out[i], err = d.prepTaskLocked(ctx, tsk); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// prepTaskLocked converts a single task into Cloud Tasks request.
func (d *Dispatcher) prepTaskLocked(ctx context.Context, t *Task) (*prepedTask, error) {
	cls, ok := d.clsByTyp[reflect.TypeOf(t.Payload)]
	if !ok {
		return nil, errors.Reason("no TaskClass registered for type %T", t.Payload).Err()
	}

	body, err := cls.serialize(t)
	if err != nil {
		return nil, err
	}

	queueID := fmt.Sprintf(
		"projects/%s/locations/%s/queues/%s",
		d.CloudProject, d.CloudRegion, cls.Queue)

	taskID := ""
	if t.DeduplicationKey != "" {
		taskID = queueID + "/tasks/" + cls.taskName(t)
	}

	var scheduleTime *timestamppb.Timestamp
	switch {
	case !t.ETA.IsZero():
		scheduleTime = timestamppb.New(t.ETA)
	case t.Delay > 0:
		scheduleTime = timestamppb.New(clock.Now(ctx).Add(t.Delay))
	}

	// We need to populate one of Task.MessageType oneof alternatives. It has
	// unexported type, so we have to instantiate the message now and then mutate
	// it.
	preped := &prepedTask{
		task: t,
		cls:  cls,
		req: taskspb.CreateTaskRequest{
			Parent: queueID,
			Task: &taskspb.Task{
				Name:         taskID,
				ScheduleTime: scheduleTime,
				// TODO(vadimsh): Make DispatchDeadline configurable?
			},
		},
	}

	if d.GAE {
		preped.req.Task.MessageType = &taskspb.Task_AppEngineHttpRequest{
			AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
				HttpMethod:  taskspb.HttpMethod_POST,
				RelativeUri: d.internalURL(taskPrefix + cls.taskTitle(t)),
				Headers:     defaultHeaders,
				Body:        body,
			},
		}
	} else {
		preped.req.Task.MessageType = &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        d.externalURL(taskPrefix + cls.taskTitle(t)),
				Headers:    defaultHeaders,
				Body:       body,
				AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
					OidcToken: &taskspb.OidcToken{
						ServiceAccountEmail: d.PushAs,
					},
				},
			},
		}
	}

	return preped, nil
}

////////////////////////////////////////////////////////////////////////////////

// taskClassImpl knows how to prepare and handle tasks of a particular class.
type taskClassImpl struct {
	TaskClass
}

// envelope is what we put into all Cloud Tasks.
type envelope struct {
	Class string           `json:"class,omitempty"` // ID of TaskClass
	Type  string           `json:"type,omitempty"`  // for compatibility with appengine/tq
	Body  *json.RawMessage `json:"body"`            // JSONPB-serialized Task.Payload
}

// taskName returns a short ID for the task to use to dedup it.
func (cls *taskClassImpl) taskName(t *Task) string {
	// If we need to run in "appengine/tq" compatible mode, cls.ID below should be
	// replaced with proto.MessageName(t.Payload). But a breaking migration is
	// inevitable at some point (there's no way to have *two* different task names
	// at the same time), so might just as well start using ID instead of
	// MesasgeName right away. Migrating users would have to deal with potentially
	// duplicate messages for some period of time.
	h := sha256.New()
	h.Write([]byte(cls.ID))
	h.Write([]byte{0})
	h.Write([]byte(t.DeduplicationKey))
	return hex.EncodeToString(h.Sum(nil))
}

// taskTitle is used to construct URLs and for logging, it is not used for
// anything important.
func (cls *taskClassImpl) taskTitle(t *Task) string {
	if t.Title != "" {
		return t.Title
	}
	return cls.ID
}

// serialize serializes the task body into JSONPB.
func (cls *taskClassImpl) serialize(t *Task) ([]byte, error) {
	opts := protojson.MarshalOptions{
		Indent:         "\t",
		UseEnumNumbers: true,
	}
	blob, err := opts.Marshal(t.Payload)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize %T", t.Payload).Err()
	}
	raw := json.RawMessage(blob)
	return json.MarshalIndent(envelope{
		Class: cls.ID,
		Type:  string(proto.MessageName(t.Payload)),
		Body:  &raw,
	}, "", "\t")
}
