// Copyright 2021 The LUCI Authors.
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

package longops

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// Operation defines the interface for long operation implementation.
type Operation interface {
	Do(context.Context) (*eventpb.LongOpCompleted, error)
}

// Base houses shared bits of all long operations.
//
// Base is a single use object.
type Base struct {
	// Run is the Run on which the long operation is to be performed.
	Run *run.Run
	// Op defines what long operation to perform.
	Op *run.OngoingLongOps_Op
	// IsCancelRequested quickly checks whether the current long operation is
	// supposed to be be cancelled.
	IsCancelRequested func() bool

	alreadyCalled int32
}

func (base *Base) assertCalledOnce() {
	if atomic.AddInt32(&base.alreadyCalled, 1) > 1 {
		panic(fmt.Errorf("longops.Operation used more than once"))
	}
}
