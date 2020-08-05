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
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/timestamppb"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/server/tq/internal"
	"go.chromium.org/luci/server/tq/tqtesting"
	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/reminder"
)

// Dispatcher submits and handles Cloud Tasks tasks.
//
// Can be instantiated directly in tests. Production code should generally use
// the dispatcher configured by the tq server module (which is global Default
// dispatcher). See NewModuleWithFlags() and ModuleOptions.
type Dispatcher struct {
	// Submitter is used to submit Cloud Tasks tasks.
	//
	// Use CloudTaskSubmitter to create a submitter that sends requests to real
	// Cloud Tasks.
	//
	// If not set, task submissions will fail.
	Submitter Submitter

	// Sweeper knows how to launch sweeps of the transactional tasks reminders.
	//
	// If not set, Sweep calls will fail.
	Sweeper Sweeper

	// Namespace is a namespace for tasks that use DeduplicationKey.
	//
	// This is needed if two otherwise independent deployments share a single
	// Cloud Tasks instance.
	//
	// Must be valid per ValidateNamespace. Default is "".
	Namespace string

	// GAE is true when running on Appengine.
	//
	// It alters how tasks are submitted and how incoming HTTP requests are
	// authenticated.
	GAE bool

	// NoAuth can be used to disable authentication on HTTP endpoints.
	//
	// This is useful when running in development mode on localhost or in tests.
	NoAuth bool

	// CloudProject is ID of a project to use to construct full queue names.
	//
	// If not set, submission of tasks that use short queue names will fail.
	CloudProject string

	// CloudRegion is a ID of a region to use to construct full queue names.
	//
	// If not set, submission of tasks that use short queue names will fail.
	CloudRegion string

	// DefaultRoutingPrefix is a URL prefix for produced Cloud Tasks.
	//
	// Used only for tasks whose TaskClass doesn't provide some custom
	// RoutingPrefix.
	//
	// Default is "/internal/tasks/t/". It means generated Cloud Tasks by will
	// have target URL "/internal/tasks/t/<generated-per-task-suffix>".
	//
	// A non-default value may be valuable if you host multiple dispatchers in
	// a single process. This is a niche use case.
	DefaultRoutingPrefix string

	// DefaultTargetHost is a hostname to dispatch Cloud Tasks to by default.
	//
	// Individual task classes may override it with their own specific host.
	//
	// On GAE defaults to the GAE application itself. Elsewhere has no default:
	// if the dispatcher can't figure out where to send the task, the task
	// submission fails.
	DefaultTargetHost string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for PushAs account.
	//
	// Optional on GAE when submitting tasks targeting GAE. Required in all other
	// cases. If not set, task submissions will fail.
	PushAs string

	// AuthorizedPushers is a list of service account emails to accept pushes from
	// in addition to PushAs.
	//
	// This is handy when migrating from one PushAs account to another, or when
	// submitting tasks from one service, but handing them in another.
	//
	// Optional.
	AuthorizedPushers []string

	// SweepInitiationLaunchers is a list of service account emails authorized to
	// launch sweeps via the exposed HTTP endpoint.
	SweepInitiationLaunchers []string

	mu       sync.RWMutex
	clsByID  map[string]*taskClassImpl
	clsByTyp map[protoreflect.MessageType]*taskClassImpl
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

// Sweeper knows how to launch sweeps of the transaction tasks reminders.
type Sweeper interface {
	// startSweep initiates an asynchronous sweep of the reminders keyspace.
	startSweep(ctx context.Context, reminderKeySpaceBytes int) error
}

// TaskKind describes how a task class interoperates with transactions.
type TaskKind int

const (
	// NonTransactional is a task kind for tasks that must be enqueued outside
	// of a transaction.
	NonTransactional TaskKind = 0

	// Transactional is a task kind for tasks that must be enqueued only from
	// a transaction.
	//
	// Using transactional tasks requires setting up a sweeper first, see
	// ModuleOptions.
	Transactional TaskKind = 1

	// FollowsContext is a task kind for tasks that are enqueue transactionally
	// if the context is transactional or non-transactionally otherwise.
	//
	// Using transactional tasks requires setting up a sweeper first, see
	// ModuleOptions.
	FollowsContext TaskKind = 2
)

// TaskClass defines how to handles tasks of a specific proto message type.
type TaskClass struct {
	// ID is unique identifier of this class of tasks.
	//
	// Must match `[a-zA-Z0-9_\-.]{1,100}`.
	//
	// It is used to decide how to deserialize and route the task. Changing IDs of
	// existing task classes is a disruptive operation, make sure the queue is
	// drained first. The dispatcher will permanently fail all Cloud Tasks with
	// unrecognized class IDs.
	//
	// Required.
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
	//
	// Required.
	Prototype proto.Message

	// Kind indicates whether the task requires a transaction to be enqueued.
	//
	// Note that using transactional tasks requires setting up a sweeper first,
	// see ModuleOptions.
	//
	// Default is NonTransactional which means that tasks can be enqueued only
	// outside of transactions.
	Kind TaskKind

	// Queue is a name of Cloud Tasks queue to use for the tasks.
	//
	// It can either be a short name like "default" or a full name like
	// "projects/<project>/locations/<region>/queues/<name>". If it is a full name
	// it must have the above format or RegisterTaskClass would panic.
	//
	// If it is a short queue name, the full queue name will be constructed using
	// dispatcher's CloudProject and CloudRegion if they are set.
	//
	// Required. The queue must exist already.
	Queue string

	// RoutingPrefix is a URL prefix for produced Cloud Tasks.
	//
	// Default is dispatcher's DefaultRoutingPrefix which itself defaults to
	// "/internal/tasks/t/". It means generated Cloud Tasks by default will have
	// target URL "/internal/tasks/t/<generated-per-task-suffix>".
	//
	// A non-default value can be used to route tasks of a particular class to
	// particular processes, assuming the load balancer is configured accordingly.
	RoutingPrefix string

	// TargetHost is a hostname to dispatch Cloud Tasks to.
	//
	// If unset, will use dispatcher's DefaultTargetHost.
	TargetHost string

	// Custom, if given, will be called to generate a custom Cloud Tasks payload
	// (i.e. a to-be-sent HTTP request) from the task's proto payload.
	//
	// Useful for interoperability with existing code that doesn't use dispatcher.
	//
	// It is possible to customize HTTP method, relative URI, headers and the
	// request body this way. Other properties of the task (such as the target
	// host, the queue, the task name, authentication headers) are not
	// customizable.
	//
	// Such produced tasks are generally not routable by the Dispatcher. You'll
	// need to make sure there's an HTTP handler that accepts them.
	//
	// Receives the exact same context as passed to AddTask. If returns nil
	// result, the task will be dispatched as usual.
	Custom func(ctx context.Context, m proto.Message) (*CustomPayload, error)

	// Handler will be called by the dispatcher to execute the tasks.
	//
	// The handler will receive the task's payload as a proto message of the exact
	// same type as the type of Prototype. See Handler doc for more info.
	//
	// Populating this field is equivalent to calling AttachHandler after
	// registering the class. It may be left nil if the current process just wants
	// to submit tasks, but not handle them. Some other process would need to
	// attach the handler then to be able to process tasks.
	//
	// The dispatcher will permanently fail Cloud Tasks if it can't find a handler
	// for them.
	Handler Handler
}

// CustomPayload is returned by TaskClass's Custom, see its doc.
type CustomPayload struct {
	Method      string            // e.g. "GET" or "POST"
	RelativeURI string            // an URI relative to the task's target host
	Headers     map[string]string // HTTP headers to attach
	Body        []byte            // serialized body of the request
}

// TaskClassRef represents a TaskClass registered in a Dispatcher.
type TaskClassRef interface {
	// AttachHandler sets a handler which will be called by the dispatcher to
	// execute the tasks.
	//
	// The handler will receive the task's payload as a proto message of the exact
	// same type as the type of TaskClass's Prototype. See Handler doc for more
	// info.
	//
	// Panics if the class has already a handler attached.
	AttachHandler(h Handler)
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
	// Either Delay or ETA may be set, but not both.
	Delay time.Duration

	// ETA specifies the earliest time a task may be executed.
	//
	// Either Delay or ETA may be set, but not both.
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

// ValidateNamespace returns an error if `n` is not a valid namespace name.
//
// An empty string is a valid namespace (denoting the default namespace). Other
// valid namespaces must start with an ASCII letter or '_', contain only
// ASCII letters, digits or '_', and be less than 50 chars in length.
func ValidateNamespace(n string) error {
	if n != "" && !namespaceRe.MatchString(n) {
		return errors.New("must start with a letter or '_' and contain only letters, numbers and '_'")
	}
	return nil
}

// RegisterTaskClass tells the dispatcher how to route and handle tasks of some
// particular type.
//
// Intended to be called during process startup. Panics if there's already
// a registered task class with the same ID or Prototype.
func (d *Dispatcher) RegisterTaskClass(cls TaskClass) TaskClassRef {
	if !taskClassIDRe.MatchString(cls.ID) {
		panic(fmt.Sprintf("bad TaskClass ID %q", cls.ID))
	}
	if cls.Prototype == nil {
		panic("TaskClass Prototype must be set")
	}
	if cls.Queue == "" {
		panic("TaskClass Queue must be set")
	}
	if strings.ContainsRune(cls.Queue, '/') && !isValidQueue(cls.Queue) {
		panic(fmt.Sprintf("not a valid full queue name %q", cls.Queue))
	}
	if cls.RoutingPrefix != "" && !strings.HasPrefix(cls.RoutingPrefix, "/") {
		panic("TaskClass RoutingPrefix must start with /")
	}

	typ := cls.Prototype.ProtoReflect().Type()

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.clsByID == nil {
		d.clsByID = make(map[string]*taskClassImpl, 1)
	}
	if d.clsByTyp == nil {
		d.clsByTyp = make(map[protoreflect.MessageType]*taskClassImpl, 1)
	}

	if _, ok := d.clsByID[cls.ID]; ok {
		panic(fmt.Sprintf("TaskClass with ID %q is already registered", cls.ID))
	}
	if _, ok := d.clsByTyp[typ]; ok {
		panic(fmt.Sprintf("TaskClass with Prototype %q is already registered", proto.MessageName(cls.Prototype)))
	}

	impl := &taskClassImpl{TaskClass: cls, disp: d, protoType: typ}
	d.clsByID[cls.ID] = impl
	d.clsByTyp[typ] = impl
	return impl
}

// AddTask submits a task for later execution.
//
// The task payload type should match some registered TaskClass. Its ID will
// be used to identify the task class in the serialized Cloud Tasks task body.
//
// At some later time, in some other process, the dispatcher will invoke
// a handler attached to the corresponding TaskClass, based on its ID extracted
// from the task body.
//
// If the given context is transactional, inherits the transaction if allowed
// according to the TaskClass's Kind. A transactional task will eventually be
// submitted to Cloud Tasks if and only if the transaction successfully commits.
// This requires a sweeper instance to be running somewhere, see ModuleOptions.
// Note that a failure to submit the task to Cloud Tasks will not abort
// the transaction.
//
// If the task has a DeduplicationKey and there already was a recent task with
// the same TaskClass ID and DeduplicationKey, silently ignores the added task.
// This works only outside of transactions. Using DeduplicationKey with
// transactional tasks results in an error.
//
// Annotates retriable errors with transient.Tag.
func (d *Dispatcher) AddTask(ctx context.Context, task *Task) (err error) {
	if d.Submitter == nil {
		return errors.New("unconfigured Dispatcher: needs a Submitter")
	}
	cls, req, err := d.prepTask(ctx, task)
	if err != nil {
		return err
	}

	// Examine the context to see if we are inside a transaction.
	db := databases.TxnDB(ctx)
	switch cls.Kind {
	case FollowsContext:
		// do nothing, will use `db` if it is non-nil
	case Transactional:
		if db == nil {
			return errors.Reason("enqueuing of tasks %q must be done from inside a transaction", cls.ID).Err()
		}
	case NonTransactional:
		if db != nil {
			return errors.Reason("enqueuing of tasks %q must be done outside of a transaction", cls.ID).Err()
		}
	default:
		panic(fmt.Sprintf("unrecognized TaskKind %v", cls.Kind))
	}

	// If not inside a transaction, submit the task right away.
	if db == nil {
		ctx, span := startSpan(ctx, "go.chromium.org/luci/server/tq.Enqueue", logging.Fields{
			"cr.dev/class": cls.ID,
			"cr.dev/title": task.Title,
		})
		defer func() { span.End(err) }()
		return internal.Submit(ctx, d.Submitter, req)
	}

	// Named transactional tasks are not supported.
	if req.Task.Name != "" {
		return errors.Reason("when enqueuing %q: can't use DeduplicationKey for a transactional task", cls.ID).Err()
	}

	// Otherwise transactionally commit a reminder and schedule a best-effort
	// post-transaction enqueuing of the actual task. If it fails, the sweeper
	// will eventually discover the reminder and enqueue the task.
	ctx, span := startSpan(ctx, "go.chromium.org/luci/server/tq.Reminder", logging.Fields{
		"cr.dev/class": cls.ID,
		"cr.dev/title": task.Title,
	})
	defer func() { span.End(err) }()
	r, err := d.makeReminder(ctx, req)
	if err != nil {
		return errors.Annotate(err, "failed to prepare a reminder").Err()
	}
	span.Attribute("cr.dev/reminder", r.Id)
	if err := db.SaveReminder(ctx, r); err != nil {
		return errors.Annotate(err, "failed to store a transactional enqueue reminder").Err()
	}

	once := int32(0)
	db.Defer(ctx, func(ctx context.Context) {
		if count := atomic.AddInt32(&once, 1); count > 1 {
			panic("transaction defer has already been called")
		}

		// `ctx` here is an outer non-transactional context.
		var err error
		ctx, span := startSpan(ctx, "go.chromium.org/luci/server/tq.DeferredEnqueue", logging.Fields{
			"cr.dev/class":    cls.ID,
			"cr.dev/title":    task.Title,
			"cr.dev/reminder": r.Id,
		})
		defer func() { span.End(err) }()

		if clock.Now(ctx).After(r.FreshUntil) {
			// Don't enqueue the task if we are too late and the sweeper has likely
			// picked up the reminder already.
			logging.Warningf(ctx, "Happy path DeferredEnqueue has no time to run")
			err = errors.New("happy path DeferredEnqueue has no time to run")
			// TODO: add metric for this
		} else {
			// Actually submit the task and delete the reminder if the task was
			// successfully enqueued or it is a non-retriable failure. This keeps the
			// reminder if the failure was transient, we'll let the sweeper try to
			// submit the task later.
			err = internal.SubmitFromReminder(ctx, d.Submitter, db, r, req)
		}
	})

	return nil
}

// Sweep initiates a sweep of transactional tasks reminders.
//
// It must be called periodically (e.g. once per minute) somewhere in the fleet.
func (d *Dispatcher) Sweep(ctx context.Context) error {
	if d.Sweeper == nil {
		return errors.New("can't sweep: the Sweeper is not set")
	}
	return d.Sweeper.startSweep(ctx, reminderKeySpaceBytes)
}

// SchedulerForTest switches the dispatcher into testing mode.
//
// In this mode tasks are enqueued into a tqtesting.Scheduler, which executes
// them by submitting them back into the Dispatcher (each task in a separate
// goroutine).
//
// SchedulerForTest should be called before first AddTask call.
//
// Returns the instantiated tqtesting.Scheduler. It is in the stopped state.
func (d *Dispatcher) SchedulerForTest() *tqtesting.Scheduler {
	if d.Submitter != nil {
		panic("Dispatcher is already configured to use some submitter")
	}
	if d.CloudProject == "" {
		d.CloudProject = "tq-project"
	}
	if d.CloudRegion == "" {
		d.CloudRegion = "tq-region"
	}
	if d.DefaultTargetHost == "" {
		d.DefaultTargetHost = "127.0.0.1" // not actually used
	}
	if d.PushAs == "" {
		d.PushAs = "tq-pusher@example.com"
	}
	sched := &tqtesting.Scheduler{Executor: directExecutor{d}}
	d.Submitter = sched
	return sched
}

// directExecutor implements tqtesting.Executor via handlePush.
type directExecutor struct {
	d *Dispatcher
}

func (e directExecutor) Execute(ctx context.Context, t *tqtesting.Task, done func(retry bool)) {
	go func() {
		var body []byte
		switch mt := t.Task.MessageType.(type) {
		case *taskspb.Task_HttpRequest:
			body = mt.HttpRequest.Body
		case *taskspb.Task_AppEngineHttpRequest:
			body = mt.AppEngineHttpRequest.Body
		default:
			panic(fmt.Sprintf("Bad task, no payload: %q", t.Task))
		}
		err := e.d.handlePush(ctx, body)
		done(Retry.In(err) || transient.Tag.In(err))
	}()
}

// InstallTasksRoutes installs tasks HTTP routes under the given prefix.
//
// The exposed HTTP endpoints are called by Cloud Tasks service when it is time
// to execute a task.
func (d *Dispatcher) InstallTasksRoutes(r *router.Router, prefix string) {
	if prefix == "" {
		prefix = "/internal/tasks/"
	} else if !strings.HasPrefix(prefix, "/") {
		panic("the prefix should start with /")
	}

	var mw router.MiddlewareChain
	if !d.NoAuth {
		// Tasks are primarily submitted as `PushAs`, but we also accept all
		// `AuthorizedPushers`.
		pushers := append([]string{d.PushAs}, d.AuthorizedPushers...)
		// On GAE X-Appengine-* headers can be trusted. Check we are being called
		// by Cloud Tasks. We don't care by which queue exactly though. It is
		// easier to move tasks between queues that way.
		header := ""
		if d.GAE {
			header = "X-Appengine-Queue"
		}
		mw = authMiddleware(pushers, header)
	}

	// We don't really care about the exact format of URLs. At the same time
	// accepting all requests under InternalRoutingPrefix is necessary for
	// compatibility with "appengine/tq" which used totally different URL format.
	prefix = strings.TrimRight(prefix, "/") + "/*path"
	r.POST(prefix, mw, func(c *router.Context) {
		// TODO(vadimsh): Parse magic headers to get the attempt count.
		body, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			httpReply(c, 500, "Failed to read the request", err)
			return
		}
		switch err := d.handlePush(c.Context, body); {
		case err == nil:
			httpReply(c, 200, "OK", nil)
		case Retry.In(err):
			httpReply(c, 409, "The handler asked for retry", err)
		case transient.Tag.In(err):
			httpReply(c, 500, "Transient error", err)
		default:
			httpReply(c, 202, "Fatal error", err)
		}
	})
}

// InstallSweepRoute installs a route that initiates a sweep.
//
// It may be called periodically (e.g. by Cloud Scheduler) to launch sweeps.
func (d *Dispatcher) InstallSweepRoute(r *router.Router, path string) {
	var mw router.MiddlewareChain
	if !d.NoAuth {
		// On GAE X-Appengine-* headers can be trusted. Check we are being called
		// by Cloud Scheduler.
		header := ""
		if d.GAE {
			header = "X-Appengine-Cron"
		}
		mw = authMiddleware(d.SweepInitiationLaunchers, header)
	}

	r.GET(path, mw, func(c *router.Context) {
		switch err := d.Sweep(c.Context); {
		case err == nil:
			httpReply(c, 200, "OK", nil)
		case transient.Tag.In(err):
			httpReply(c, 500, "Transient error", err)
		default:
			httpReply(c, 202, "Fatal error", err)
		}
	})
}

////////////////////////////////////////////////////////////////////////////////

// defaultHeaders are added to all submitted Cloud Tasks.
var defaultHeaders = map[string]string{"Content-Type": "application/json"}

var (
	// namespaceRe is used to validate Dispatcher.Namespace.
	namespaceRe = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]{0,49}$`)
	// taskClassIDRe is used to validate TaskClass.ID.
	taskClassIDRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]{1,100}$`)
)

