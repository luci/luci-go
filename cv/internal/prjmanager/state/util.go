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

package state

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// pokeRuns pokes Run Manager for each incomplete Run.
//
// Doesn't have to be called in a transaction.
func (h *Handler) pokeRuns(ctx context.Context, s *State) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, id := range s.PB.IncompleteRuns() {
			work <- func() error {
				return h.RunNotifier.PokeNow(ctx, id)
			}
		}
	})
	return common.MostSevereError(err)
}

// indexOfConfigGroup returns index of the externed Config Group name, which
// must exist.
func (s *State) indexOfConfigGroup(id prjcfg.ConfigGroupID) int32 {
	want := id.Name()
	// This can be optimized by lazily creating and caching a map from
	// ConfigGroupID to its index, but most projects have <10 groups,
	// so iterating a slice is faster & good enough.
	names := s.PB.GetConfigGroupNames()
	for i, name := range names {
		if name == want {
			return int32(i)
		}
	}
	panic(fmt.Errorf("%s doesn't match any in known %s ConfigGroupNames", id, names))
}
