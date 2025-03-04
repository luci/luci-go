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

package tq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	srvinternal "go.chromium.org/luci/server/internal"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq/internal"
	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/metrics"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

const (
	// TraceContextHeader is name of a header that contains the trace context of
	// a span that produced the task.
	//
	// This header is read only by Dispatcher itself and exists mostly for FYI
	// purposes to help in debugging issues.
	TraceContextHeader = "X-Luci-Tq-Trace-Context"

	// ExpectedETAHeader is the name of a header that indicates when the task was
	// originally expected to run.
	//
	// One use of this header is for measuring latency of task completion.
	ExpectedETAHeader = "X-Luci-Tq-Expected-ETA"
)

// Dispatcher is a registry of task classes that knows how serialize and route
// them.
//
// There's rarely a need to manually create instances of Dispatcher outside of
// Dispatcher's own tests. You should generally use the global Default
// dispatcher which is configured by the tq server module. Methods of the
// default dispatcher (such as RegisterTaskClass and AddTask) are also available
// as lop-level functions, prefer to use them.
//
// The dispatcher needs a way to submit tasks to Cloud Tasks or Cloud PubSub.
// This is the job of Submitter. It lives in the context, so that it can be
// mocked in tests. In production contexts (setup when using the tq server
// module), the submitter is initialized to be CloudSubmitter. Tests will need
// to provide their own submitter (usually via TestingContext).
//
// TODO(vadimsh): Support consuming PubSub tasks, not just producing them.
type Dispatcher struct {
	// Sweeper knows how to sweep transactional tasks reminders.
	//
	// If not set, Sweep calls will fail.
	Sweeper Sweeper

	// Namespace is a namespace for tasks that use DeduplicationKey.
	//
	// This is needed if two otherwise independent deployments share a single
	// Cloud Tasks instance.
	//
	// Used only for Cloud Tasks tasks. Doesn't affect PubSub tasks.
	//
	// Must be valid per ValidateNamespace. Default is "".
	Namespace string

	// GAE is true when running on Appengine.
	//
	// It alters how tasks are submitted and how incoming HTTP requests are
	// authenticated.
	GAE bool

	// DisableAuth can be used to disable authentication on HTTP endpoints.
	//
	// This is useful when running in development mode on localhost or in tests.
	DisableAuth bool

	// CloudProject is ID of a project to use to construct full resource names.
	//
	// If not set, "default" will be used, which is pretty useless outside of
	// tests.
	CloudProject string

	// CloudRegion is a ID of a region to use to construct full resource names.
	//
	// If not set, "default" will be used, which is pretty useless outside of
	// tests.
	CloudRegion string

	// DefaultRoutingPrefix is a URL prefix for produced Cloud Tasks.
	//
	// Used only for Cloud Tasks tasks whose TaskClass doesn't provide some custom
	// RoutingPrefix. Doesn't affect PubSub tasks.
	//
	// Default is "/internal/tasks/t/". It means generated Cloud Tasks by will
	// have target URL "/internal/tasks/t/<generated-per-task-suffix>".
	//
	// A non-default value may be valuable if you host multiple dispatchers in
	// a single process. This is a niche use case.
	DefaultRoutingPrefix string

	// DefaultTargetHost is a hostname to dispatch Cloud Tasks to by default.
	//
	// Individual Cloud Tasks task classes may override it with their own specific
	// host. Doesn't affect PubSub tasks.
	//
	// On GAE defaults to the GAE application itself. Elsewhere defaults to
	// "127.0.0.1", which is pretty useless outside of tests.
	DefaultTargetHost string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// Used only for Cloud Tasks tasks. Doesn't affect PubSub tasks.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for PushAs account.
	//
	// Optional on GAE when submitting tasks targeting GAE. Elsewhere defaults to
	// "default@example.com", which is pretty useless outside of tests.
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

// Sweeper knows how sweep transaction tasks reminders.
type Sweeper interface {
	// sweep either performs the full sweep itself or schedules a task to do it.
	sweep(ctx context.Context, s Submitter, reminderKeySpaceBytes int) error
}

// TaskKind describes how a task class interoperates with transactions.
type TaskKind int

const (
	// NonTransactional is a task kind for tasks that must be enqueued outside
	// of a transaction.
	NonTransactional TaskKind = 1

	// Transactional is a task kind for tasks that must be enqueued only from
	// a transaction.
	Transactional TaskKind = 2

	// FollowsContext is a task kind for tasks that are enqueue transactionally
	// if the context is transactional or non-transactionally otherwise.
	FollowsContext TaskKind = 3
)

// TaskClass defines how to treat tasks of a specific proto message type.
//
// It assigns some stable ID to a proto message kind and also defines how tasks
// of this kind should be submitted and routed.
//
// The are two backends for tasks: Cloud Tasks and Cloud PubSub. Which one to
// use for a particular task class is defined via mutually exclusive Queue and
// Topic fields.
//
// Refer to Google Cloud documentation for all semantic differences between
// Cloud Tasks and Cloud PubSub. One important difference is that Cloud PubSub
// tasks can't be deduplicated and thus the handler must expect to receive
// duplicates.
type TaskClass struct {
	// ID is unique identifier of this class of tasks.
	//
	// Must match `[a-zA-Z0-9_\-.]{1,100}`.
	//
	// It is used to decide how to deserialize and route the task. Changing IDs of
	// existing task classes is a disruptive operation, make sure the queue is
	// drained first. The dispatcher will reject Cloud Tasks with unrecognized
	// class IDs with HTTP 404 error (causing Cloud Tasks to retry them later).
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
	// reject Cloud Tasks with bodies it can't deserialize with HTTP 400 error
	// (causing Cloud Tasks to retry them later).
	//
	// Required.
	Prototype proto.Message

	// Kind indicates whether the task requires a transaction to be enqueued.
	//
	// Note that using transactional tasks requires setting up a sweeper first
	// and importing a module that implements transactions support for the
	// database you are using. See "Transactional tasks" section above.
	//
	// Required. Pick one of NonTransactional, Transactional or FollowsContext.
	Kind TaskKind

	// Queue is a name of Cloud Tasks queue to use for the tasks.
	//
	// If set, indicates the task should be submitted through Cloud Tasks API.
	// The queue must exist already. It can either be a short name like "default"
	// or a full name like "projects/<project>/locations/<region>/queues/<name>".
	// If it is a full name, it must have the above format or RegisterTaskClass
	// would panic. If it is a short queue name, the full queue name will be
	// constructed using dispatcher's CloudProject and CloudRegion if they are
	// set.
	//
	// Can't be set together with QueuePicker or Topic.
	Queue string

	// QueuePicker is a callback that picks a queue for each individual task.
	//
	// It is an alternative to specifying a single queue via Queue. It can be used
	// to distribute tasks across multiple queues, for example to spread the load
	// or to use different queues for tasks with different priorities.
	//
	// Receives Task with Payload proto.Message having the same underlying type as
	// Prototype. It can be type cast to a concrete type and examined, if
	// necessary.
	//
	// Must be lightweight, will be called from within AddTask implementation for
	// each added task, receiving the context passed to AddTask. The returned
	// queue name should be in the same format as Queue, i.e. either be a short
	// queue name or a full queue name. See Queue for details.
	//
	// Can't be set together with Queue or Topic.
	QueuePicker func(context.Context, *Task) (string, error)

	// Topic is a name of PubSub topic to use for the tasks.
	//
	// If set, indicates the task should be submitted through Cloud PubSub API.
	// The topic must exist already. It can either be a short name like "tasks" or
	// a full name like "projects/<project>/topics/<name>". If it is a full name,
	// it must have the above format or RegisterTaskClass would panic.
	//
	// Can't be set together with Queue or QueuePicker.
	Topic string

	// RoutingPrefix is a URL prefix for produced Cloud Tasks.
	//
	// Can only be used for Cloud Tasks task (i.e. only if Queue is also set).
	//
	// Default is dispatcher's DefaultRoutingPrefix which itself defaults to
	// "/internal/tasks/t/". It means generated Cloud Tasks by default will have
	// target URL "/internal/tasks/t/<generated-per-task-suffix>".
	//
	// A non-default value can be used to route Cloud Tasks tasks of a particular
	// class to particular processes, assuming the load balancer is configured
	// accordingly.
	RoutingPrefix string

	// TargetHost is a hostname to dispatch Cloud Tasks to.
	//
	// Can only be used for Cloud Tasks task (i.e. only if Queue is also set).
	//
	// If unset, will use dispatcher's DefaultTargetHost.
	TargetHost string

	// Quiet, if set, instructs the dispatcher not to log bodies of tasks.
	Quiet bool

	// QuietOnError, if set, instructs the dispatcher not to log errors returned
	// by the task handler.
	//
	// This is useful if task handler wants to do its own custom error logging.
	QuietOnError bool

	// Custom, if given, will be called to generate a custom payload from the
	// task's proto payload.
	//
	// Useful for interoperability with existing code that doesn't use dispatcher
	// or if the tasks are meant to be consumed in some custom way. You'll need to
	// setup the consumer manually, the Dispatcher doesn't know how to handle
	// tasks with custom payload.
	//
	// For Cloud Tasks tasks it is possible to customize HTTP method, relative
	// URI, headers and the request body this way. Other properties of the task
	// (such as the target host, the queue, the task name, authentication headers)
	// are not customizable.
	//
	// For PubSub tasks it is possible to customize only task's body and
	// attributes (via CustomPayload.Meta). Other fields in CustomPayload are
	// ignored.
	//
	// Receives the exact same context as passed to AddTask. If returns nil
	// result, the task will be submitted as usual.
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
	// The dispatcher will permanently fail tasks if it can't find a handler for
	// them.
	Handler Handler
}

// CustomPayload is returned by TaskClass's Custom, see its doc.
type CustomPayload struct {
	Method      string            // e.g. "GET" or "POST", Cloud Tasks only
	RelativeURI string            // an URI relative to the task's target host, Cloud Tasks only
	Meta        map[string]string // HTTP headers or message attributes to attach
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

	// Definition returns the original task class definition.
	Definition() TaskClass
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
	// Can be used only with Cloud Tasks tasks, since PubSub doesn't support
	// deduplication during enqueuing.
	//
	// Named tasks can only be used outside of transactions.
	DeduplicationKey string

	// Title is optional string that identifies the task in server logs.
	//
	// For Cloud Tasks it will also show up as a suffix in task handler URL. It
	// exists exclusively to simplify reading server logs. It serves no other
	// purpose! In particular, it is *not* a task name.
	//
	// Handlers won't ever see it. Pass all information through the payload.
	Title string

	// Delay specifies the duration the Cloud Tasks service must wait before
	// attempting to execute the task.
	//
	// Can be used only with Cloud Tasks tasks. Either Delay or ETA may be set,
	// but not both.
	Delay time.Duration

	// ETA specifies the earliest time a task may be executed.
	//
	// Can be used only with Cloud Tasks tasks. Either Delay or ETA may be set,
	// but not both.
	ETA time.Time
}

var (
	// Fatal is an error tag used to indicate that the handler wants the task to
	// be dropped due to unrecoverable failure.
	//
	// See Handler doc for more details.
	Fatal = errtag.Make("the task should be dropped due to fatal failure", true)

	// Ignore is an error tag used to indicate that the handler wants the task to
	// be dropped as no longer needed.
	//
	// See Handler doc for more details.
	Ignore = errtag.Make("the task should be dropped as no longer needed", true)
)

// Used to override HTTP status of some errors.
var (
	httpStatusTag = errtag.Make("http status override", 0)
	httpStatus404 = httpStatusTag.WithDefault(404)
	httpStatus400 = httpStatusTag.WithDefault(400)
)

// quietOnError is an error tag used to implement TaskClass.QuietOnError.
var quietOnError = errtag.Make("QuietOnError", true)

// Handler is called to handle one enqueued task.
//
// If Handler returns an error tagged with Ignore tag, the task will be dropped
// with HTTP 204 reply to Cloud Tasks. This is useful when task is no longer
// needed yet it's desirable to distinguish such a case from the normal case
// for monitoring purposes (e.g. in emitted logs or tsmon metrics).
//
// If Handler returns an error tagged with Fatal tag, the task will be dropped with
// HTTP 202 reply to Cloud Tasks. This should be rarely used.
//
// Otherwise, the task will be retried later (per the queue configuration) with
// HTTP 429 reply.
//
// Errors tagged with transient.Tag result in HTTP 500 replies. They also
// trigger a retry.
type Handler func(ctx context.Context, payload proto.Message) error

// ExecutionInfo is parsed from incoming task's metadata.
//
// It is accessible from within task handlers via TaskExecutionInfo(ctx).
type ExecutionInfo struct {
	// ExecutionCount is 0 on a first delivery attempt and increased by 1 for each
	// failed attempt.
	ExecutionCount int

	// TaskID is the ID of the task in the underlying backend service.
	//
	// For Cloud Task, it is `X-CloudTasks-TaskName`.
	// For PubSub, it is `messageID`.
	TaskID string

	// Queue is the name of the Cloud Tasks queue that delivered the task.
	//
	// Empty for PubSub tasks.
	Queue string

	taskRetryReason       string    // X-CloudTasks-TaskRetryReason
	taskPreviousResponse  string    // X-CloudTasks-TaskPreviousResponse
	submitterTraceContext string    // see TraceContextHeader
	expectedETA           time.Time // see ExpectedETAHeader
}

var executionInfoKey = "go.chromium.org/luci/server/tq.ExecutionInfo"

// TaskExecutionInfo returns information about the currently executing task.
//
// Returns nil if called not from a task handler.
func TaskExecutionInfo(ctx context.Context) *ExecutionInfo {
	info, _ := ctx.Value(&executionInfoKey).(*ExecutionInfo)
	return info
}

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
	if cls.RoutingPrefix != "" && !strings.HasPrefix(cls.RoutingPrefix, "/") {
		panic("TaskClass RoutingPrefix must start with /")
	}
	if cls.Kind == 0 {
		panic("TaskClass Kind is required")
	}

	usesQueues := cls.Queue != "" || cls.QueuePicker != nil
	if cls.Queue != "" && cls.QueuePicker != nil {
		panic("TaskClass must have either Queue or QueuePicker set, not both")
	}

	var backend taskBackend
	switch {
	case !usesQueues && cls.Topic == "":
		panic("TaskClass must have either Queue, QueuePicker or Topic set")
	case usesQueues && cls.Topic != "":
		panic("TaskClass must have either Queue/QueuePicker or Topic set, not both")
	case usesQueues:
		backend = backendCloudTasks
		if cls.Queue != "" {
			if !isShortQueueName(cls.Queue) && !isValidFullQueueName(cls.Queue) {
				panic(fmt.Sprintf("not a valid queue name %q", cls.Queue))
			}
		}
	case cls.Topic != "":
		backend = backendPubSub
		if strings.ContainsRune(cls.Topic, '/') && !isValidTopic(cls.Topic) {
			panic(fmt.Sprintf("not a valid full topic name %q", cls.Topic))
		}
		if cls.RoutingPrefix != "" {
			panic("PubSub tasks do not support RoutingPrefix")
		}
		if cls.TargetHost != "" {
			panic("PubSub tasks do not support TargetHost")
		}
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

	impl := &taskClassImpl{
		TaskClass: cls,
		disp:      d,
		protoType: typ,
		backend:   backend,
	}
	d.clsByID[cls.ID] = impl
	d.clsByTyp[typ] = impl
	return impl
}

// TaskClassRef returns a task class reference given its ID or nil if no such
// task class is registered.
func (d *Dispatcher) TaskClassRef(id string) TaskClassRef {
	impl, _, _ := d.classByID(id)
	if impl == nil {
		return nil
	}
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
	sub, err := currentSubmitter(ctx)
	if err != nil {
		return err
	}

	// Start a span annotated with the task's class.
	cls, _, err := d.classByMsg(task.Payload)
	if err != nil {
		return err
	}
	ctx, span := startSpan(ctx, "go.chromium.org/luci/server/tq.AddTask", map[string]string{
		"cr.dev.class": cls.ID,
		"cr.dev.title": task.Title,
	})
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Prepare a raw request. We'll either submit it right away (for non-tx
	// tasks), or attach it to a reminder and store in the DB for later handling.
	payload, err := d.prepPayload(ctx, cls, task)
	if err != nil {
		return err
	}

	// Examine the context to see if we are inside a transaction.
	txndb := db.TxnDB(ctx)
	switch cls.Kind {
	case FollowsContext:
		// do nothing, will use `txndb` if it is non-nil
	case Transactional:
		if txndb == nil {
			if !db.Configured() {
				return errors.Reason("enqueuing of tasks %q requires transactions support, "+
					"see https://pkg.go.dev/go.chromium.org/luci/server/tq#hdr-Transactional_tasks", cls.ID).Err()
			}
			return errors.Reason("enqueuing of tasks %q must be done from inside a transaction", cls.ID).Err()
		}
	case NonTransactional:
		if txndb != nil {
			return errors.Reason("enqueuing of tasks %q must be done outside of a transaction", cls.ID).Err()
		}
	default:
		panic(fmt.Sprintf("unrecognized TaskKind %v", cls.Kind))
	}

	// If not inside a transaction, submit the task right away.
	if txndb == nil {
		return internal.Submit(ctx, sub, payload, internal.TxnPathNone)
	}

	// Named transactional tasks are not supported.
	if task.DeduplicationKey != "" {
		return errors.Reason("when enqueuing %q: can't use DeduplicationKey for a transactional task", cls.ID).Err()
	}

	// Otherwise transactionally commit a reminder and schedule a best-effort
	// post-transaction enqueuing of the actual task. If it fails, the sweeper
	// will eventually discover the reminder and enqueue the task. Note that this
	// modifies `payload` with the reminder's ID.
	r, err := d.attachToReminder(ctx, payload)
	if err != nil {
		return errors.Annotate(err, "failed to prepare a reminder").Err()
	}
	span.SetAttributes(attribute.String("cr.dev.reminder", r.ID))
	if err := txndb.SaveReminder(ctx, r); err != nil {
		return errors.Annotate(err, "failed to store a transactional enqueue reminder").Err()
	}

	once := int32(0)
	txndb.Defer(ctx, func(ctx context.Context) {
		if count := atomic.AddInt32(&once, 1); count > 1 {
			panic("transaction defer has already been called")
		}

		// `ctx` here is an outer non-transactional context.
		var err error
		ctx, span := startSpan(ctx, "go.chromium.org/luci/server/tq.PostTxn", map[string]string{
			"cr.dev.class":    cls.ID,
			"cr.dev.title":    task.Title,
			"cr.dev.reminder": r.ID,
		})
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()

		// Attempt to submit the task right away if the reminder is still fresh.
		err = internal.ProcessReminderPostTxn(ctx, sub, txndb, r)
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
	sub, err := currentSubmitter(ctx)
	if err != nil {
		return err
	}
	return d.Sweeper.sweep(ctx, sub, reminderKeySpaceBytes)
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
	if !d.DisableAuth {
		// Tasks are primarily submitted as `PushAs`, but we also accept all
		// `AuthorizedPushers`.
		pushers := append([]string{d.PushAs}, d.AuthorizedPushers...)
		// On GAE X-Appengine-* headers can be trusted. Check we are being called
		// by Cloud Tasks. We don't care by which queue exactly though. It is
		// easier to move tasks between queues that way.
		header := ""
		if d.GAE {
			header = "X-Appengine-Queuename"
		}
		mw = srvinternal.CloudAuthMiddleware(pushers, header, func(c *router.Context) {
			metrics.ServerRejectedCount.Add(c.Request.Context(), 1, "", "auth")
		})
	}

	// We don't really care about the exact format of URLs. At the same time
	// accepting all requests under InternalRoutingPrefix is necessary for
	// compatibility with "appengine/tq" which used totally different URL format.
	prefix = strings.TrimRight(prefix, "/") + "/*path"
	r.POST(prefix, mw, func(c *router.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpReply(c, 500, "Failed to read the request", err)
		} else {
			replyWithErr(c, d.handlePush(c.Request.Context(), body, parseHeaders(c.Request.Header)))
		}
	})
}

// InstallSweepRoute installs a route that initiates a sweep.
//
// It may be called periodically (e.g. by Cloud Scheduler) to launch sweeps.
func (d *Dispatcher) InstallSweepRoute(r *router.Router, path string) {
	var mw router.MiddlewareChain
	if !d.DisableAuth {
		// On GAE X-Appengine-* headers can be trusted. Check we are being called
		// by Cloud Scheduler.
		header := ""
		if d.GAE {
			header = "X-Appengine-Cron"
		}
		mw = srvinternal.CloudAuthMiddleware(d.SweepInitiationLaunchers, header, nil)
	}

	r.GET(path, mw, func(c *router.Context) {
		err := d.Sweep(c.Request.Context())
		if err != nil && !transient.Tag.In(err) {
			err = Fatal.Apply(err)
		}
		replyWithErr(c, err)
	})
}

// ReportMetrics writes gauge metrics to tsmon.
//
// This should be called before tsmon flush. By reporting them only here, we
// can avoid hitting tsmon state every time some gauge value changes (which
// can happen very often).
func (d *Dispatcher) ReportMetrics(ctx context.Context) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for id, cls := range d.clsByID {
		metrics.ServerRunning.Set(ctx, int64(atomic.LoadInt32(&cls.running)), id)
	}
}

////////////////////////////////////////////////////////////////////////////////

var (
	// namespaceRe is used to validate Dispatcher.Namespace.
	namespaceRe = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]{0,49}$`)
	// taskClassIDRe is used to validate TaskClass.ID.
	taskClassIDRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]{1,100}$`)
	// tracer is used to report tracing spans.
	tracer = otel.Tracer("go.chromium.org/luci/server/tq")
)

