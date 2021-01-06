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

package impl

import (
	"context"

	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
)

// This file contains notifications sent by PM to other CV parts.
//
// Some must be executed transactionally with updating Project entity
// (see also eventbox.SideEffectFn). These must be kept as fast as possible
// and should read/write as few Datastore entities as possible.
//
// The rest are executed before the Project entity transaction and thus may
// take some time, possibly doing their own transactions.

const concurrency = 16

// updateRunsConfigFactory returns a function to ask run manager to update
// config for each of IncompleteRuns.
func (s *state) updateRunsConfigFactory(meta config.Meta) eventbox.SideEffectFn {
	hash := meta.Hash()
	return func(ctx context.Context) error {
		err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
			for _, id := range s.incompleteRuns {
				id := id
				work <- func() error {
					return run.UpdateConfig(ctx, id, hash, meta.EVersion)
				}
			}
		})
		return common.MostSevereError(err)
	}
}

// cancelRuns asks run manager to cancel each of IncompleteRuns.
//
// Expects to be called in a transaction as a eventbox.SideEffectFn.
func (s *state) cancelRuns(ctx context.Context) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, id := range s.incompleteRuns {
			id := id
			work <- func() error {
				// TODO(tandrii): pass "Project disabled" as a reason.
				return run.Cancel(ctx, id)
			}
		}
	})
	return common.MostSevereError(err)
}

// pokeRuns pokes run manager of each of IncompleteRuns.
//
// Doesn't have to be called in a transaction.
func (s *state) pokeRuns(ctx context.Context) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, id := range s.incompleteRuns {
			id := id
			work <- func() error {
				return run.Poke(ctx, id)
			}
		}
	})
	return common.MostSevereError(err)
}
