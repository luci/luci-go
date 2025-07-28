// Copyright 2017 The LUCI Authors.
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

// Package tq implements simple routing layer for task queue tasks.
//
// Deprecated: use go.chromium.org/luci/server/tq instead.
package tq

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/server/router"
)

// TODO(vadimsh): Instrument with tsmon metrics. In particular have a metric
// for failed retries (when a task fails more than once).

// Retry is an error tag used to indicate that the handler wants the task to
// be redelivered later.
//
// See Handler doc for more details.
var Retry = errtag.Make("the task should be retried", true)

// Dispatcher submits and handles task queue tasks.
type Dispatcher struct {
	BaseURL string // URL prefix for all URLs, "/internal/tasks/" by default

	mu       sync.RWMutex
	handlers map[string]handler // the key is proto message type name
}

// Task contains task body and additional parameters that influence how it is
// routed.
type Task struct {
	// Payload is task's payload as well as indicator of its type.
	//
	// Tasks are routed based on type of the payload message, see RegisterTask.
	Payload proto.Message

	// NamePrefix, if not empty, is a string that will be prefixed to the task's
	// name. Characters in NamePrefix must be appropriate task queue name
	// characters. NamePrefix can be useful because the Task Queue system allows
	// users to search for tasks by prefix.
	//
	// Lexicographically close names can cause hot spots in the Task Queues
	// backend. If NamePrefix is specified, users should try and ensure that
	// it is friendly to sharding (e.g., begins with a hash string).
	//
	// Setting NamePrefix and/or DeduplicationKey will result in a named task
	// being generated. This task can be cancelled using DeleteTask.
	NamePrefix string

	// DeduplicationKey is optional unique key of the task.
	//
	// If a task of a given proto type with a given key has already been enqueued
	// recently, this task will be silently ignored.
	//
	// Such tasks can only be used outside of transactions.
	//
	// Setting NamePrefix and/or DeduplicationKey will result in a named task
	// being generated. This task can be cancelled using DeleteTask.
	DeduplicationKey string

	// Title is optional string that identifies the task in HTTP logs.
	//
	// It will show up as a suffix in task handler URL. It exists exclusively to
	// simplify reading HTTP logs. It serves no other purpose! In particular,
	// it is NOT a task name.
	//
	// Handlers won't ever see it. Pass all information through the task body.
	Title string

	// Delay specifies the duration the task queue service must wait before
	// executing the task.
	//
	// Either Delay or ETA may be set, but not both.
	Delay time.Duration

	// ETA specifies the earliest time a task may be executed.
	//
	// Either Delay or ETA may be set, but not both.
	ETA time.Time

	// Retry options for this task.
	//
	// If given, overrides default options set when this task was registered.
	RetryOptions *taskqueue.RetryOptions
}

// Name generates and returns the task's name.
//
// If the task is not a named task (doesn't have NamePrefix or DeduplicationKey
// set), this will return an empty string.
func (task *Task) Name() string {
	if task.NamePrefix == "" && task.DeduplicationKey == "" {
		return ""
	}

	parts := make([]string, 0, 2)

	if task.NamePrefix != "" {
		parts = append(parts, task.NamePrefix)
	}

	// There's some weird restrictions on what characters are allowed inside task
	// names. Lexicographically close names also cause hot spot problems in the
	// Task Queues backend. To avoid these two issues, we always use SHA256 hashes
	// as task names. Also each task kind owns its own namespace of deduplication
	// keys, so add task type to the digest as well.
	if task.DeduplicationKey != "" {
		h := sha256.New()
		if task.Payload == nil {
			panic("task must have a Payload")
		}
		h.Write([]byte(proto.MessageName(task.Payload)))
		h.Write([]byte{0})
		h.Write([]byte(task.DeduplicationKey))
		parts = append(parts, hex.EncodeToString(h.Sum(nil)))
	}

	return strings.Join(parts, "-")
}

