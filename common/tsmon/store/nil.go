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
	"errors"
	"time"

	"go.chromium.org/luci/common/tsmon/types"
	"golang.org/x/net/context"
)

// NewNilStore creates a metric store that completely ignores all metrics.
//
// It's setters are noop, and getters always return errors.
func NewNilStore() Store {
	return nilStore{}
}

// IsNilStore returns true if given Store is in fact nil-store.
func IsNilStore(s Store) bool {
	_, yep := s.(nilStore)
	return yep
}

type nilStore struct{}

func (nilStore) Register(m types.Metric)   {}
func (nilStore) Unregister(m types.Metric) {}

func (nilStore) DefaultTarget() types.Target     { return nil }
func (nilStore) SetDefaultTarget(t types.Target) {}

func (nilStore) Get(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}) (value interface{}, err error) {
	return nil, errors.New("not implemented")
}

func (nilStore) Set(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, value interface{}) error {
	return nil
}

func (nilStore) Incr(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, delta interface{}) error {
	return nil
}

func (nilStore) GetAll(ctx context.Context) []types.Cell {
	return nil
}

func (nilStore) Reset(ctx context.Context, m types.Metric) {
}
