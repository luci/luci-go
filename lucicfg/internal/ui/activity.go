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
)

// Activity represents some process that runs concurrently in a group with other
// similar processes.
//
// State of such concurrent activities will be displayed in a coordinated way
// via an ActivityGroup.
type Activity struct {
	// TODO
}

// ActivityInfo is a description of the activity.
type ActivityInfo struct {
	Package string // the package being worked on
	Version string // the version being worked on (or "" if not important)
}

// NewActivity creates a new activity and registers it in the current activity
// group.
//
// Does nothing (and returns a noop *Activity) if the context doesn't have
// an ActivityGroup set.
func NewActivity(ctx context.Context, info ActivityInfo) *Activity {
	// TODO
	return &Activity{}
}

// ActivityStart sets the given activity as the current in the context and
// marks it as running.
func ActivityStart(ctx context.Context, a *Activity, fmt string, args ...any) context.Context {
	// TODO
	return ctx
}

// ActivityProgress updates the state of the activity in the context.
//
// Does nothing if the context doesn't have an activity.
func ActivityProgress(ctx context.Context, fmt string, args ...any) {
	// TODO
}

// ActivityError closes the activity as failed.
//
// If `fmt` is empty, will retain the last state reported via ActivityProgress,
// otherwise will override it.
//
// Does nothing if the context doesn't have an activity.
func ActivityError(ctx context.Context, fmt string, args ...any) {
	// TODO
}

// ActivityDone closes the activity as finished successfully.
//
// If `fmt` is empty, will retain the last state reported via ActivityProgress,
// otherwise will override it.
//
// Does nothing if the context doesn't have an activity.
func ActivityDone(ctx context.Context, fmt string, args ...any) {
	// TODO
}
