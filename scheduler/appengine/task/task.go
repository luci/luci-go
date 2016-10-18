// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package task defines interface between Scheduler engine and implementations
// of particular tasks (such as URL fetch tasks, Swarming tasks, DM tasks, etc).
//
// Its subpackages contain concrete realizations of Manager interface.
package task

import (
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"
)

// Status is status of a single job invocation.
type Status string

const (
	// StatusStarting means the task is about to start.
	StatusStarting Status = "STARTING"
	// StatusRunning means the task has started and is running now.
	StatusRunning Status = "RUNNING"
	// StatusSucceeded means the task finished with success.
	StatusSucceeded Status = "SUCCEEDED"
	// StatusFailed means the task finished with error or failed to start.
	StatusFailed Status = "FAILED"
	// StatusOverrun means the task should have been started, but previous one is
	// still running.
	StatusOverrun Status = "OVERRUN"
	// StatusAborted means the task was forcefully aborted (manually or due to
	// hard deadline).
	StatusAborted Status = "ABORTED"
)

// Final returns true if Status represents some final status.
func (s Status) Final() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusOverrun, StatusAborted:
		return true
	default:
		return false
	}
}

// Manager knows how to work with a particular kind of tasks (e.g URL fetch
// tasks, Swarming tasks, etc): how to deserialize, validate and execute them.
//
// Manager uses Controller to talk back to the scheduler engine.
type Manager interface {
	// Name returns task manager name. It identifies the corresponding kind
	// of tasks and used in various resource names (e.g. PubSub topic names).
	Name() string

	// ProtoMessageType returns a pointer to protobuf message struct that
	// describes config for the task kind, e.g. &UrlFetchTask{}. Will be used
	// only for its type signature.
	ProtoMessageType() proto.Message

	// ValidateProtoMessage verifies task definition proto message makes sense.
	// msg must have same underlying type as ProtoMessageType() return value.
	ValidateProtoMessage(msg proto.Message) error

	// LaunchTask starts (or starts and finishes in one go) the task.
	//
	// Manager's responsibilities:
	//  * To move the task to some state other than StatusStarting
	//    (by changing ctl.State().Status).
	//  * Be idempotent, if possible, using ctl.InvocationNonce() as an operation
	//    key.
	//  * Not to use supplied controller outside of LaunchTask call.
	//  * Not to use supplied controller concurrently without synchronization.
	//
	// If `LaunchTask` crashes before returning or returns a transient error, it
	// will be called again later, receiving exact same ctl.InvocationNonce(), but
	// different ctl.InvocationID().
	//
	// TaskManager may optionally use ctl.Save() to checkpoint a progress, in this
	// case returned error will switch invocation status to StatusFailed only if
	// its currently saved status is still StatusStarting.
	LaunchTask(c context.Context, ctl Controller) error

	// AbortTask is called to opportunistically abort launched task.
	//
	// It is called right before the job is forcefully switched to a failed state.
	// The engine does not wait for the task runner to acknowledge this action.
	//
	// AbortTask must be idempotent since it may be called multiple times in case
	// of errors.
	AbortTask(c context.Context, ctl Controller) error

	// HandleNotification is called whenever engine receives a PubSub message sent
	// to a topic created with Controller.PrepareTopic. Expect duplicated and
	// out-of-order messages here. HandleNotification must be idempotent.
	//
	// Returns transient error to trigger a redeliver of the message, no error to
	// to acknowledge the message and fatal error to move the invocation to failed
	// state.
	//
	// Any modifications made to the invocation state will be saved regardless of
	// the return value (to save the debug log).
	HandleNotification(c context.Context, ctl Controller, msg *pubsub.PubsubMessage) error

	// HandleTimer is called to process timers set up by Controller.AddTimer.
	//
	// Expect duplicated or delayed events here. HandleTimer must be idempotent.
	//
	// Returns transient error to trigger a redeliver of the event, no error to
	// acknowledge the event and fatal error to move the invocation to failed
	// state.
	//
	// Any modifications made to the invocation state will be saved regardless of
	// the return value (to save the debug log).
	HandleTimer(c context.Context, ctl Controller, name string, payload []byte) error
}