// Handler is called to handle one enqueued task.
//
// The passed context is produced by a middleware chain installed with
// InstallHandlers. In addition it carries task queue request headers,
// accessible through RequestHeaders(ctx) function. They are passed implicitly
// via the context to avoid complicating Handler signature for a feature that
// most callers aren't going to use.
//
// If the returned error is tagged with Retry tag, the request finishes with
// HTTP status 409, indicating to the task queue that it should redeliver the
// task (which it may or may not do, depending on its RetryOptions). Same
// happens if the error is transient (i.e. tagged with transient.Tag), except
// the request finishes with HTTP status 500. This difference allows to
// distinguish "expected" retry requests (errors tagged with Retry) from
// "unexpected" ones (errors tagged with transient.Tag). Retry tag should be
// used **only** if the handler is fully aware of Task Queues semantics and it
// **explicitly** wants the task to be retried because it can't be processed
// right now and the handler expects that the retry will help.
//
// For a contrived example, if the handler can process the task only after 2 PM,
// but it is 01:55 PM now, the handler should return an error tagged with Retry
// to indicate this. On the other hand, if the handler failed to process the
// task due to a RPC timeout or some other exceptional transient situation, it
// should return an error tagged with transient.Tag.
//
// Note that it is OK (and often desirable) to tag an error with both Retry and
// transient.Tag. Such errors propagate through the call stack as transient,
// until they reach Dispatcher, which treats them as retriable.
//
// An untagged error (or success) marks the task as "done", it won't be retried.
type Handler func(ctx context.Context, payload proto.Message) error

