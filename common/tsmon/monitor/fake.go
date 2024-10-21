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

package monitor

import (
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/types"
)

// Fake is a fake Monitor.
type Fake struct {
	CS      int
	Cells   [][]types.Cell
	SendErr error
}

// ChunkSize returns the fake value.
func (m *Fake) ChunkSize() int {
	return m.CS
}

// Send appends the cells to Cells, unless SendErr is set.
func (m *Fake) Send(ctx context.Context, cells []types.Cell, now time.Time) error {
	if m.SendErr != nil {
		return m.SendErr
	}
	m.Cells = append(m.Cells, cells)
	return nil
}

// Close always works.
func (m *Fake) Close() error {
	return nil
}
