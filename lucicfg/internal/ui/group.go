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

package ui

import (
	"context"
	"os"
)

var (
	configCtxKey        = "lucicfg.internal.ui.Config"
	activityGroupCtxKey = "lucicfg.internal.ui.ActivityGroup"
)

// Config lives in the context and defines how activities are displayed.
type Config struct {
	Fancy bool     // if true, use fancy terminal output instead of plain logging
	Term  *os.File // a terminal for fancy output (unused if Fancy == false)
}

// WithConfig configures how activities will be displayed.
func WithConfig(ctx context.Context, cfg Config) context.Context {
	return context.WithValue(ctx, &configCtxKey, cfg)
}

// ActivityGroup is a group of related activities running at the same time.
//
// All activities in an ActivityGroup are displayed together in a nicely
// formatted coordinated way.
type ActivityGroup struct {
	cfg Config
}

// NewActivityGroup returns a new context with a new activity group.
//
// Call the returned context.CancelFunc to finalize the activity group when all
// activities are finished and no new activities are expected. This will also
// cancel the context (to make sure no new activities can run in the finalized
// group).
func NewActivityGroup(ctx context.Context, title string) (context.Context, context.CancelFunc) {
	cfg, _ := ctx.Value(&configCtxKey).(Config)

	group := &ActivityGroup{
		cfg: cfg,
	}

	ctx = context.WithValue(ctx, &activityGroupCtxKey, group)
	ctx, cancel := context.WithCancel(ctx)

	return ctx, func() {
		group.stop()
		cancel()
	}
}

// stop finishes running the goroutine that updates the activity group state.
func (ag *ActivityGroup) stop() {
	// TODO
}