const (
	// reminderKeySpaceBytes defines the space of the Reminder Ids.
	//
	// Because Reminder.ID is hex-encoded, actual length is doubled.
	//
	// 16 is chosen is big enough to avoid collisions in practice yet small enough
	// for easier human-debugging of key ranges in queries.
	reminderKeySpaceBytes = 16

	// happyPathMaxDuration caps how long the happy path will be waited for.
	happyPathMaxDuration = time.Minute
)

// defaultHeaders returns headers to add to all submitted tasks.
func defaultHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

// startSpan starts a new span and puts `meta` into its attributes and into
// logger fields.
func startSpan(ctx context.Context, title string, meta map[string]string) (context.Context, trace.Span) {
	attrs := make([]attribute.KeyValue, 0, len(meta))
	fields := make(logging.Fields, len(meta))
	for k, v := range meta {
		attrs = append(attrs, attribute.String(k, v))
		fields[k] = v
	}
	return tracer.Start(logging.SetFields(ctx, fields), title, trace.WithAttributes(attrs...))
}

// prepPayload converts a task into a reminder.Payload.
func (d *Dispatcher) prepPayload(ctx context.Context, cls *taskClassImpl, t *Task) (*reminder.Payload, error) {
	payload := &reminder.Payload{
		TaskClass: cls.ID,
		Created:   clock.Now(ctx),
		Raw:       t.Payload, // used on a happy path only (essentially only in tests)
	}
	var err error
	switch cls.backend {
	case backendCloudTasks:
		payload.CreateTaskRequest, err = d.prepCloudTasksRequest(ctx, cls, t)
	case backendPubSub:
		payload.PublishRequest, err = d.prepPubSubRequest(ctx, cls, t)
	default:
		panic("impossible")
	}
	return payload, err
}

