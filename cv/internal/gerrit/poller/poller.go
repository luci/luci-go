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

// Package implements stateful Gerrit polling.
package poller

import (
	"time"

	"go.chromium.org/luci/common/errors"
	"golang.org/x/net/context"
)

// Poke schedules the next poll via task queue.
//
// Under perfect operation, this is redundant, but not harmful.
// Given bugs or imperfect operation, this ensures poller continues operating.
func Poke(ctx context.Context, luciProject string) error {
	return errors.New("TODO(tandrii): implement")
}

// ScheduleUpdateConfig instructs poller to update project config.
func ScheduleUpdateConfig(ctx context.Context, luciProject string) error {
	return errors.New("TODO(tandrii): implement")
}

// poll executes the next poll with the latest known to poller config.
//
// For each discovered CL, enqueues a task for CL updater to refresh CL state.
// Automatically enqueues a new task to perform next poll.
func poll(ctx context.Context, luciProject string) error {
	return errors.New("TODO(tandrii): implement")
}

// updateConfig updates poller to start using the latest project config.
//
// If newer config is discovered, immediately performs a full poll.
func updateConfig(ctx context.Context, luciProject string) error {
	return errors.New("TODO(tandrii): implement")
}

// state persists poller's state in datastore.
type state struct {
	_kind string `gae:"$kind,GerritPoller"`

	// Project is the name of the LUCI Project for which poller is works.
	LuciProject string `gae:"$id"`
	// Enabled indicates whether polling is enabled for this LUCI Project.
	//
	// It's set to false if project is disabled.
	Enabled bool `gae:",noindex"`
	// UpdateTime is the timestamp when this state was last updated.
	UpdateTime time.Time `gae:",noindex"`
	// EVersion is the latest version number of the state.
	//
	// It increments by 1 every time state is updated either due to new project config
	// being updated OR after each successful poll.
	EVersion int64 `gae:",noindex"`
	// ConfigHash defines which Config version was last worked on.
	ConfigHash string `gae:",noindex"`
	// SubPollers track individual states of sub pollers.
	//
	// Most LUCI projects will have just 1 per Gerrit host,
	// but CV may split the set of watched Gerrit projects (aka Git repos) on the
	// same Gerrit host among several SubPollers.
	SubPollers *SubPollers
}
