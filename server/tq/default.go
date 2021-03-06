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

package tq

import (
	"context"
)

// Default is a dispatcher installed into the server when using NewModule or
// NewModuleFromFlags.
//
// The module takes care of configuring this dispatcher based on the server
// environment and module's options.
//
// You still need to register your tasks in it using RegisterTaskClass.
var Default Dispatcher

// RegisterTaskClass is a shortcut for Default.RegisterTaskClass.
func RegisterTaskClass(t TaskClass) TaskClassRef {
	return Default.RegisterTaskClass(t)
}

// AddTask is a shortcut for Default.AddTask.
func AddTask(ctx context.Context, task *Task) error {
	return Default.AddTask(ctx, task)
}

// MustAddTask is like AddTask, but panics on errors.
//
// Mostly useful for AddTask calls inside a Spanner transaction, where they
// essentially just call span.BufferWrite (i.e. make no RPCs) and thus can fail
// only if arguments are bad (in which case it is OK to panic).
func MustAddTask(ctx context.Context, task *Task) {
	if err := AddTask(ctx, task); err != nil {
		panic(err)
	}
}

// Sweep is a shortcut for Default.Sweep.
func Sweep(ctx context.Context) error {
	return Default.Sweep(ctx)
}