// prepCloudTasksRequest prepares Cloud Tasks request based on a *Task.
func (d *Dispatcher) prepCloudTasksRequest(ctx context.Context, cls *taskClassImpl, t *Task) (*taskspb.CreateTaskRequest, error) {
	var queue string
	switch {
	case cls.Queue != "":
		queue = cls.Queue
	case cls.QueuePicker != nil:
		var err error
		if queue, err = cls.QueuePicker(ctx, t); err != nil {
			return nil, err
		}
	default:
		panic("impossible, backendCloudTasks tasks have either Queue or QueuePicker set")
	}
	queueID, err := d.queueID(queue)
	if err != nil {
		return nil, err
	}

	taskID := ""
	if t.DeduplicationKey != "" {
		taskID = queueID + "/tasks/" + cls.taskName(t, d.Namespace)
	}

	var scheduleTime *timestamppb.Timestamp
	switch {
	case !t.ETA.IsZero():
		if t.Delay != 0 {
			return nil, errors.New("bad task: either ETA or Delay should be given, not both")
		}
		scheduleTime = timestamppb.New(t.ETA)
	case t.Delay > 0:
		scheduleTime = timestamppb.New(clock.Now(ctx).Add(t.Delay))
	}

	// E.g. ("example.com", "/internal/tasks/t/<class>[/<title>]").
	// Note: relativeURI is discarded when using custom payload.
	host, relativeURI, err := d.taskTarget(cls, t)
	if err != nil {
		return nil, err
	}

	var payload *CustomPayload
	if cls.Custom != nil {
		if payload, err = cls.Custom(ctx, t.Payload); err != nil {
			return nil, err
		}
	}
	if payload == nil {
		// This is not really a "custom" payload, we are just reusing the struct.
		payload = &CustomPayload{
			Method:      "POST",
			RelativeURI: relativeURI,
			Meta:        defaultHeaders(),
		}
		if payload.Body, err = cls.serialize(t); err != nil {
			return nil, err
		}
	} else {
		// We'll likely be mutating the headers below, make a copy.
		meta := make(map[string]string, len(payload.Meta))
		for k, v := range payload.Meta {
			meta[k] = v
		}
		payload.Meta = meta
	}

	// Inject tracing headers.
	if traceCtx := traceContext(ctx); traceCtx != "" {
		payload.Meta[TraceContextHeader] = traceCtx
	}

	// Inject magic header with ETA.
	if scheduleTime == nil {
		payload.Meta[ExpectedETAHeader] = makeETAHeader(clock.Now(ctx))
	} else {
		payload.Meta[ExpectedETAHeader] = makeETAHeader(scheduleTime.AsTime())
	}

	method := taskspb.HttpMethod(taskspb.HttpMethod_value[payload.Method])
	if method == 0 {
		return nil, errors.Reason("bad HTTP method %q", payload.Method).Err()
	}
	if !strings.HasPrefix(payload.RelativeURI, "/") {
		return nil, errors.Reason("bad relative URI %q", payload.RelativeURI).Err()
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
				Headers:     payload.Meta,
				Body:        payload.Body,
			},
		}
		return req, nil
	}

	// Elsewhere pick up some defaults mostly used only in tests.
	if host == "" {
		host = "127.0.0.1"
	}
	pushAs := d.PushAs
	if d.PushAs == "" {
		pushAs = "default@example.com"
	}

	req.Task.MessageType = &taskspb.Task_HttpRequest{
		HttpRequest: &taskspb.HttpRequest{
			HttpMethod: method,
			Url:        "https://" + host + payload.RelativeURI,
			Headers:    payload.Meta,
			Body:       payload.Body,
			AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
				OidcToken: &taskspb.OidcToken{
					ServiceAccountEmail: pushAs,
				},
			},
		},
	}
	return req, nil
}