// RegisterTask tells the dispatcher that tasks of given proto type should be
// handled by the given handler and routed through the given task queue.
//
// 'prototype' should be a pointer to some concrete proto message. It will be
// used only for its type signature.
//
// Intended to be called during process startup. Panics if such message has
// already been registered.
func (d *Dispatcher) RegisterTask(prototype proto.Message, cb Handler, queue string, opts *taskqueue.RetryOptions) {
	if queue == "" {
		queue = "default" // default GAE task queue name, always exists
	}

	name := proto.MessageName(prototype)
	if name == "" {
		panic(fmt.Sprintf("unregistered proto message type %T", prototype))
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.handlers[name]; ok {
		panic(fmt.Sprintf("handler for %q has already been registered", name))
	}

	if d.handlers == nil {
		d.handlers = make(map[string]handler)
	}

	d.handlers[name] = handler{
		cb:        cb,
		typeName:  name,
		queue:     queue,
		retryOpts: opts,
	}
}

// GetQueues returns the names of task queues known to the dispatcher.
func (d *Dispatcher) GetQueues() []string {
	queues := stringset.New(0)

	d.mu.RLock()
	for _, h := range d.handlers {
		queues.Add(h.queue)
	}
	d.mu.RUnlock()

	return queues.ToSlice()
}

// runBatchesPerQueue is a generic parallel task distributor. It solves the
// problems that:
//   - "tasks" may be assigned to different queues, and tasks assigned to the
//     same queue should be batched together.
//   - Any given batch may exceed queue operation limits, and thus needs to be
//     broken into multiple operations on sub-batches.
//
// fn is called for each sub-batch assigned to each queue. All resulting errors
// are then flattened. If no fn invocation returns any errors, nil will be
// returned. If a single error is returned, this function will return that
// error. If an errors.MultiError is returned, it and any embedded
// MultiError (recursively) will be flattened into a single MultiError
// containing only the non-nil errors. This simplifies user expectations.
func (d *Dispatcher) runBatchesPerQueue(ctx context.Context, tasks []*Task,
	fn func(ctx context.Context, queue string, tasks []*taskqueue.Task) error) error {
	if len(tasks) == 0 {
		return nil
	}

	// Handle the most common case of one task in a more efficient way.
	if len(tasks) == 1 {
		t, queue, err := d.tqTask(tasks[0])
		if err != nil {
			return err
		}
		if err := fn(ctx, queue, []*taskqueue.Task{t}); err != nil {
			return transient.Tag.Apply(err)
		}
		return nil
	}

	perQueue := map[string][]*taskqueue.Task{}
	for _, task := range tasks {
		t, queue, err := d.tqTask(task)
		if err != nil {
			return err
		}
		perQueue[queue] = append(perQueue[queue], t)
	}

	// Enqueue in parallel, per-queue, split into batches based on Task Queue
	// RPC limits (100 tasks per batch).
	const maxBatchSize = 100
	errs := make(chan error)
	ops := 0
	for q, tasks := range perQueue {
		for len(tasks) > 0 {
			count := maxBatchSize
			if count > len(tasks) {
				count = len(tasks)
			}
			go func(q string, batch []*taskqueue.Task) {
				errs <- fn(ctx, q, batch)
			}(q, tasks[:count])
			tasks = tasks[count:]
			ops++
		}
	}

	all := errors.NewLazyMultiError(ops)
	for i := range ops {
		err := <-errs
		if err != nil {
			all.Assign(i, err)
		}
	}

	if err := errors.Flatten(all.Get()); err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}

// AddTask submits given tasks to an appropriate task queue.
//
// It means, at some later time in some other GAE process, callbacks registered
// as handlers for corresponding proto types will be called.
//
// If the given context is transactional or namespaced, inherits the
// transaction/namespace. Note if running outside of a transaction and multiple
// tasks are passed, the operation is not atomic: it returns an error if at
// least one enqueue operation failed (there's no way to figure out which one
// exactly).
//
// Returns only transient errors. Unlike regular Task Queue's Add,
// ErrTaskAlreadyAdded is not considered an error.
func (d *Dispatcher) AddTask(ctx context.Context, tasks ...*Task) error {
	return d.runBatchesPerQueue(ctx, tasks, func(ctx context.Context, queue string, tasks []*taskqueue.Task) error {
		if err := taskqueue.Add(ctx, queue, tasks...); err != nil {
			return errors.Filter(err, taskqueue.ErrTaskAlreadyAdded)
		}
		return nil
	})
}

// DeleteTask deletes the specified tasks from their queues.
//
// If the given context is transactional or namespaced, inherits the
// transaction/namespace. Note if running outside of a transaction and multiple
// tasks are passed, the operation is not atomic: it returns an error if at
// least one enqueue operation failed (there's no way to figure out which one
// exactly).
//
// Returns only transient errors. Unlike regular Task Queue's Delete,
// attempts to delete an unknown or tombstoned task are not considered errors.
func (d *Dispatcher) DeleteTask(ctx context.Context, tasks ...*Task) error {
	return d.runBatchesPerQueue(ctx, tasks, func(ctx context.Context, queue string, tasks []*taskqueue.Task) error {
		return errors.FilterFunc(taskqueue.Delete(ctx, queue, tasks...), func(err error) bool {
			// Currently, the best way to detect an attempt to delete an unknown task
			// is to check the string with tolerable error message phrases.
			for _, phrase := range []string{"UNKNOWN_TASK", "TOMBSTONED_TASK"} {
				if strings.Contains(err.Error(), phrase) {
					return true
				}
			}
			return false
		})
	})
}

// InstallRoutes installs appropriate HTTP routes in the router.
//
// Must be called only after all task handlers are registered!
func (d *Dispatcher) InstallRoutes(r *router.Router, mw router.MiddlewareChain) {
	queues := stringset.New(0)

	d.mu.RLock()
	for _, h := range d.handlers {
		queues.Add(h.queue)
	}
	d.mu.RUnlock()

	for _, q := range queues.ToSlice() {
		r.POST(
			fmt.Sprintf("%s%s/*title", d.baseURL(), q),
			mw.Extend(gaemiddleware.RequireTaskQueue(q)),
			d.processHTTPRequest)
	}
}

////////////////////////////////////////////////////////////////////////////////

var (
	requestHeadersKey = "taskqueue.RequestHeaders"
	errOutsideHandler = errors.New("request headers are only available inside a task handler")
)

func withRequestHeaders(ctx context.Context, hdr *taskqueue.RequestHeaders) context.Context {
	return context.WithValue(ctx, &requestHeadersKey, hdr)
}

// RequestHeaders returns the special task-queue HTTP request headers for
// the current task handler.
//
// Returns an error if called from outside of a task handler.
func RequestHeaders(ctx context.Context) (*taskqueue.RequestHeaders, error) {
	if hdr, ok := ctx.Value(&requestHeadersKey).(*taskqueue.RequestHeaders); ok {
		return hdr, nil
	}
	return nil, errOutsideHandler
}

////////////////////////////////////////////////////////////////////////////////

type handler struct {
	cb        Handler                 // the actual handler
	typeName  string                  // name of the proto type it handles
	queue     string                  // name of the task queue
	retryOpts *taskqueue.RetryOptions // default retry options
}

// tqTask constructs task queue task struct.
func (d *Dispatcher) tqTask(task *Task) (*taskqueue.Task, string, error) {
	h, err := d.handler(task.Payload)
	if err != nil {
		return nil, "", err
	}

	blob, err := serializePayload(task.Payload)
	if err != nil {
		return nil, "", err
	}

	title := h.typeName
	if task.Title != "" {
		title = task.Title
	}

	retryOpts := h.retryOpts
	if task.RetryOptions != nil {
		retryOpts = task.RetryOptions
	}

	return &taskqueue.Task{
		Path:         fmt.Sprintf("%s%s/%s", d.baseURL(), h.queue, title),
		Name:         task.Name(),
		Method:       "POST",
		Payload:      blob,
		ETA:          task.ETA,
		Delay:        task.Delay,
		RetryOptions: retryOpts,
	}, h.queue, nil
}

// baseURL returns a URL prefix for all HTTP routes used by Dispatcher.
//
// It ends with '/'.
func (d *Dispatcher) baseURL() string {
	switch {
	case d.BaseURL != "" && strings.HasSuffix(d.BaseURL, "/"):
		return d.BaseURL
	case d.BaseURL != "":
		return d.BaseURL + "/"
	default:
		return "/internal/tasks/"
	}
}

// handler returns a handler struct registered with Register.
func (d *Dispatcher) handler(payload proto.Message) (handler, error) {
	name := proto.MessageName(payload)

	d.mu.RLock()
	defer d.mu.RUnlock()

	handler, registered := d.handlers[name]
	if !registered {
		return handler, fmt.Errorf("handler for %q is not registered", name)
	}
	return handler, nil
}

// processHTTPRequest is invoked on each HTTP POST.
//
// It deserializes the task and invokes an appropriate callback. Finishes the
// request with status 202 in case of a fatal error (to stop retries).
func (d *Dispatcher) processHTTPRequest(c *router.Context) {
	headers := taskqueue.ParseRequestHeaders(c.Request.Header)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpReply(c, false, 500, "Failed to read request body: %s", err)
		return
	}
	logging.Debugf(c.Request.Context(), "Received task (exec count %d): %s", headers.TaskExecutionCount, body)

	payload, err := deserializePayload(body)
	if err != nil {
		httpReply(c, false, 202, "Bad payload, can't deserialize: %s", err)
		return
	}

	h, err := d.handler(payload)
	if err != nil {
		httpReply(c, false, 202, "Bad task: %s", err)
		return
	}

	ctx := withRequestHeaders(c.Request.Context(), headers)
	switch err = h.cb(ctx, payload); {
	case err == nil:
		httpReply(c, true, 200, "OK")
	case Retry.In(err):
		httpReply(c, false, 409, "The handler asked for retry: %s", err)
	case transient.Tag.In(err):
		httpReply(c, false, 500, "Transient error: %s", err)
	default:
		httpReply(c, false, 202, "Fatal error: %s", err)
	}
}

