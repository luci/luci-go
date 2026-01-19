// Copyright 2026 The LUCI Authors.
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

package spanutil

import (
	"fmt"
)

// BufferingOptions specifies buffering options for a paged iterator.
// This does not affect the number of results returned but may affect
// the performance of the iterator.
type BufferingOptions struct {
	// FirstPageSize is the initial number of results to fetch in the
	// first page.
	FirstPageSize int
	// SecondPageSize is the number of results to fetch in the
	// second page.
	SecondPageSize int
	// GrowthFactor is the factor by which to grow the page size from the
	// the third page onwards.
	// A growth factor of 1.0 keeps the page size constant.
	// A growth factor of 2.0 doubles the page size from the third page onwards.
	//
	// Empirical evidence indicates larger page sizes typically improve
	// query time _per row_, until the point where the page size is so
	// large that it causes spilling to disk.
	// To avoid disk spills, growth is capped to a maximum page size of
	// 10,000, which seems to be small enough to avoid disk spills.
	GrowthFactor float64
}

// The page size that usually avoids Spanner spilling data to disk. Tested
// empirically on TestResults table. It is believed spills happens when the
// result set size approaches 32 MB.
// At 1 KB per test result, this keeps query results to ~10MB.
const maxPageSize = 10_000

// PageSizeController controls the page size for a paged iterator.
type PageSizeController struct {
	opts         BufferingOptions
	pageNumber   int
	lastPageSize int
}

// NewPageSizeController initialises a new page size controller.
func NewPageSizeController(opts BufferingOptions) *PageSizeController {
	return &PageSizeController{
		opts: opts,
	}
}

// NextPageSize returns the next page size based on the previous page size and
// the growth factor.
func (bc *PageSizeController) NextPageSize() (int, error) {
	if bc.opts.FirstPageSize <= 0 {
		return 0, fmt.Errorf("initial buffer size must be positive")
	}
	if bc.opts.SecondPageSize <= 0 {
		return 0, fmt.Errorf("second buffer size must be positive")
	}
	if bc.opts.GrowthFactor < 1.0 {
		return 0, fmt.Errorf("growth factor must be equal or greater than 1.0")
	}

	bc.pageNumber++
	if bc.pageNumber == 1 {
		bc.lastPageSize = bc.opts.FirstPageSize
		return bc.lastPageSize, nil
	}
	if bc.pageNumber == 2 {
		bc.lastPageSize = bc.opts.SecondPageSize
		return bc.lastPageSize, nil
	}
	// Grow the page size, clamping to MaxPageSize.
	newPageSize := (float64(bc.lastPageSize) * bc.opts.GrowthFactor)
	if newPageSize > maxPageSize {
		newPageSize = maxPageSize
	}
	bc.lastPageSize = int(newPageSize)
	return bc.lastPageSize, nil
}
