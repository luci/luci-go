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
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon/types"
)

// NewNilStore creates a metric store that completely ignores all metrics.
//
// It's setters are noop, and getters return nil.
func NewNilStore() Store {
	return nilStore{}
}

// IsNilStore returns true if given Store is in fact nil-store.
func IsNilStore(s Store) bool {
	_, yep := s.(nilStore)
	return yep
}

type nilStore struct{}

func (nilStore) DefaultTarget() types.Target     { return nil }
func (nilStore) SetDefaultTarget(t types.Target) {}

func (nilStore) Get(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []any) any {
	return nil
}

func (nilStore) Set(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []any, value any) {
}

func (nilStore) Del(ctx context.Context, m types.Metric, fieldVals []any) {
}

func (nilStore) Incr(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []any, delta any) {
}

func (nilStore) GetAll(ctx context.Context) []types.Cell   { return nil }
func (nilStore) Reset(ctx context.Context, m types.Metric) {}
func (nilStore) Now(ctx context.Context) time.Time         { return clock.Now(ctx) }
