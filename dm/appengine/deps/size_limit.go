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

package deps

import (
	"sync/atomic"
)

type sizeLimit struct {
	// current must only be accessed via sync/atomic
	current uint32

	// is read-only and may be accessed directly.
	max uint32
}

func (s *sizeLimit) PossiblyOK(amount uint32) bool {
	if s.max == 0 {
		return true
	}
	if amount > s.max {
		return false
	}
	// 0 <= 10 - 6
	return atomic.LoadUint32(&s.current) <= (s.max - amount)
}

func (s *sizeLimit) Add(amount uint32) bool {
	if s.max == 0 {
		return true
	}
	if amount > s.max {
		return false
	}
	lim := s.max - amount
	for {
		cur := atomic.LoadUint32(&s.current)
		if cur > lim {
			return false
		}
		if atomic.CompareAndSwapUint32(&s.current, cur, cur+amount) {
			return true
		}
	}
}
