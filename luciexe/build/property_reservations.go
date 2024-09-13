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

package build

import (
	"fmt"
	"runtime"
	"sync"

	"go.chromium.org/luci/common/errors"
)

type resLocations struct {
	mu sync.Mutex
	// locations maps reserved namespace to the source location where they were
	// first reserved.
	locations map[string]string
}

func (r *resLocations) snap() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ret := make(map[string]string, len(r.locations))
	for k, v := range r.locations {
		ret[k] = v
	}
	return ret
}

// skip is the number of frames to skip from your callsite.
//
// skip=1 means "your caller"
func (r *resLocations) reserve(ns string, kind string, skip int) {
	if ns == "" {
		panic(errors.New("empty namespace not allowed"))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if current, ok := r.locations[ns]; ok {
		panic(
			errors.Reason("cannot reserve %s namespace %q: already reserved by %s",
				kind, ns, current).Err())
	}

	reservationLocation := "<unknown>"
	if _, file, line, ok := runtime.Caller(skip + 1); ok {
		reservationLocation = fmt.Sprintf("\"%s:%d\"", file, line)
	}

	if r.locations == nil {
		r.locations = map[string]string{}
	}
	r.locations[ns] = reservationLocation
}

func (r *resLocations) each(cb func(ns string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ns := range r.locations {
		cb(ns)
	}
}

func (r *resLocations) clear(cb func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locations = nil
	if cb != nil {
		cb()
	}
}

func (r *resLocations) get(ns string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locations[ns]
}
