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

//go:generate stringer -type=Toggle

package datastore

// GeoPoint represents a location as latitude/longitude in degrees.
//
// You probably shouldn't use these, but their inclusion here is so that the
// datastore service can interact (and round-trip) correctly with other
// datastore API implementations.
type GeoPoint struct {
	Lat, Lng float64
}

// Valid returns whether a GeoPoint is within [-90, 90] latitude and [-180,
// 180] longitude.
func (g GeoPoint) Valid() bool {
	return -90 <= g.Lat && g.Lat <= 90 && -180 <= g.Lng && g.Lng <= 180
}

// TransactionOptions are the options for running a transaction.
type TransactionOptions struct {
	// XG is whether the transaction can cross multiple entity groups. In
	// comparison, a single group transaction is one where all datastore keys
	// used have the same root key. Note that cross group transactions do not
	// have the same behavior as single group transactions. In particular, it
	// is much more likely to see partially applied transactions in different
	// entity groups, in global queries.
	// It is valid to set XG to true even if the transaction is within a
	// single entity group.
	XG bool
	// Attempts controls the number of retries to perform when commits fail
	// due to a conflicting transaction. If omitted, it defaults to 3.
	Attempts int
}

// Toggle is a tri-state boolean (Auto/True/False), which allows structs
// to control boolean flags for metadata in a non-ambiguous way.
type Toggle byte

// These are the allowed values for Toggle. Any other values are invalid.
const (
	Auto Toggle = iota
	On
	Off
)

// BoolList is a convenience wrapper for []bool that provides summary methods
// for working with the list in aggregate.
type BoolList []bool

// All returns true iff all of the booleans in this list are true.
func (bl BoolList) All() bool {
	for _, b := range bl {
		if !b {
			return false
		}
	}
	return true
}

// Any returns true iff any of the booleans in this list are true.
func (bl BoolList) Any() bool {
	for _, b := range bl {
		if b {
			return true
		}
	}
	return false
}

// ExistsResult is a 2-dimensional boolean array that represents the existence
// of entries in the datastore. It is returned by the datastore Exists method.
// It is designed to accommodate the potentially-nested variadic arguments that
// can be passed to Exists.
//
// The first dimension contains one entry for each Exists input index. If the
// argument is a single entry, the boolean value at this index will be true if
// that argument was present in the datastore and false otherwise. If the
// argument is a slice, it will contain an aggregate value that is true iff no
// values in that slice were missing from the datastore.
//
// The second dimension presents a boolean slice for each input argument. Single
// arguments will have a slice of size 1 whose value corresponds to the first
// dimension value for that argument. Slice arguments have a slice of the same
// size. A given index in the second dimension slice is true iff the element at
// that index was present.
type ExistsResult struct {
	// values is the first dimension aggregate values.
	values BoolList
	// slices is the set of second dimension positional values.
	slices []BoolList
}

func (r *ExistsResult) init(sizes ...int) {
	// In order to reduce allocations, we allocate a single continuous boolean
	// slice and then partition it up into first- and second-dimension slices.
	//
	// To determine the size of the continuous slize, count the number of elements
	// that we'll need:
	// - Single arguments and slice arguments with size 1 will have their
	//   second-dimension slice just point to their element in the first-dimension
	//   slice.
	// - Slice elements of size >1 will have their second-dimension slice added to
	//   the end of the continuous slice. Their slice will be carved off in the
	//   subsequent loop.
	//
	// Consequently, we need one element for each argument, plus len(slice)
	// additional elements for each slice argument of size >1.
	count := len(sizes) // [0..n)
	for _, s := range sizes {
		if s > 1 {
			count += s
		}
	}

	// Allocate our continuous array and partition it into first- and
	// second-dimension slices.
	entries := make(BoolList, count)
	r.values, entries = entries[:len(sizes)], entries[len(sizes):]
	r.slices = make([]BoolList, len(sizes))
	for i, s := range sizes {
		switch {
		case s <= 0:
			break

		case s == 1:
			// Single-entry slice out of "entries".
			r.slices[i] = r.values[i : i+1]

		default:
			r.slices[i], entries = entries[:s], entries[s:]
		}
	}
}

func (r *ExistsResult) set(i, j int) { r.slices[i][j] = true }

// updateSlices updates the top-level value for multi-dimensional elements based
// on their current values.
func (r *ExistsResult) updateSlices() {
	for i, s := range r.slices {
		// Zero-length slices will have a first-dimension true value, since they
		// have no entries and first-dimension is true when there are no false
		// entries.
		r.values[i] = (len(s) == 0 || s.All())
	}
}

// All returns true if all of the available boolean slots are true.
func (r *ExistsResult) All() bool { return r.values.All() }

// Any returns true if any of the boolean slots are true.
func (r *ExistsResult) Any() bool {
	// We have to implement our own Any so zero-length slices don't count towards
	// our result.
	for i, b := range r.values {
		if b && len(r.slices[i]) > 0 {
			return true
		}
	}
	return false
}

// Get returns the boolean value at the specified index.
//
// The one-argument form returns the first-dimension boolean. If i is a slice
// argument, this will be true iff all of the slice's booleans are true.
//
// An optional second argument can be passed to access a specific boolean value
// in slice i. If the argument at i is a single argument, the only valid index,
// 0, will be the same as calling the single-argument Get.
//
// Passing more than one additional argument will result in a panic.
func (r *ExistsResult) Get(i int, j ...int) bool {
	switch len(j) {
	case 0:
		return r.values[i]
	case 1:
		return r.slices[i][j[0]]
	default:
		panic("this method takes one or two arguments")
	}
}

// List returns the BoolList for the given argument index.
//
// The zero-argument form returns the first-dimension boolean list.
//
// An optional argument can be passed to access a specific argument's boolean
// slice. If the argument at i is a non-slice argument, the list will be a slice
// of size 1 containing i's first-dimension value.
//
// Passing more than one argument will result in a panic.
func (r *ExistsResult) List(i ...int) BoolList {
	switch len(i) {
	case 0:
		return r.values
	case 1:
		return r.slices[i[0]]
	default:
		panic("this method takes zero or one arguments")
	}
}

// Len returns the number of boolean results available.
//
// The zero-argument form returns the first-dimension size, which will equal the
// total number of arguments passed to Exists.
//
// The one-argument form returns the number of booleans in the slice for
// argument i.
//
// Passing more than one argument will result in a panic.
func (r *ExistsResult) Len(i ...int) int {
	switch len(i) {
	case 0:
		return len(r.values)
	case 1:
		return len(r.slices[i[0]])
	default:
		panic("this method takes zero or one arguments")
	}
}