// Controller is passed to LaunchTask by the scheduler engine. It gives Manager
// control over one job invocation. Manager must not use it outside of
// LaunchTask. Controller implementation is generally not thread safe (but it's
// fine to use it from multiple goroutines if access is protected by a lock).
type Controller interface {
	// JobID returns full job ID the controller is operating on.
	JobID() string

	// InvocationID returns unique identifier of this particular invocation.
	InvocationID() int64

	// InvocationNonce returns an identifier for the task launch request.
	//
	// If for whatever reason LaunchTask crashed or returned transient error, the
	// engine will create new invocation (with new InvocationID), that has same
	// InvocationNonce. TaskManager implementation thus can use it to add
	// idempotency to LaunchTask calls.
	InvocationNonce() int64

	// Task is proto message with task definition.
	//
	// It is guaranteed to have same underlying type as manager.ProtoMessageType()
	// return value.
	Task() proto.Message

	// State returns a mutable portion of task invocation state.
	//
	// TaskManager can modify it in-place and then call Controller.Save to persist
	// the changes. The state will also be saved by the engine automatically if
	// Manager doesn't call Save.
	State() *State

	// AddTimer sets up a new delayed call to Manager.HandleTimer.
	//
	// Timers are active as long as the invocation is not in one of the final
	// states. There is no way to cancel a timer (ignore HandleTimer call
	// instead).
	//
	// 'name' will be visible in logs, it should convey a purpose for this timer.
	// It doesn't have to be unique.
	//
	// 'payload' is any byte blob carried verbatim to Manager.HandleTimer.
	//
	// All timers are actually enabled in Save(), in the same transaction that
	// updates the job state.
	//
	// TODO(vadimsh): Need a way to deduplicate/disable timers added when retrying
	// on HandleTimer transient errors.
	AddTimer(delay time.Duration, name string, payload []byte)

	// PrepareTopic create PubSub topic for notifications related to the task and
	// adds given publisher to its ACL.
	//
	// It returns full name of the topic and a token that will be used to route
	// PubSub messages back to the Manager. Topic name and its configuration are
	// controlled by the Engine. The publisher to the topic must be instructed to
	// put the token into 'auth_token' attribute of PubSub messages. The engine
	// will know how to route such messages to Manager.HandleNotification.
	//
	// 'publisher' can be a service account email, or an URL to some luci service.
	// If URL is given, its /auth/api/v1/server/info endpoint will be used to
	// grab a corresponding service account name. All service that use luci auth
	// component expose this endpoint.
	PrepareTopic(publisher string) (topic string, token string, err error)

	// GetClient returns http.Client that is configured to use job's service
	// account credentials to talk to other services.
	//
	// All requests made by the client must finish before given deadline time
	// (or they will be forcefully aborted).
	GetClient(timeout time.Duration) (*http.Client, error)

	// DebugLog appends a line to the free form text log of the task.
	// For debugging.
	DebugLog(format string, args ...interface{})

	// Save updates the state of the task in the persistent store.
	//
	// It also schedules all pending timer ticks added via AddTimer.
	//
	// Will be called by the engine after it launches the task. May also be called
	// by the Manager itself, even multiple times (e.g. once to notify that the
	// task has started, a second time to notify it has finished).
	//
	// Returns error if it couldn't save the invocation state. It is fine to
	// ignore it. The engine will attempt to Save the invocation at the end anyway
	// and it will properly handle the error if it happens again.
	Save() error
}

// State is mutable portion of the task invocation state.
//
// It can be mutated by TaskManager directly.
type State struct {
	Status   Status // overall status of the invocation, see the enum
	TaskData []byte // storage for TaskManager-specific task data
	ViewURL  string // URL to human readable task page, shows in UI
}
