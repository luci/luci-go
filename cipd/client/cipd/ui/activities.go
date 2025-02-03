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
	"fmt"
	"strings"
	"sync"

	"go.chromium.org/luci/common/logging"
)

var (
	activityCtxKey = "cipd.ui.Activity"
	implCtxKey     = "cipd.ui.Implementation"
)

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
	Log(ctx context.Context, lc *logging.LogContext, level logging.Level, calldepth int, f string, args []any)
	// Done is called when the activity finishes.
	Done(ctx context.Context)
}

// ActivityGroup is a group of related activities running at the same time.
//
// Used to assign unique titles to them.
type ActivityGroup struct {
	m   sync.RWMutex
	ids map[string]int
}

// allocateID returns the next sequential ID for this given kind.
func (g *ActivityGroup) allocateID(kind string) int {
	g.m.Lock()
	defer g.m.Unlock()

	if g.ids == nil {
		g.ids = map[string]int{}
	}
	id := g.ids[kind] + 1
	g.ids[kind] = id

	return id
}

// activityTitle returns a title for the activity with the given ID.
func (g *ActivityGroup) activityTitle(kind string, id int) string {
	g.m.RLock()
	total := g.ids[kind]
	g.m.RUnlock()
	if total <= 1 {
		return kind
	}
	totalStr := fmt.Sprintf("%d", total)
	idStr := fmt.Sprintf("%d", id)
	return fmt.Sprintf("%s %s/%s", kind, padLeft(idStr, len(totalStr)), totalStr)
}

// Implementation implements a UI that shows activities.
//
// It lives in the context.
type Implementation interface {
	// NewActivity creates a new activity, optionally putting it in a group.
	NewActivity(ctx context.Context, group *ActivityGroup, kind string) Activity
}

// SetImplementation puts the Implementation into the context.
func SetImplementation(ctx context.Context, impl Implementation) context.Context {
	return context.WithValue(ctx, &implCtxKey, impl)
}

// NewActivity creates a new activity and sets it as current in the context.
//
// Does nothing if the context is already associated with an activity.
//
// This also replaces the logger with the one that logs into the activity.
// If `group` is not-nil, adds this activity into the group. Otherwise it stands
// on its own (whatever it means depends on the UI implementation).
//
// If there's no Implementation in the context, sets up a primitive activity
// implementation that just logs into the logger.
//
// The activity must be closed through the returned CancelFunc. Note that it
// will also close the associated context.
func NewActivity(ctx context.Context, group *ActivityGroup, kind string) (context.Context, context.CancelFunc) {
	if ctx.Value(&activityCtxKey) != nil {
		return context.WithCancel(ctx)
	}

	var activity Activity
	if impl, _ := ctx.Value(&implCtxKey).(Implementation); impl != nil {
		activity = impl.NewActivity(ctx, group, kind)
	} else {
		primitive := &primitiveActivity{
			logger: logging.GetFactory(ctx),
			kind:   kind,
			group:  group,
		}
		if group != nil && kind != "" {
			primitive.id = group.allocateID(kind)
		}
		activity = primitive
	}

	ctx = context.WithValue(ctx, &activityCtxKey, activity)
	ctx = logging.SetFactory(ctx, func(ctx context.Context, lc *logging.LogContext) logging.Logger {
		return &activityLogger{activity: activity, callCtx: ctx, lc: lc}
	})

	ctx, done := context.WithCancel(ctx)
	return ctx, func() {
		activity.Done(ctx)
		done()
	}
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
	callCtx  context.Context     // a context of a particular logging call
	lc       *logging.LogContext // a logging context at the logging call
}

func (l *activityLogger) Debugf(fmt string, args ...any) {
	l.LogCall(logging.Debug, 1, fmt, args)
}

func (l *activityLogger) Infof(fmt string, args ...any) {
	l.LogCall(logging.Info, 1, fmt, args)
}

func (l *activityLogger) Warningf(fmt string, args ...any) {
	l.LogCall(logging.Warning, 1, fmt, args)
}

func (l *activityLogger) Errorf(fmt string, args ...any) {
	l.LogCall(logging.Error, 1, fmt, args)
}

func (l *activityLogger) LogCall(level logging.Level, calldepth int, f string, args []any) {
	l.activity.Log(l.callCtx, l.lc, level, calldepth+1, f, args)
}

// padLeft pads an ASCII string with spaces on the left to make it l bytes long.
func padLeft(s string, l int) string {
	if len(s) < l {
		return strings.Repeat(" ", l-len(s)) + s
	}
	return s
}

// padRight pads an ASCII string with spaces on the right to make it l bytes long.
func padRight(s string, l int) string {
	if len(s) < l {
		return s + strings.Repeat(" ", l-len(s))
	}
	return s
}