const (
	// reminderKeySpaceBytes defines the space of the Reminder Ids.
	//
	// Because Reminder.Id is hex-encoded, actual length is doubled.
	//
	// 16 is chosen is big enough to avoid collisions in practice yet small enough
	// for easier human-debugging of key ranges in queries.
	reminderKeySpaceBytes = 16

	// happyPathMaxDuration caps how long the happy path will be waited for.
	happyPathMaxDuration = time.Minute
)

// startSpan starts a new span and puts `meta` into its attributes and into
// logger fields.
func startSpan(ctx context.Context, title string, meta logging.Fields) (context.Context, trace.Span) {
	ctx = logging.SetFields(ctx, meta)
	ctx, span := trace.StartSpan(ctx, title)
	for k, v := range meta {
		span.Attribute(k, v)
	}
	return ctx, span
}

// prepTask converts a task into Cloud Tasks request.
func (d *Dispatcher) prepTask(ctx context.Context, t *Task) (*taskClassImpl, *taskspb.CreateTaskRequest, error) {
	cls, _, err := d.classByMsg(t.Payload)
	if err != nil {
		return nil, nil, err
	}

	queueID, err := d.queueID(cls.Queue)
	if err != nil {
		return nil, nil, err
	}

	taskID := ""
	if t.DeduplicationKey != "" {
		taskID = queueID + "/tasks/" + cls.taskName(t, d.Namespace)
	}

	var scheduleTime *timestamppb.Timestamp
	switch {
	case !t.ETA.IsZero():
		if t.Delay != 0 {
			return nil, nil, errors.New("bad task: either ETA or Delay should be given, not both")
		}
		scheduleTime = timestamppb.New(t.ETA)
	case t.Delay > 0:
		scheduleTime = timestamppb.New(clock.Now(ctx).Add(t.Delay))
	}

	// E.g. ("example.com", "/internal/tasks/t/<class>[/<title>]").
	// Note: relativeURI is discarded when using custom payload.
	host, relativeURI, err := d.taskTarget(cls, t)
	if err != nil {
		return nil, nil, err
	}

	var payload *CustomPayload
	if cls.Custom != nil {
		if payload, err = cls.Custom(ctx, t.Payload); err != nil {
			return nil, nil, err
		}
	}
	if payload == nil {
		// This is not really a "custom" payload, we are just reusing the struct.
		payload = &CustomPayload{
			Method:      "POST",
			RelativeURI: relativeURI,
			Headers:     defaultHeaders,
		}
		if payload.Body, err = cls.serialize(t); err != nil {
			return nil, nil, err
		}
	}

	method := taskspb.HttpMethod(taskspb.HttpMethod_value[payload.Method])
	if method == 0 {
		return nil, nil, errors.Reason("bad HTTP method %q", payload.Method).Err()
	}
	if !strings.HasPrefix(payload.RelativeURI, "/") {
		return nil, nil, errors.Reason("bad relative URI %q", payload.RelativeURI).Err()
	}

	// We need to populate one of Task.MessageType oneof alternatives. It has
	// unexported type, so we have to instantiate the message now and then mutate
	// it.
	req := &taskspb.CreateTaskRequest{
		Parent: queueID,
		Task: &taskspb.Task{
			Name:         taskID,
			ScheduleTime: scheduleTime,
			// TODO(vadimsh): Make DispatchDeadline configurable?
		},
	}

	// On GAE we by default push to the GAE itself.
	if host == "" && d.GAE {
		req.Task.MessageType = &taskspb.Task_AppEngineHttpRequest{
			AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
				HttpMethod:  method,
				RelativeUri: payload.RelativeURI,
				Headers:     payload.Headers,
				Body:        payload.Body,
			},
		}
		return cls, req, nil
	}

	// Elsewhere we need to know a target host and how to authenticate to it.
	if host == "" {
		return nil, nil, errors.Reason("bad task class %q: no TargetHost", cls.ID).Err()
	}
	if d.PushAs == "" {
		return nil, nil, errors.Reason("unconfigured Dispatcher: PushAs is not set").Err()
	}

	req.Task.MessageType = &taskspb.Task_HttpRequest{
		HttpRequest: &taskspb.HttpRequest{
			HttpMethod: method,
			Url:        "https://" + host + payload.RelativeURI,
			Headers:    payload.Headers,
			Body:       payload.Body,
			AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
				OidcToken: &taskspb.OidcToken{
					ServiceAccountEmail: d.PushAs,
				},
			},
		},
	}
	return cls, req, nil
}

