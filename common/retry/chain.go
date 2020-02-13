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

package retry

import (
	"context"
	"time"
)

type chainIterator struct {
	iters []Iterator
	factories []Factory
}

func (ci *chainIterator) Next(ctx context.Context, err error) time.Duration {
	for i, f := range ci.factories {
		if ci.iters[i] == nil {
			ci.iters[i] = f()
		}
		if delay := ci.iters[i].Next(ctx, err); delay != Stop {
			return delay
		}
		ci.iters[i] = nil
	}
	return Stop
}

func ChainFactories(factories ...Factory) Factory {
	if len(factories) == 0 {
		return nil
	}

	return func() Iterator {
		return &chainIterator{
			iters: make([]Iterator, len(factories)),
			factories:factories}
	}
}
