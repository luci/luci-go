// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package task defines interface between cron engine and implementations of
// cron tasks (such as URL fetch tasks, Swarming tasks, DM tasks, etc).
//
// Its subpackages contain concrete realizations of Manager interface.
package task

import (
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Status is status of a single cron job invocation.
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
)

// Final returns true if Status represents some final status.
func (s Status) Final() bool {
	switch s {
	case StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

// Manager knows how to work with a particular kind of tasks (e.g URL fetch
// tasks, Swarming tasks, etc): how to deserialize, validate and execute them.
//
// Manager uses Controller to talk back to cron engine.
type Manager interface {
	// ProtoMessageType returns a pointer to protobuf message struct that
	// describes config for the task kind, e.g. &cron.UrlFetchTask{}. Will be used
	// only for its type signature.
	ProtoMessageType() proto.Message

	// ValidateProtoMessage verifies task definition proto message makes sense.
	// msg must have same underlying type as ProtoMessageType() return value.
	ValidateProtoMessage(msg proto.Message) error

	// LaunchTask starts (or starts and finishes in one go) the given task,
	// described by its proto message. msg must have same underlying type as
	// ProtoMessageType() return value. Manager responsibilities:
	//  * To move the task to some state other than StatusStarting
	//    (by calling ctl.Save(...)).
	//  * Not to use supplied controller outside of LaunchTask call.
	//  * Not to use supplied controller concurrently without synchronization.
	LaunchTask(c context.Context, msg proto.Message, ctl Controller) error
}

// Controller is passed to LaunchTask by cron engine. It gives Manager control
// over one task. Manager must not used it outside of LaunchTask. Controller
// implementation is generally not thread safe (but it's fine to use it from
// multiple goroutines if access is protected by a lock).
type Controller interface {
	// DebugLog appends a line to the free form text log of the task.
	// For debugging.
	DebugLog(format string, args ...interface{})

	// Save updates state of the task in the persistent store. Save must be called
	// at least once, otherwise no changes will be stored. May be called multiple
	// times (e.g. once to notify that task has started, second time to notify it
	// has finished).
	Save(status Status) error
}
