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

package eval

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// progress prints the number of processed items at most once per second.
type progress struct {
	Total int

	done       int
	mu         sync.Mutex
	lastReport time.Time
}

func (p *progress) Done(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.done++

	if clock.Since(ctx, p.lastReport) < time.Second {
		return
	}

	if !p.lastReport.IsZero() {
		logging.Infof(ctx, "Processing patchset: %d/%d\n", p.done, p.Total)
	}
	p.lastReport = clock.Now(ctx)
}
