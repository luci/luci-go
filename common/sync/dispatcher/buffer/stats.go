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

package buffer

import "fmt"

// Stats is a block of information about the Buffer's present state.
type Stats struct {
	// UnleasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which are currently owned by the Buffer but are not currently
	// leased. This includes:
	//    * Items buffered, but not yet cut into a Batch.
	//    * Items in unleased Batches.
	//
	// UnleasedItemSize is the size in 'size units' of the same items.
	UnleasedItemCount int
	UnleasedItemSize  int

	// LeasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which are currently owned by the Buffer and are in active
	// leases.
	//
	// LeasedItemSize is the size in 'size units' of the same items.
	LeasedItemCount int
	LeasedItemSize  int

	// DroppedLeasedItemCount is the total number of items (i.e. objects passed to
	// AddNoBlock) which were part of leases, but where those leases have been
	// dropped (due to FullBehavior policy), but have not yet been ACK/NACK'd.
	//
	// Once these batches are ACK/NACK'd they'll be dropped from Stats entirely.
	//
	// DroppedLeasedItemSize is the size in 'size units' of the same items.
	DroppedLeasedItemCount int
	DroppedLeasedItemSize  int
}

// Empty returns true iff the Buffer is totally empty (has zero user-provided
// items).
func (s Stats) Empty() bool {
	return s.Total() == 0
}

// Total returns the total number of items currently referenced by the Buffer.
func (s Stats) Total() int {
	return s.UnleasedItemCount + s.LeasedItemCount + s.DroppedLeasedItemCount
}

// TotalSize returns the total number of items currently referenced by the Buffer.
func (s Stats) TotalSize() int {
	return s.UnleasedItemSize + s.LeasedItemSize + s.DroppedLeasedItemSize
}

type category int

const (
	categoryUnleased category = iota
	categoryLeased
	categoryDropped
)

func (s *Stats) getCategoryVars(cat category) (count, sizeUnits *int) {
	switch cat {
	case categoryUnleased:
		count, sizeUnits = &s.UnleasedItemCount, &s.UnleasedItemSize
	case categoryLeased:
		count, sizeUnits = &s.LeasedItemCount, &s.LeasedItemSize
	case categoryDropped:
		count, sizeUnits = &s.DroppedLeasedItemCount, &s.DroppedLeasedItemSize
	default:
		panic(fmt.Errorf("unknown category %d", cat))
	}
	return
}

func (s *Stats) addOneUnleased(siz int) {
	count, sizeUnits := s.getCategoryVars(categoryUnleased)
	*count++
	*sizeUnits += siz
}

// batchStates is an interface over batch that exposes only the following
// methods, which do not depend on the parameter T for a Batch[T].
//
// - getCountedItems()
// - getCountedSize()
type batchStats interface {
	getCountedItems() int
	getCountedSize() int
}

func (s *Stats) add(b batchStats, to category) {
	count, sizeUnits := s.getCategoryVars(to)
	*count += b.getCountedItems()
	*sizeUnits += b.getCountedSize()
}

func (s *Stats) del(b batchStats, to category) {
	count, sizeUnits := s.getCategoryVars(to)
	*count -= b.getCountedItems()
	*sizeUnits -= b.getCountedSize()
}

func (s *Stats) mv(b batchStats, from, to category) {
	s.del(b, from)
	s.add(b, to)
}