func httpReply(c *router.Context, ok bool, code int, msg string, args ...any) {
	body := fmt.Sprintf(msg, args...)
	if !ok {
		logging.Errorf(c.Request.Context(), "%s", body)
	}
	http.Error(c.Writer, body, code)
}

////////////////////////////////////////////////////////////////////////////////

var marshaller = jsonpb.Marshaler{}

type envelope struct {
	Type string           `json:"type"`
	Body *json.RawMessage `json:"body"`
}

func serializePayload(task proto.Message) ([]byte, error) {
	var buf bytes.Buffer
	if err := marshaller.Marshal(&buf, task); err != nil {
		return nil, err
	}
	raw := json.RawMessage(buf.Bytes())
	return json.Marshal(envelope{
		Type: proto.MessageName(task),
		Body: &raw,
	})
}

func deserializePayload(blob []byte) (proto.Message, error) {
	env := envelope{}
	if err := json.Unmarshal(blob, &env); err != nil {
		return nil, err
	}

	tp := proto.MessageType(env.Type) // this is **ConcreteStruct{}
	if tp == nil {
		return nil, fmt.Errorf("unregistered proto message name %q", env.Type)
	}
	if env.Body == nil {
		return nil, fmt.Errorf("no task body given")
	}

	task := reflect.New(tp.Elem()).Interface().(proto.Message)
	if err := jsonpb.Unmarshal(bytes.NewReader(*env.Body), task); err != nil {
		return nil, err
	}

	return task, nil
}
