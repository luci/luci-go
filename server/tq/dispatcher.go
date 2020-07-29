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
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"
)

// Dispatcher submits and handles Cloud Tasks tasks.
type Dispatcher struct {
	// CloudProject is ID of a Google Cloud project that hosts queues.
	CloudProject string
	// CloudRegion is a ID of a Google Cloud region that hosts queues.
	CloudRegion string

	mu           sync.RWMutex
	routesPrefix string // e.g. "/internal/tasks/", always with trailing '/'
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

	// Handler will be called by the dispatcher to execute the tasks.
	//
	// The handler will receive the task's payload as a proto message of the exact
	// same type as type of Prototype.
	//
	// See Handler doc for more info.
	Handler Handler

	// Queue is a name of Cloud Tasks queue to use for the tasks.
	//
	// It is a short queue name within the region. The full queue name would be
	// "projects/<CloudProject>/locations/<CloudRegion>/queues/<Queue>", where
	// <CloudProject> and <CloudRegion> come from the Dispatcher configuration.
	//
	// This queue must exist already.
	Queue string
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
// a registered task class with the same ID or Payload.
func (d *Dispatcher) RegisterTaskClass(t TaskClass) {
	// ...
}

// InstallRoutes installs HTTP routes under the given prefix.
//
// Panics if routes have already been installed.
//
// The exposed HTTP endpoints are called by Cloud Tasks service when it is time
// to execute a task.
func (d *Dispatcher) InstallRoutes(r *router.Router, prefix string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.routesPrefix != "" {
		panic("InstallRoutes called twice")
	}
	d.routesPrefix = strings.TrimRight(prefix, "/") + "/"

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
// Tags errors related to RPCs with transient.Tag.
func (d *Dispatcher) AddTask(ctx context.Context, tasks ...*Task) error {
	return nil
}
