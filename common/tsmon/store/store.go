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

// Package store contains code for storing and retrieving metrics.
package store

import (
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/types"
)

// A Store is responsible for handling all metric data.
type Store interface {
	DefaultTarget() types.Target
	SetDefaultTarget(t types.Target)

	Get(ctx context.Context, m types.Metric, fieldVals []any) any
	Set(ctx context.Context, m types.Metric, fieldVals []any, value any)
	Del(ctx context.Context, m types.Metric, fieldVals []any)
	Incr(ctx context.Context, m types.Metric, fieldVals []any, delta any)

	GetAll(ctx context.Context) []types.Cell
	Reset(ctx context.Context, m types.Metric)
	Now(ctx context.Context) time.Time
}