// queueID expands `id` into a full queue name if necessary.
func (d *Dispatcher) queueID(id string) (string, error) {
	if strings.HasPrefix(id, "projects/") {
		return id, nil // already full name
	}
	if d.CloudProject == "" {
		return "", errors.Reason("can't construct full queue name: no cloud project").Err()
	}
	if d.CloudRegion == "" {
		return "", errors.Reason("can't construct full queue name: no cloud region").Err()
	}
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", d.CloudProject, d.CloudRegion, id), nil
}

// taskTarget constructs a target URL for a task.
//
// `host` will be "" if no explicit host is configured anywhere. On GAE this
// means "send the task back to the GAE app". On non-GAE this fails AddTask.
func (d *Dispatcher) taskTarget(cls *taskClassImpl, t *Task) (host string, relativeURI string, err error) {
	if cls.TargetHost != "" {
		host = cls.TargetHost
	} else {
		host = d.DefaultTargetHost
	}

	pfx := cls.RoutingPrefix
	if pfx == "" {
		pfx = d.DefaultRoutingPrefix
	}
	if pfx == "" {
		pfx = "/internal/tasks/t/"
	}

	if !strings.HasPrefix(pfx, "/") {
		return "", "", errors.Reason("bad routing prefix %q: must start with /", pfx).Err()
	}
	if !strings.HasSuffix(pfx, "/") {
		pfx += "/"
	}

	relativeURI = pfx + cls.ID
	if t.Title != "" {
		relativeURI += "/" + t.Title
	}
	return
}

