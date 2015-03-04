// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bf

import (
	"fmt"
)

// BitField is a AppEngine-serializable bit field implementation. It should
// be nice and fast for non-AppEngine use as well.
//
// You should construct a new one with bf.Make, rather than by direct
// construction. The field names are public due to an appengine artifact.
type BitField struct {
	// fields are public so that datastore can see them... but don't touch them if
	// you know what's good for you.

	// Data is the actual bits data. rightmost bit of 0th number is the first bit.
	Data []int64 `datastore:",noindex"`

	// NumBits is the number of bits held in this BitField. It's stored as a
	// casted uint64 since datastore can't store unsigned things, apparently.
	NumBits int64 `datastore:",noindex"`
}

// Make creates a new properly-initialized BitField.
func Make(size uint64) BitField {
	return BitField{
		Data:    make([]int64, (size+64-1)>>6),
		NumBits: int64(size),
	}
}

// Size returns the number of bits which this BitField can hold.
func (bf BitField) Size() uint64 {
	return uint64(bf.NumBits)
}

// Set turns the given bit to true, regardless of its previous value. Error
// returned is for bounds checking.
func (bf BitField) Set(idx uint64) error {
	if idx >= bf.Size() {
		return fmt.Errorf("Cannot set bit %d in BitField of size %d", idx, bf.Size())
	}
	bf.Data[idx>>6] |= 1 << (idx % 64)
	return nil
}

// Clear turns the given bit to false, regardless of its previous value. Error
// returned is for bounds checking.
func (bf BitField) Clear(idx uint64) error {
	if idx >= bf.Size() {
		return fmt.Errorf("Cannot clear bit %d in BitField of size %d", idx, bf.Size())
	}
	bf.Data[idx>>6] &^= 1 << (idx % 64)
	return nil
}

// All returns true iff all of the bits are equal to `val`.
func (bf BitField) All(val bool) bool {
	targ := bf.Size()
	if !val {
		targ = 0
	}
	return bf.CountSet() == targ
}

func countBitsSet(v uint64) uint8 {
	// https://graphics.stanford.edu/~seander/bithacks.html#CountBitsSetParallel
	const (
		maxUint64   = ^(uint64(0))
		eightyfives = maxUint64 / 3        // 0x555555...
		fiftyones   = maxUint64 / 15 * 3   // 0x333333...
		fifteens    = maxUint64 / 255 * 15 // 0x0f0f0f...
		ones        = maxUint64 / 255      // 0x010101...
		offset      = (64/8 - 1) * 8       // 56
	)

	v -= v >> 1 & eightyfives
	v = v&fiftyones + v>>2&fiftyones
	v = (v + v>>4) & fifteens
	return uint8(v * ones >> offset)
}

// CountSet returns the number of true bits.
func (bf BitField) CountSet() uint64 {
	ret := uint64(0)
	for _, word := range bf.Data {
		if word != 0 {
			ret += uint64(countBitsSet(uint64(word)))
		}
	}
	return ret
}

// IsSet returns the value of a given bit.
func (bf BitField) IsSet(idx uint64) bool {
	return idx < bf.Size() && ((bf.Data[idx>>6] & (1 << (idx % 64))) != 0)
}
