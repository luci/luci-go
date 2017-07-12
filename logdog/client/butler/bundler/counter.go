// Copyright 2015 The LUCI Authors.
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

package bundler

import (
	"sync/atomic"
)

// counter is a goroutine-safe monotonically-increasing counter.
type counter struct {
	// current is the current counter value.
	//
	// It must be the first field in the struct to ensure it's 64-bit aligned
	// for atomic operations.
	current int64
}

func (c *counter) next() int64 {
	// AddInt64 returns the value + 1, so subtract one to get its previous value
	// (current counter value).
	return atomic.AddInt64(&c.current, 1) - 1
}
