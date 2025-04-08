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

package scan

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

// DeadBotDetector is a BotVisitor that recognizes bots that haven't been seen
// for a while and moves them into "DEAD" state, terminating whatever tasks
// they were running with "BOT_DIED" status.
type DeadBotDetector struct {
	// BotDeathTimeout is how long a bot must be away before being declared dead.
	BotDeathTimeout time.Duration
	// TasksManager is used to change state of tasks.
	TasksManager tasks.Manager

	// The goroutine group executing the transactions.
	eg *errgroup.Group
	// Set to true it at least one transaction failed (all failures are logged).
	failed atomic.Bool

	// submitUpdate is usually botinfo.Update.Submit, but can be mocked in tests.
	submitUpdate func(ctx context.Context, update *botinfo.Update) (*botinfo.SubmittedUpdate, error)
}

var _ BotVisitor = (*DeadBotDetector)(nil)

// ID returns an unique identifier of this visitor used for storing its state.
//
// Part of BotVisitor interface.
func (*DeadBotDetector) ID() string {
	return "DeadBotDetector"
}

// Frequency returns how frequently this visitor should run.
//
// Part of BotVisitor interface.
func (*DeadBotDetector) Frequency() time.Duration {
	return 0
}

// Prepare prepares the visitor to use `shards` parallel queries.
//
// Part of BotVisitor interface.
func (r *DeadBotDetector) Prepare(ctx context.Context, shards int, lastRun time.Time) {
	r.eg, _ = errgroup.WithContext(ctx)
	r.eg.SetLimit(1024 / shards) // 1024 goroutines total across all shards
	if r.submitUpdate == nil {
		r.submitUpdate = func(ctx context.Context, update *botinfo.Update) (*botinfo.SubmittedUpdate, error) {
			return update.Submit(ctx)
		}
	}
}

// Visit is called for every bot.
//
// Part of BotVisitor interface.
func (r *DeadBotDetector) Visit(ctx context.Context, shard int, bot *model.BotInfo) {
	if r.shouldMarkAsDead(ctx, bot) {
		r.eg.Go(func() error {
			if err := r.markAsDead(ctx, bot.BotID()); err != nil {
				r.failed.Store(true)
			}
			return nil
		})
	}
}

// Finalize is called once the scan is done.
//
// Part of BotVisitor interface.
func (r *DeadBotDetector) Finalize(ctx context.Context, scanErr error) error {
	_ = r.eg.Wait()
	if r.failed.Load() {
		return errors.Reason("failed to mark some bot(s) as dead").Err()
	}
	return nil
}

// shouldMarkAsDead is true if the bot should be marked as dead, but it isn't.
func (r *DeadBotDetector) shouldMarkAsDead(ctx context.Context, bot *model.BotInfo) bool {
	return !bot.IsDead() && bot.LastSeen.IsSet() && clock.Since(ctx, bot.LastSeen.Get()) > r.BotDeathTimeout
}

// markAsDead runs a transaction to mark the bot as dead, logging the outcome.
func (r *DeadBotDetector) markAsDead(ctx context.Context, botID string) error {
	update := &botinfo.Update{
		BotID:        botID,
		EventType:    model.BotEventMissing,
		TasksManager: r.TasksManager,
		Prepare: func(ctx context.Context, bot *model.BotInfo) (*botinfo.PrepareOutcome, error) {
			return &botinfo.PrepareOutcome{
				Proceed: bot != nil && r.shouldMarkAsDead(ctx, bot),
			}, nil
		},
	}
	submitted, err := r.submitUpdate(ctx, update)
	switch {
	case err != nil:
		logging.Errorf(ctx, "When marking bot %s as dead: %s", botID, err)
	case submitted != nil:
		logging.Infof(ctx, "Dead bot: %s", botID)
	}
	return err
}