// makeETAHeader converts the given time into a decimal string representing
// the number of seconds since the Unix epoch with microsecond resolution.
func makeETAHeader(t time.Time) string {
	mics := t.UnixNano() / 1000
	return fmt.Sprintf("%d.%06d", mics/1e6, mics%1e6)
}

// queueID expands `id` into a full queue name if necessary.
//
// Validates `id` as a full queue name if it looks like it.
func (d *Dispatcher) queueID(id string) (string, error) {
	if isShortQueueName(id) {
		project := d.CloudProject
		if project == "" {
			project = "default"
		}
		region := d.CloudRegion
		if region == "" {
			region = "default"
		}
		return fmt.Sprintf("projects/%s/locations/%s/queues/%s", project, region, id), nil
	}
	if !isValidFullQueueName(id) {
		return "", errors.Reason("not a valid queue name: %q", id).Err()
	}
	return id, nil
}

// taskTarget constructs a target URL for a task.
//
// `host` will be "" if no explicit host is configured anywhere. On GAE this
// means "send the task back to the GAE app". On non-GAE this indicates to use
// default "127.0.0.1" which is really usable only in tests.
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
	switch {
	case t.Title == "":
		return
	case strings.ContainsRune(t.Title, ' '):
		return "", "", errors.Reason("bad task title %q: must not contain spaces", t.Title).Err()
	case len(relativeURI)+1+len(t.Title) > 2083:
		return "", "", errors.Reason("bad task title %q: too long;"+
			" must not exceed 2083 characters when combined with %q", t.Title, relativeURI).Err()
	default:
		relativeURI += "/" + t.Title
		return
	}
}