// makeReminder prepares a reminder to be stored in the database to remind
// the sweeper to submit the task if best-effort post-transactional submit
// fails.
//
// Mutates the task request to include an auto-generated task name (the same one
// as stored in the reminder).
func (d *Dispatcher) makeReminder(ctx context.Context, req *taskspb.CreateTaskRequest) (*reminder.Reminder, error) {
	buf := make([]byte, reminderKeySpaceBytes)
	if _, err := io.ReadFull(cryptorand.Get(ctx), buf); err != nil {
		return nil, errors.Annotate(err, "failed to get random bytes").Tag(transient.Tag).Err()
	}

	// Note: length of the generate ID here is different from length of IDs
	// we generate when using DeduplicationKey, so there'll be no collisions
	// between two different sorts of named tasks.
	r := &reminder.Reminder{Id: hex.EncodeToString(buf)}
	req.Task.Name = req.Parent + "/tasks/" + r.Id

	// Bound FreshUntil to at most current context deadline.
	r.FreshUntil = clock.Now(ctx).Add(happyPathMaxDuration)
	if deadline, ok := ctx.Deadline(); ok && r.FreshUntil.After(deadline) {
		// TODO(tandrii): allow propagating custom deadline for the async happy
		// path which won't bind the context's deadline.
		r.FreshUntil = deadline
	}
	r.FreshUntil = r.FreshUntil.UTC().Truncate(reminder.FreshUntilPrecision)

	var err error
	if r.Payload, err = proto.Marshal(req); err != nil {
		return nil, errors.Annotate(err, "failed to marshal the task").Err()
	}
	return r, nil
}

