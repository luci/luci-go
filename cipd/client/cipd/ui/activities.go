// Copyright 2021 The LUCI Authors.
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

package ui

import (
	"context"

	"go.chromium.org/luci/common/logging"
)

var activityCtxKey = "cipd.ui.Activity"

// Units is what kind of units to use for an activity progress.
type Units string

// UnitBytes indicates units passed to Progress are bytes.
const UnitBytes Units = "bytes"

// Activity is a single-threaded process with a progress indicator.
//
// Once installed into a context it also acts as a logger sink in that
// context.
//
// The progress is allowed to "jump back" (e.g. when a download is restarted).
type Activity interface {
	// Progress updates the activity progress.
	Progress(ctx context.Context, title string, units Units, cur, total int64)
	// Log is called by the logging system when the activity is installed into the context.
	Log(ctx context.Context, level logging.Level, calldepth int, f string, args []interface{})
}

// ActivityGroup is a group of related activities stopped at the same time.
type ActivityGroup interface {
	// NewActivity creates a new activity in this group.
	NewActivity(ctx context.Context, kind string) Activity
	// Close marks all activities in this group as finished.
	Close()
}

// NewActivityGroup creates a new activity group using factory in the context.
//
// If there's no factory there, uses a primitive implementation that just writes
// activity progress as log messages.
func NewActivityGroup(ctx context.Context) ActivityGroup {
	// TODO(vadimsh): Actually use a factory from the context, so that callers
	// that have some UI can supply the implementation.
	return &primitiveActivityGroup{}
}

// NewActivity creates a new activity and sets it as current in the context.
//
// This also replaces the logger with the one that logs into the activity.
func NewActivity(ctx context.Context, g ActivityGroup, kind string) context.Context {
	a := g.NewActivity(ctx, kind)
	ctx = context.WithValue(ctx, &activityCtxKey, a)
	return logging.SetFactory(ctx, func(ctx context.Context) logging.Logger {
		return &activityLogger{activity: a, callCtx: ctx}
	})
}

// CurrentActivity returns the current activity in the context.
//
// Constructs a primitive implementation that just logs into the logger if
// there's no activity in the context.
func CurrentActivity(ctx context.Context) Activity {
	if a, _ := ctx.Value(&activityCtxKey).(Activity); a != nil {
		return a
	}
	return &primitiveActivity{logger: logging.GetFactory(ctx)}
}

// activityLogger forwards messages to the given activity.
type activityLogger struct {
	activity Activity
	callCtx  context.Context // a context of a particular logging call
}

func (l *activityLogger) Debugf(fmt string, args ...interface{}) {
	l.LogCall(logging.Debug, 1, fmt, args)
}

func (l *activityLogger) Infof(fmt string, args ...interface{}) {
	l.LogCall(logging.Info, 1, fmt, args)
}

func (l *activityLogger) Warningf(fmt string, args ...interface{}) {
	l.LogCall(logging.Warning, 1, fmt, args)
}

func (l *activityLogger) Errorf(fmt string, args ...interface{}) {
	l.LogCall(logging.Error, 1, fmt, args)
}

func (l *activityLogger) LogCall(level logging.Level, calldepth int, f string, args []interface{}) {
	l.activity.Log(l.callCtx, level, calldepth+1, f, args)
}