// prepPubSubRequest prepares Cloud PubSub request based on a *Task.
func (d *Dispatcher) prepPubSubRequest(ctx context.Context, cls *taskClassImpl, t *Task) (*pubsubpb.PublishRequest, error) {
	if t.DeduplicationKey != "" {
		return nil, errors.New("can't use DeduplicationKey with PubSub tasks")
	}
	if t.Delay != 0 || !t.ETA.IsZero() {
		return nil, errors.New("can't use Delay or ETA with PubSub tasks")
	}

	topicID, err := d.topicID(cls.Topic)
	if err != nil {
		return nil, err
	}

	var payload *CustomPayload
	if cls.Custom != nil {
		if payload, err = cls.Custom(ctx, t.Payload); err != nil {
			return nil, err
		}
	}
	if payload == nil {
		// This is not really a "custom" payload, we are just reusing the struct.
		payload = &CustomPayload{}
		if payload.Body, err = cls.serialize(t); err != nil {
			return nil, err
		}
	}

	msg := &pubsubpb.PubsubMessage{
		Data:       payload.Body,
		Attributes: make(map[string]string, len(payload.Meta)+1),
	}
	for k, v := range payload.Meta {
		msg.Attributes[k] = v
	}
	if traceCtx := traceContext(ctx); traceCtx != "" {
		msg.Attributes[TraceContextHeader] = traceCtx
	}

	return &pubsubpb.PublishRequest{
		Topic:    topicID,
		Messages: []*pubsubpb.PubsubMessage{msg},
	}, nil
}