// isValidQueue is true if q looks like "projects/.../locations/.../queues/...".
func isValidQueue(q string) bool {
	chunks := strings.Split(q, "/")
	return len(chunks) == 6 &&
		chunks[0] == "projects" &&
		chunks[1] != "" &&
		chunks[2] == "locations" &&
		chunks[3] != "" &&
		chunks[4] == "queues" &&
		chunks[5] != ""
}

// handlePush handles one incoming task.
//
// Returns errors annotated in the same style as errors from Handler, see its
// doc.
func (d *Dispatcher) handlePush(ctx context.Context, body []byte) error {
	// See taskClassImpl.serialize().
	env := envelope{}
	if err := json.Unmarshal(body, &env); err != nil {
		return errors.Annotate(err, "not a valid JSON body").Err()
	}

	// Find the matching registered task class. Newer tasks always have `class`
	// set. Older ones have `type` instead.
	var cls *taskClassImpl
	var h Handler
	var err error
	if env.Class != "" {
		cls, h, err = d.classByID(env.Class)
	} else if env.Type != "" {
		cls, h, err = d.classByTyp(env.Type)
	} else {
		err = errors.Reason("malformed task body, no class").Err()
	}
	if err != nil {
		return err
	}
	if h == nil {
		return errors.Reason("task class %q exists, but has no handler attached", cls.ID).Err()
	}

	msg, err := cls.deserialize(&env)
	if err != nil {
		return errors.Annotate(err, "malformed body of task class %q", cls.ID).Err()
	}

	return h(ctx, msg)
}

