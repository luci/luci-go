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

// Package monitor contains the code for sending metric data to monitoring
// endpoints.
package monitor

import (
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/types"
)

// A Monitor can send a collection of metrics somewhere.
type Monitor interface {
	// ChunkSize returns the maximum number of metrics this Monitor can Send in
	// one go.
	//
	// If there are more metrics, they will be broken up into chunks and Send will
	// be called multiple times. A ChunkSize of 0 disables chunking.
	ChunkSize() int

	// Send uploads a chunk of cells to the backend.
	//
	// Can be called concurrently with other Send calls.
	Send(ctx context.Context, cells []types.Cell, now time.Time) error

	// Close closes any resources held by the monitor.
	//
	// Close need not be called at program exit.
	Close() error
}