// topicID expands `id` into a full topic name if necessary.
func (d *Dispatcher) topicID(id string) (string, error) {
	if strings.HasPrefix(id, "projects/") {
		return id, nil // already full name
	}
	project := d.CloudProject
	if project == "" {
		project = "default"
	}
	return fmt.Sprintf("projects/%s/topics/%s", project, id), nil
}

// attachToReminder makes a reminder and attaches the payload to it, thus
// mutating the payload with reminder's ID.
//
// Returns the constructed reminder. It will eventually be stored in the
// database to remind the sweeper to submit the task if best-effort
// post-transactional submit fails.
func (d *Dispatcher) attachToReminder(ctx context.Context, payload *reminder.Payload) (*reminder.Reminder, error) {
	buf := make([]byte, reminderKeySpaceBytes)
	if _, err := io.ReadFull(cryptorand.Get(ctx), buf); err != nil {
		return nil, errors.Annotate(err, "failed to get random bytes").Tag(transient.Tag).Err()
	}

	// Note: length of the generated ID here is different from the length of IDs
	// we generate when using DeduplicationKey, so there'll be no collisions
	// between two different sorts of named tasks.
	r := &reminder.Reminder{ID: hex.EncodeToString(buf)}

	// Bound FreshUntil to at most current context deadline.
	r.FreshUntil = clock.Now(ctx).Add(happyPathMaxDuration)
	if deadline, ok := ctx.Deadline(); ok && r.FreshUntil.After(deadline) {
		// TODO(tandrii): allow propagating custom deadline for the async happy
		// path which won't bind the context's deadline.
		r.FreshUntil = deadline
	}
	r.FreshUntil = r.FreshUntil.UTC().Truncate(reminder.FreshUntilPrecision)

	return r, r.AttachPayload(payload)
}