// classByID returns a task class given its ID or an error if no such class.
//
// Reads cls.Handler while under the lock as well, since it may be concurrently
// modified by AttachHandler.
func (d *Dispatcher) classByID(id string) (*taskClassImpl, Handler, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByID[id]; cls != nil {
		return cls, cls.Handler, nil
	}
	return nil, nil, errors.Reason("no task class with ID %q is registered", id).Err()
}

// classByMsg returns a task class given proto message or an error if no
// such class.
//
// Reads cls.Handler while under the lock as well, since it may be concurrently
// modified by AttachHandler.
func (d *Dispatcher) classByMsg(msg proto.Message) (*taskClassImpl, Handler, error) {
	typ := msg.ProtoReflect().Type()
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByTyp[typ]; cls != nil {
		return cls, cls.Handler, nil
	}
	return nil, nil, errors.Reason("no task class matching type %q is registered", typ.Descriptor().FullName()).Err()
}

// classByTyp returns a task class given proto message name or an error if no
// such class.
//
// Reads cls.Handler while under the lock as well, since it may be concurrently
// modified by AttachHandler.
func (d *Dispatcher) classByTyp(typ string) (*taskClassImpl, Handler, error) {
	msgTyp, _ := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typ))
	if msgTyp == nil {
		return nil, nil, errors.Reason("no proto message %q is registered", typ).Err()
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByTyp[msgTyp]; cls != nil {
		return cls, cls.Handler, nil
	}
	return nil, nil, errors.Reason("no task class matching type %q is registered", typ).Err()
}

