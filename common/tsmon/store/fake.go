// Copyright 2016 The LUCI Authors.
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

package store

import (
	"time"

	"go.chromium.org/luci/common/tsmon/types"
	"golang.org/x/net/context"
)

// Fake is a fake Store.
type Fake struct {
	Cells []types.Cell
	DT    types.Target
}

// DefaultTarget returns DT.
func (s *Fake) DefaultTarget() types.Target { return s.DT }

// SetDefaultTarget does nothing.
func (s *Fake) SetDefaultTarget(types.Target) {}

// Get does nothing.
func (s *Fake) Get(context.Context, types.Metric, time.Time, []interface{}) interface{} {
	return nil
}

// Set does nothing.
func (s *Fake) Set(context.Context, types.Metric, time.Time, []interface{}, interface{}) {
}

// Incr does nothing.
func (s *Fake) Incr(context.Context, types.Metric, time.Time, []interface{}, interface{}) {
}

// GetAll returns the pre-set list of cells.
func (s *Fake) GetAll(context.Context) []types.Cell { return s.Cells }

// Reset does nothing.
func (s *Fake) Reset(context.Context, types.Metric) {}
