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
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/timestamppb"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
)

// Dispatcher submits and handles Cloud Tasks tasks.
type Dispatcher struct {
	// Submitter is used to submit Cloud Tasks tasks.
	//
	// Use CloudTaskSubmitter to create a submitter that sends requests to real
	// Cloud Tasks.
	//
	// If not set, task submissions will fail.
	Submitter Submitter

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

	// DefaultRoutingPrefix is an URL prefix for produced Cloud Tasks.
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
	// It can either be a short name like "default" or a full name like
	// "projects/<project>/locations/<region>/queues/<name>". If it is a full name
	// it must have the above format or RegisterTaskClass would panic.
	//
	// If it is a short queue name, the full queue name will be constructed using
	// dispatcher's CloudProject and CloudRegion if they are set.
	//
	// This queue must exist already.
	Queue string

	// RoutingPrefix is an URL prefix for produced Cloud Tasks.
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

// RegisterTaskClass tells the dispatcher how to route and handle tasks of some
// particular type.
//
// Intended to be called during process startup. Panics if there's already
// a registered task class with the same ID or Prototype.
func (d *Dispatcher) RegisterTaskClass(cls TaskClass) {
	if !taskClassIDRe.MatchString(cls.ID) {
		panic(fmt.Sprintf("bad TaskClass ID %q", cls.ID))
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

	impl := &taskClassImpl{TaskClass: cls, protoType: typ}
	d.clsByID[cls.ID] = impl
	d.clsByTyp[typ] = impl
}

// InstallRoutes installs HTTP routes under the given prefix.
//
// The exposed HTTP endpoints are called by Cloud Tasks service when it is time
// to execute a task.
func (d *Dispatcher) InstallRoutes(r *router.Router, prefix string) {
	if prefix == "" {
		prefix = "/internal/tasks/"
	} else if !strings.HasPrefix(prefix, "/") {
		panic("the prefix should start with /")
	}

	// We don't really care about the exact format of URLs. At the same time
	// accepting all requests under InternalRoutingPrefix is necessary for
	// compatibility with "appengine/tq" which used totally different URL format.
	prefix = strings.TrimRight(prefix, "/") + "/*path"
	r.POST(prefix, d.authorize(), func(c *router.Context) {
		switch err := d.handlePush(c.Context, c.Request); {
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

// AddTask submits a task for later execution.
//
// The task payload type should match some registered TaskClass. Its ID will
// be used to identify the task class in the serialized Cloud Tasks task body.
//
// At some later time, in some other process, the dispatcher will invoke
// a handler attached to the corresponding TaskClass, based on its ID extracted
// from the task body.
//
// If the given context is transactional, inherits the transaction. It means
// the task will eventually be executed if and only if the transaction
// successfully commits.
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

	ctx, span := trace.StartSpan(ctx, "go.chromium.org/luci/server/tq.AddTask")
	span.Attribute("cr.dev/class", cls.ID)
	span.Attribute("cr.dev/title", task.Title)
	defer func() { span.End(err) }()

	ctx = logging.SetFields(ctx, logging.Fields{
		"cr.dev/class": cls.ID,
		"cr.dev/title": task.Title,
	})

	// Each individual RPC should be pretty quick. Also Cloud Tasks client bugs
	// out if the context has a large deadline.
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()

	err = d.Submitter.CreateTask(ctx, req)

	code := status.Code(err)
	span.Attribute("cr.dev/code", code)

	switch code {
	case codes.OK, codes.AlreadyExists:
		return nil
	case codes.Internal, codes.Unknown, codes.Unavailable:
		return transient.Tag.Apply(err)
	default:
		return err
	}
}

////////////////////////////////////////////////////////////////////////////////

// defaultHeaders are added to all submitted Cloud Tasks.
var defaultHeaders = map[string]string{"Content-Type": "application/json"}

// taskClassIDRe is used to validate TaskClass.ID.
var taskClassIDRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]{1,100}$`)

// prepTask converts a task into Cloud Tasks request.
func (d *Dispatcher) prepTask(ctx context.Context, t *Task) (*taskClassImpl, *taskspb.CreateTaskRequest, error) {
	cls, err := d.classByMsg(t.Payload)
	if err != nil {
		return nil, nil, err
	}

	body, err := cls.serialize(t)
	if err != nil {
		return nil, nil, err
	}

	queueID, err := d.queueID(cls.Queue)
	if err != nil {
		return nil, nil, err
	}

	taskID := ""
	if t.DeduplicationKey != "" {
		taskID = queueID + "/tasks/" + cls.taskName(t)
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
	host, relativeURI, err := d.taskTarget(cls, t)
	if err != nil {
		return nil, nil, err
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
				HttpMethod:  taskspb.HttpMethod_POST,
				RelativeUri: relativeURI,
				Headers:     defaultHeaders,
				Body:        body,
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
			HttpMethod: taskspb.HttpMethod_POST,
			Url:        "https://" + host + relativeURI,
			Headers:    defaultHeaders,
			Body:       body,
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

// authorize returns a middleware chain that authorizes requests to handlers.
func (d *Dispatcher) authorize() router.MiddlewareChain {
	if d.NoAuth {
		return router.MiddlewareChain{}
	}

	// We first want to validate the OpenID Connect token in the request (if any)
	// and then check that thus authenticated caller is authorized to push tasks
	// to us. On GAE we also trust X-AppEngine-* headers.
	oidc := auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
		AudienceCheck: openid.AudienceMatchesHost,
	})
	return router.NewMiddlewareChain(oidc, func(c *router.Context, next router.Handler) {
		// On GAE X-AppEngine-* headers can be trusted. Check we are being called
		// by Cloud Tasks. We don't care by which queue exactly though. It is
		// easier to move tasks between queues that way.
		if d.GAE && c.Request.Header.Get("X-AppEngine-QueueName") != "" {
			next(c)
			return
		}

		// If this is an HttpRequest task, it must have an OpenID token. This can
		// happen on GAE too if the task was submitted as HttpRequest instead of
		// AppEngineHttpRequest.
		if ident := auth.CurrentIdentity(c.Context); ident.Kind() != identity.Anonymous {
			if d.isAuthorizedPusher(ident) {
				next(c)
			} else {
				httpReply(c, 403,
					fmt.Sprintf("Caller %q is not authorized", ident),
					errors.Reason("expecting %q or any of %q", d.PushAs, d.AuthorizedPushers).Err(),
				)
			}
			return
		}

		// An anonymous request on GAE likely indicates an attempt to use magic
		// headers.
		if d.GAE {
			httpReply(c, 403,
				"This endpoint can only be called by Cloud Tasks",
				errors.Reason("no OIDC token and no X-AppEngine-QueueName header").Err(),
			)
		} else {
			httpReply(c, 403,
				"This endpoint can only be called by Cloud Tasks",
				errors.Reason("no OIDC token").Err(),
			)
		}
	})
}

// isAuthorizedPusher is true if `ident` is allowed to push tasks to us.
func (d *Dispatcher) isAuthorizedPusher(ident identity.Identity) bool {
	if ident.Kind() != identity.User {
		return false // we want service accounts
	}
	email := ident.Email()
	if email == d.PushAs {
		return true
	}
	for _, extra := range d.AuthorizedPushers {
		if email == extra {
			return true
		}
	}
	return false
}

// handlePush handles one incoming task.
//
// Returns errors annotated in the same style as errors from Handler, see its
// doc.
func (d *Dispatcher) handlePush(ctx context.Context, r *http.Request) error {
	// TODO(vadimsh): Parse magic headers to get the attempt count.

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Annotate(err, "failed to read the request").Tag(transient.Tag).Err()
	}

	// See taskClassImpl.serialize().
	env := envelope{}
	if err := json.Unmarshal(body, &env); err != nil {
		return errors.Annotate(err, "not a valid JSON body").Err()
	}

	// Find the matching registered task class. Newer tasks always have `class`
	// set. Older ones have `type` instead.
	var cls *taskClassImpl
	if env.Class != "" {
		cls, err = d.classByID(env.Class)
	} else if env.Type != "" {
		cls, err = d.classByTyp(env.Type)
	} else {
		err = errors.Reason("malformed task body, no class").Err()
	}
	if err != nil {
		return err
	}

	msg, err := cls.deserialize(&env)
	if err != nil {
		return errors.Annotate(err, "malformed body of task class %q", cls.ID).Err()
	}

	return cls.Handler(ctx, msg)
}

// classByID returns a task class given its ID or an error if no such class.
func (d *Dispatcher) classByID(id string) (*taskClassImpl, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByID[id]; cls != nil {
		return cls, nil
	}
	return nil, errors.Reason("no task class with ID %q is registered", id).Err()
}

// classByMsg returns a task class given proto message  or an error if no
// such class.
func (d *Dispatcher) classByMsg(msg proto.Message) (*taskClassImpl, error) {
	typ := msg.ProtoReflect().Type()
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByTyp[typ]; cls != nil {
		return cls, nil
	}
	return nil, errors.Reason("no task class matching type %q is registered", typ.Descriptor().FullName()).Err()
}

// classByTyp returns a task class given proto message name or an error if no
// such class.
func (d *Dispatcher) classByTyp(typ string) (*taskClassImpl, error) {
	msgTyp, _ := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(typ))
	if msgTyp == nil {
		return nil, errors.Reason("no proto message %q is registered", typ).Err()
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if cls := d.clsByTyp[msgTyp]; cls != nil {
		return cls, nil
	}
	return nil, errors.Reason("no task class matching type %q is registered", typ).Err()
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

////////////////////////////////////////////////////////////////////////////////

// taskClassImpl knows how to prepare and handle tasks of a particular class.
type taskClassImpl struct {
	TaskClass
	protoType protoreflect.MessageType
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