////////////////////////////////////////////////////////////////////////////////

// taskClassImpl knows how to prepare and handle tasks of a particular class.
type taskClassImpl struct {
	TaskClass
	disp      *Dispatcher
	protoType protoreflect.MessageType
}

// envelope is what we put into all Cloud Tasks.
type envelope struct {
	Class string           `json:"class,omitempty"` // ID of TaskClass
	Type  string           `json:"type,omitempty"`  // for compatibility with appengine/tq
	Body  *json.RawMessage `json:"body"`            // JSONPB-serialized Task.Payload
}

// AttachHandler implements TaskClassRef interface.
func (cls *taskClassImpl) AttachHandler(h Handler) {
	cls.disp.mu.Lock()
	defer cls.disp.mu.Unlock()
	if h == nil {
		panic("The handler must not be nil")
	}
	if cls.Handler != nil {
		panic("The task class has a handler attached already")
	}
	cls.Handler = h
}

// taskName returns a short ID for the task to use to dedup it.
func (cls *taskClassImpl) taskName(t *Task, namespace string) string {
	h := sha256.New()
	h.Write([]byte(namespace))
	h.Write([]byte{0})
	h.Write([]byte(cls.ID))
	h.Write([]byte{0})
	h.Write([]byte(t.DeduplicationKey))
	return hex.EncodeToString(h.Sum(nil))
}

