// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
)

// TODO - implement more direct encode/decode routines

// outputPropertyState retains the state for a single output property in State.
type outputPropertyState struct {
	mu sync.Mutex

	// data is the non-nil *struct or map for this property state.
	// This is the authoritative value for this property.
	data any

	toJSON func(source any) []byte
	cached *structpb.Struct
}

func (pstate *outputPropertyState) toStruct() *structpb.Struct {
	pstate.mu.Lock()
	defer pstate.mu.Unlock()

	if pstate.cached != nil {
		return pstate.cached
	}

	var ret *structpb.Struct
	if spb, ok := pstate.data.(*structpb.Struct); ok {
		ret = spb
	} else {
		ret = &structpb.Struct{}
		if err := protojson.Unmarshal(pstate.toJSON(pstate.data), ret); err != nil {
			// NOTE: this can never happen - we already know that pstate.data can be
			// serialized to JSON, and thus can be unmarshaled to a Struct.
			panic(errors.Fmt("impossible - JSON cannot Unmarshal to Struct: %w", err))
		}
	}

	pstate.cached = ret

	return ret
}

func (pstate *outputPropertyState) interact(s *State, cb func(dat any) bool) {
	mutated := func() (mutated bool) {
		pstate.mu.Lock()
		defer pstate.mu.Unlock()
		if mutated = cb(pstate.data); mutated {
			pstate.cached = nil
		}
		return
	}()
	if mutated {
		s.incrementVersion()
	}
}

func (pstate *outputPropertyState) set(s *State, dat any) {
	func() {
		pstate.mu.Lock()
		defer pstate.mu.Unlock()

		pstate.cached = nil
		pstate.data = dat
	}()
	s.incrementVersion()
}