// isShortQueueName is true if q looks like a short queue name.
func isShortQueueName(q string) bool {
	return q != "" && !strings.ContainsRune(q, '/')
}

// isValidFullQueueName is true if q is "projects/.../locations/.../queues/...".
func isValidFullQueueName(q string) bool {
	chunks := strings.Split(q, "/")
	return len(chunks) == 6 &&
		chunks[0] == "projects" &&
		chunks[1] != "" &&
		chunks[2] == "locations" &&
		chunks[3] != "" &&
		chunks[4] == "queues" &&
		chunks[5] != ""
}

// isValidTopic is true if t looks like "projects/.../topics/...".
func isValidTopic(t string) bool {
	chunks := strings.Split(t, "/")
	return len(chunks) == 4 &&
		chunks[0] == "projects" &&
		chunks[1] != "" &&
		chunks[2] == "topics" &&
		chunks[3] != ""
}

// handlePush handles one incoming task.
//
// Returns errors annotated in the same style as errors from Handler, see its
// doc.
func (d *Dispatcher) handlePush(ctx context.Context, body []byte, info ExecutionInfo) error {
	// See taskClassImpl.serialize().
	env := envelope{}
	if err := json.Unmarshal(body, &env); err != nil {
		metrics.ServerRejectedCount.Add(ctx, 1, info.Queue, "bad_request")
		return errors.Annotate(err, "not a valid JSON body").Tag(httpStatus400).Err()
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
		err = errors.Reason("malformed task body, no class").Tag(httpStatus400).Err()
	}
	if err != nil {
		logging.Debugf(ctx, "TQ: %s", body)
		metrics.ServerRejectedCount.Add(ctx, 1, info.Queue, "unknown_class")
		return err
	}

	if !cls.Quiet {
		logging.Debugf(ctx, "TQ: %s", body)
		if info.submitterTraceContext != "" {
			logging.Debugf(ctx, "TQ: submitted at %s", info.submitterTraceContext)
		}
		if info.ExecutionCount != 0 {
			logging.Debugf(ctx, "TQ: this is a retry: %d previous attempt(s) already failed", info.ExecutionCount)
			if info.taskRetryReason != "" || info.taskPreviousResponse != "" {
				logging.Debugf(ctx, "TQ: the previous attempt failed with %s: %s", info.taskPreviousResponse, info.taskRetryReason)
			}
		}
	}

	if h == nil {
		metrics.ServerRejectedCount.Add(ctx, 1, info.Queue, "no_handler")
		return errors.Reason("task class %q exists, but has no handler attached", cls.ID).Tag(httpStatus404).Err()
	}

	msg, err := cls.deserialize(&env)
	if err != nil {
		metrics.ServerRejectedCount.Add(ctx, 1, info.Queue, "bad_payload")
		return errors.Annotate(err, "malformed body of task class %q", cls.ID).Tag(httpStatus400).Err()
	}

	atomic.AddInt32(&cls.running, 1)
	defer atomic.AddInt32(&cls.running, -1)

	ctx = context.WithValue(ctx, &executionInfoKey, &info)

	start := clock.Now(ctx)
	err = h(ctx, msg)
	dur := clock.Now(ctx).Sub(start)

	result := "OK"
	switch {
	case Fatal.In(err):
		result = "fatal"
	case Ignore.In(err):
		result = "ignore"
	case transient.Tag.In(err):
		result = "transient"
	case err != nil:
		result = "retry"
	}

	retry := info.ExecutionCount
	if retry > metrics.MaxRetryFieldValue {
		retry = metrics.MaxRetryFieldValue
	}

	metrics.ServerHandledCount.Add(ctx, 1, cls.ID, info.Queue, result, retry)
	metrics.ServerDurationMS.Add(ctx, float64(dur.Milliseconds()), cls.ID, info.Queue, result)
	if !info.expectedETA.IsZero() {
		latency := clock.Since(ctx, info.expectedETA).Milliseconds()
		if latency < 0 {
			latency = 0
		}
		metrics.ServerTaskLatency.Add(ctx, float64(latency), cls.ID, info.Queue, result, retry)
	}

	if err != nil && cls.QuietOnError {
		err = quietOnError.Apply(err)
	}
	return err
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
	return nil, nil, errors.Reason("no task class with ID %q is registered", id).Tag(httpStatus404).Err()
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
	return nil, nil, errors.Reason("no task class matching type %q is registered", typ.Descriptor().FullName()).Tag(httpStatus404).Err()
}

