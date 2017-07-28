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
package tq

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/server/router"
)

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

	// DeduplicationKey is optional unique key of the task.
	//
	// If a task of a given proto type with a given key has already been enqueued
	// recently, this task will be silently ignored.
	//
	// Such tasks can only be used outside of transactions.
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

// Handler is called to handle one enqueued task.
//
// The passed context is produced by a middleware chain installed with
// InstallHandlers.
//
// execCount corresponds to X-AppEngine-TaskExecutionCount header value: it is
// 1 on first execution attempt, 2 on a retry, and so on.
//
// May return transient errors. In this case, task queue may attempt to
// redeliver the task (depending on RetryOptions).
//
// A fatal error (or success) mark the task as "done", it won't be retried.
type Handler func(c context.Context, payload proto.Message, execCount int) error

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

// AddTask submits given tasks to an appropriate task queue.
//
// It means, at some later time in some other GAE process, callbacks registered
// as handlers for corresponding proto types will be called.
//
// If the given context is transactional, inherits the transaction. Note if
// running outside of a transaction and multiple tasks are passed, the operation
// is not atomic: it returns an error if at least one enqueue operation failed
// (there's no way to figure out which one exactly).
//
// Returns only transient errors. Unlike regular Task Queue's Add,
// ErrTaskAlreadyAdded is not considered an error.
func (d *Dispatcher) AddTask(c context.Context, tasks ...*Task) error {
	if len(tasks) == 0 {
		return nil
	}

	// Handle the most common case of one task in a more efficient way.
	if len(tasks) == 1 {
		t, queue, err := d.tqTask(tasks[0])
		if err != nil {
			return err
		}
		if err := taskqueue.Add(c, queue, t); err != nil {
			if err == taskqueue.ErrTaskAlreadyAdded {
				return nil
			}
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
	errs := make(chan error)
	ops := 0
	for q, tasks := range perQueue {
		for len(tasks) > 0 {
			count := 100
			if count > len(tasks) {
				count = len(tasks)
			}
			go func(q string, batch []*taskqueue.Task) {
				errs <- taskqueue.Add(c, q, batch...)
			}(q, tasks[:count])
			tasks = tasks[count:]
			ops++
		}
	}

	// Gather all errors throwing away ErrTaskAlreadyAdded.
	var all errors.MultiError
	for i := 0; i < ops; i++ {
		err := <-errs
		if merr, yep := err.(errors.MultiError); yep {
			for _, e := range merr {
				if e != nil && e != taskqueue.ErrTaskAlreadyAdded {
					all = append(all, e)
				}
			}
		} else if err != nil && err != taskqueue.ErrTaskAlreadyAdded {
			all = append(all, err)
		}
	}

	if len(all) == 0 {
		return nil
	}

	return transient.Tag.Apply(all)
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

type handler struct {
	cb        Handler                 // the actual handler
	typeName  string                  // name of the proto type it handles
	queue     string                  // name of the task queue
	retryOpts *taskqueue.RetryOptions // default retry options
}

// tqTask constructs task queue task struct.
func (d *Dispatcher) tqTask(task *Task) (*taskqueue.Task, string, error) {
	handler, err := d.handler(task.Payload)
	if err != nil {
		return nil, "", err
	}

	blob, err := serializePayload(task.Payload)
	if err != nil {
		return nil, "", err
	}

	title := handler.typeName
	if task.Title != "" {
		title = task.Title
	}

	retryOpts := handler.retryOpts
	if task.RetryOptions != nil {
		retryOpts = task.RetryOptions
	}

	// There's some weird restrictions on what characters are allowed inside task
	// names. Lexicographically close names also cause hot spot problems in the
	// Task Queues backend. To avoid these two issues, we always use SHA256 hashes
	// as task names. Also each task kind owns its own namespace of deduplication
	// keys, so add task type to the digest as well.
	name := ""
	if task.DeduplicationKey != "" {
		h := sha256.New()
		h.Write([]byte(handler.typeName))
		h.Write([]byte{0})
		h.Write([]byte(task.DeduplicationKey))
		name = hex.EncodeToString(h.Sum(nil))
	}

	return &taskqueue.Task{
		Path:         fmt.Sprintf("%s%s/%s", d.baseURL(), handler.queue, title),
		Name:         name,
		Method:       "POST",
		Payload:      blob,
		ETA:          task.ETA,
		Delay:        task.Delay,
		RetryOptions: retryOpts,
	}, handler.queue, nil
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
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		httpReply(c, false, 500, "Failed to read request body: %s", err)
		return
	}
	logging.Debugf(c.Context, "Received task: %s", body)

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

	execCount, _ := strconv.Atoi(c.Request.Header.Get("X-AppEngine-TaskExecutionCount"))
	switch err = h.cb(c.Context, payload, execCount); {
	case err == nil:
		httpReply(c, true, 200, "OK")
	case transient.Tag.In(err):
		httpReply(c, false, 500, "Transient error: %s", err)
	default:
		httpReply(c, false, 202, "Fatal error: %s", err)
	}
}

func httpReply(c *router.Context, ok bool, code int, msg string, args ...interface{}) {
	body := fmt.Sprintf(msg, args...)
	if !ok {
		logging.Errorf(c.Context, "%s", body)
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