// serialize serializes the task body into JSONPB.
func (cls *taskClassImpl) serialize(t *Task) ([]byte, error) {
	opts := protojson.MarshalOptions{
		Indent:         "\t",
		UseEnumNumbers: true,
	}
	blob, err := opts.Marshal(t.Payload)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize %q", proto.MessageName(t.Payload)).Err()
	}
	raw := json.RawMessage(blob)
	return json.MarshalIndent(envelope{
		Class: cls.ID,
		Type:  string(proto.MessageName(t.Payload)),
		Body:  &raw,
	}, "", "\t")
}

// deserialize instantiates a proto message based on its serialized body.
func (cls *taskClassImpl) deserialize(env *envelope) (proto.Message, error) {
	if env.Body == nil {
		return nil, errors.Reason("no body").Err()
	}
	opts := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	msg := cls.protoType.New().Interface()
	if err := opts.Unmarshal(*env.Body, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

////////////////////////////////////////////////////////////////////////////////

// authMiddleware returns a middleware chain that authorizes requests from given
// callers.
//
// Checks OpenID Connect tokens have us in the audience, and the email in them
// is in `callers` list.
//
// If `header` is set, will also accept requests that have this header,
// regardless of its value. This is used to authorize GAE tasks and cron based
// on `X-AppEngine-*` headers.
func authMiddleware(callers []string, header string) router.MiddlewareChain {
	oidc := auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
		AudienceCheck: openid.AudienceMatchesHost,
	})
	return router.NewMiddlewareChain(oidc, func(c *router.Context, next router.Handler) {
		if header != "" && c.Request.Header.Get(header) != "" {
			next(c)
			return
		}

		if ident := auth.CurrentIdentity(c.Context); ident.Kind() != identity.Anonymous {
			if checkContainsIdent(callers, ident) {
				next(c)
			} else {
				httpReply(c, 403,
					fmt.Sprintf("Caller %q is not authorized", ident),
					errors.Reason("expecting any of %q", callers).Err(),
				)
			}
			return
		}

		var err error
		if header != "" {
			err = errors.Reason("no OIDC token and no %s header", header).Err()
		} else {
			err = errors.Reason("no OIDC token").Err()
		}
		httpReply(c, 403, "Authentication required", err)
	})
}

// checkContainsIdent is true if `ident` emails matches some of `callers`.
func checkContainsIdent(callers []string, ident identity.Identity) bool {
	if ident.Kind() != identity.User {
		return false // we want service accounts
	}
	email := ident.Email()
	for _, c := range callers {
		if email == c {
			return true
		}
	}
	return false
}

// httpReply writes and logs HTTP response.
//
// `msg` is sent to the caller as is. `err` is logged, but not sent.
func httpReply(c *router.Context, code int, msg string, err error) {
	if err != nil {
		logging.Errorf(c.Context, "%s: %s", msg, err)
	}
	http.Error(c.Writer, msg, code)
}