// classByTyp returns a task class given proto message name or an error if no
// such class.
//
// Reads cls.Handler while under the lock as well, since it may be concurrently
// modified by AttachHandler.
func (d *Dispatcher) classByTyp(typ string) (*taskClassImpl, Handler, error) {
	msgTyp, _ := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typ))
	if msgTyp == nil {
		return nil, nil, errors.Reason("no proto message %q is registered", typ).Tag(httpStatus404).Err()
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByTyp[msgTyp]; cls != nil {
		return cls, cls.Handler, nil
	}
	return nil, nil, errors.Reason("no task class matching type %q is registered", typ).Tag(httpStatus404).Err()
}

////////////////////////////////////////////////////////////////////////////////

type taskBackend int

const (
	backendCloudTasks taskBackend = 1
	backendPubSub     taskBackend = 2
)

// taskClassImpl knows how to prepare and handle tasks of a particular class.
type taskClassImpl struct {
	TaskClass
	disp      *Dispatcher
	protoType protoreflect.MessageType
	backend   taskBackend
	running   int32
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

// Definition implements TaskClassRef interface.
func (cls *taskClassImpl) Definition() TaskClass {
	return cls.TaskClass
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

// traceContext returns a tracing context for TraceContextHeader header or "".
//
// We use Cloud Trace propagation format.
func traceContext(ctx context.Context) string {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return ""
	}
	headers := make(propagation.MapCarrier, 1)
	(propagator.CloudTraceFormatPropagator{}).Inject(ctx, headers)
	return headers[propagator.TraceContextHeaderName]
}

// parseHeaders examines headers of the incoming Cloud Tasks push.
func parseHeaders(h http.Header) ExecutionInfo {
	magicHeader := func(key string) string {
		if val := h.Get("X-AppEngine-" + key); val != "" {
			return val
		}
		return h.Get("X-CloudTasks-" + key)
	}

	var execCount int64
	if count := magicHeader("TaskExecutionCount"); count != "" {
		execCount, _ = strconv.ParseInt(count, 10, 32)
	}

	var eta time.Time
	if s := h.Get(ExpectedETAHeader); s != "" {
		// Expected format is "<seconds(int64)>.<microseconds(int32)>".
		parts := strings.Split(s, ".")
		if len(parts) == 2 {
			secs, errS := strconv.ParseInt(parts[0], 10, 64)
			micros, errM := strconv.ParseInt(parts[1], 10, 32)
			if errS == nil && errM == nil {
				eta = time.Unix(secs, micros*1000)
			}
		}
	}

	return ExecutionInfo{
		ExecutionCount:        int(execCount),
		TaskID:                magicHeader("TaskName"),
		Queue:                 magicHeader("QueueName"),
		taskRetryReason:       magicHeader("TaskRetryReason"),
		taskPreviousResponse:  magicHeader("TaskPreviousResponse"),
		submitterTraceContext: h.Get(TraceContextHeader),
		expectedETA:           eta,
	}
}

// httpReply writes and logs HTTP response.
//
// `msg` is sent to the caller as is. `err` is logged, but not sent.
func httpReply(c *router.Context, code int, msg string, err error) {
	if err != nil && !quietOnError.In(err) {
		if Ignore.In(err) {
			logging.Warningf(c.Request.Context(), "server/tq task %s: %s", msg, err)
		} else {
			logging.Errorf(c.Request.Context(), "server/tq task %s: %s", msg, err)
		}
	}
	if code == http.StatusNoContent {
		msg = ""
	}
	http.Error(c.Writer, msg, code)
}

// replyWithErr calls httpReply deriving status code from `err`.
func replyWithErr(c *router.Context, err error) {
	switch {
	case err == nil:
		httpReply(c, http.StatusOK /* 200 */, "OK", nil)
	case Fatal.In(err):
		httpReply(c, http.StatusAccepted /* 202 */, "fatal error", err)
	case Ignore.In(err):
		httpReply(c, http.StatusNoContent /* 204 */, "ignored error", err)
	case transient.Tag.In(err):
		httpReply(c, http.StatusInternalServerError /* 500 */, "transient error", err)
	default:
		status := http.StatusTooManyRequests
		if code, ok := httpStatusTag.Value(err); ok {
			status = code
		}
		httpReply(c, status, "error", err)
	}
}
