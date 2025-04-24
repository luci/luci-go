// Copyright 2025 The LUCI Authors.
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

// Package ui implements a simple terminal UI for showing concurrent operations.
//
// Used to display the process of loading of remote packages.
package ui

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/logging"
)

var activityCtxKey = "lucicfg.internal.ui.Activity"

type activityState string

const (
	activityPending activityState = "pending" // hasn't started yet
	activityRunning activityState = "running" // started and running now
	activityDone    activityState = "done"    // finished successfully
	activityErr     activityState = "err"     // finished with an error
)

func (as activityState) isFinished() bool {
	return as == activityDone || as == activityErr
}

// Activity represents some process that runs concurrently in a group with other
// similar processes.
//
// State of such concurrent activities will be displayed in a coordinated way
// via an ActivityGroup.
type Activity struct {
	info  ActivityInfo   // the static description
	group *activityGroup // the parent activity group or nil for noop activities

	// These fields are protected by the parent's ActivityGroup.mu.
	state   activityState // the current state of the activity
	message string        // the last reported status message
	spinner int           // incremented to spin the spinner
}

// ActivityInfo is a description of the activity.
type ActivityInfo struct {
	Package string // the package being worked on (required)
	Version string // the version being worked on (or "" if not important)
}

// NewActivity creates a new activity and registers it in the current activity
// group.
//
// Does nothing (and returns a noop *Activity) if the context doesn't have
// an ActivityGroup set.
func NewActivity(ctx context.Context, info ActivityInfo) *Activity {
	group, _ := ctx.Value(&activityGroupCtxKey).(*activityGroup)
	activity := &Activity{info: info, group: group, state: activityPending}
	if group != nil {
		group.addActivity(activity)
	}
	return activity
}

// ActivityStart sets the given activity as the current in the context and
// marks it as running.
func ActivityStart(ctx context.Context, a *Activity, msg string, args ...any) context.Context {
	f := logging.Fields{"pkg": a.info.Package}
	if a.info.Version != "" {
		f["ver"] = a.info.Version
	}
	ctx = logging.SetFields(ctx, f)

	// If not using a fancy UI, the adjusted logger in the context is enough. All
	// functions like ActivityProgress will log through it as well instead of
	// trying to update the fancy UI.
	if a.group == nil {
		if msg == "" {
			logging.Infof(ctx, "starting")
		} else {
			logging.Infof(ctx, msg, args...)
		}
		return ctx
	}

	a.group.updateActivity(func() {
		if a.state == activityPending {
			a.state = activityRunning
			a.message = fmt.Sprintf(msg, args...)
		}
	})
	return context.WithValue(ctx, &activityCtxKey, a)
}

// updateActivity updates the current activity in the context though its group.
func updateActivity(ctx context.Context, cb func(a *Activity)) bool {
	if a, _ := ctx.Value(&activityCtxKey).(*Activity); a != nil {
		a.group.updateActivity(func() { cb(a) })
		return true
	}
	return false
}

// ActivityProgress updates the state of the running activity in the context.
//
// Just logs the message if the context doesn't have an activity.
func ActivityProgress(ctx context.Context, msg string, args ...any) {
	ok := updateActivity(ctx, func(a *Activity) {
		if a.state == activityRunning {
			a.message = fmt.Sprintf(msg, args...)
		}
	})
	if !ok && msg != "" {
		logging.Infof(ctx, msg, args...)
	}
}

// ActivityError closes the activity as failed.
//
// If `msg` is empty, will retain the last state reported via ActivityProgress,
// otherwise will override it.
//
// Just logs the message if the context doesn't have an activity.
func ActivityError(ctx context.Context, msg string, args ...any) {
	ok := updateActivity(ctx, func(a *Activity) {
		if a.state == activityRunning {
			a.state = activityErr
			switch {
			case msg != "":
				a.message = fmt.Sprintf(msg, args...)
			case a.message == "":
				a.message = "unspecified error"
			}
		}
	})
	if !ok {
		if msg == "" {
			logging.Errorf(ctx, "failed")
		} else {
			logging.Errorf(ctx, msg, args...)
		}
	}
}

// ActivityDone closes the activity as finished successfully.
//
// If `msg` is empty, will retain the last state reported via ActivityProgress,
// otherwise will override it.
//
// Just logs the message if the context doesn't have an activity.
func ActivityDone(ctx context.Context, msg string, args ...any) {
	ok := updateActivity(ctx, func(a *Activity) {
		if a.state == activityRunning {
			a.state = activityDone
			switch {
			case msg != "":
				a.message = fmt.Sprintf(msg, args...)
			case a.message == "":
				a.message = "done"
			}
		}
	})
	if !ok {
		if msg == "" {
			logging.Infof(ctx, "done")
		} else {
			logging.Infof(ctx, msg, args...)
		}
	}
}
