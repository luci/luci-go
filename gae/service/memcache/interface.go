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

package memcache

import (
	"go.chromium.org/luci/common/errors"
	"golang.org/x/net/context"
)

func filterItems(lme errors.LazyMultiError, items []Item, nilErr error) ([]Item, []int) {
	idxMap := make([]int, 0, len(items))
	retItems := make([]Item, 0, len(items))
	for i, itm := range items {
		if itm != nil {
			idxMap = append(idxMap, i)
			retItems = append(retItems, itm)
		} else {
			lme.Assign(i, nilErr)
		}
	}
	return retItems, idxMap
}

func multiCall(items []Item, nilErr error, inner func(items []Item, cb RawCB) error) error {
	lme := errors.NewLazyMultiError(len(items))
	realItems, idxMap := filterItems(lme, items, nilErr)
	j := 0
	err := inner(realItems, func(err error) {
		lme.Assign(idxMap[j], err)
		j++
	})
	if err == nil {
		err = lme.Get()
		if len(items) == 1 {
			err = errors.SingleError(err)
		}
	}
	return err
}

// NewItem creates a new, mutable, memcache item.
func NewItem(c context.Context, key string) Item {
	return Raw(c).NewItem(key)
}

// Add writes items to memcache iff they don't already exist.
//
// If only one item is provided its error will be returned directly. If more
// than one item is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the item at that index.
func Add(c context.Context, items ...Item) error {
	return multiCall(items, ErrNotStored, Raw(c).AddMulti)
}

// Set writes items into memcache unconditionally.
//
// If only one item is provided its error will be returned directly. If more
// than one item is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the item at that index.
func Set(c context.Context, items ...Item) error {
	return multiCall(items, ErrNotStored, Raw(c).SetMulti)
}

func getMultiImpl(raw RawInterface, items []Item) error {
	lme := errors.NewLazyMultiError(len(items))
	realItems, idxMap := filterItems(lme, items, ErrCacheMiss)
	if len(realItems) == 0 {
		return lme.Get()
	}

	keys := make([]string, len(realItems))
	for i, itm := range realItems {
		keys[i] = itm.Key()
	}

	j := 0
	err := raw.GetMulti(keys, func(item Item, err error) {
		i := idxMap[j]
		if !lme.Assign(i, err) {
			items[i].SetAll(item)
		}
		j++
	})
	if err == nil {
		err = lme.Get()
		if len(items) == 1 {
			err = errors.SingleError(err)
		}
	}
	return err
}

// Get retrieves items from memcache.
func Get(c context.Context, items ...Item) error {
	return getMultiImpl(Raw(c), items)
}

// GetKey is a convenience method for generating and retrieving an Item instance
// for the specified from memcache key.
//
// On a cache miss ErrCacheMiss will be returned. Item will always be
// returned, even on a miss, but it's value may be empty if it was a miss.
func GetKey(c context.Context, key string) (Item, error) {
	raw := Raw(c)
	ret := raw.NewItem(key)
	err := getMultiImpl(raw, []Item{ret})
	return ret, err
}

// Delete deletes items from memcache.
//
// If only one item is provided its error will be returned directly. If more
// than one item is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the item at that index.
func Delete(c context.Context, keys ...string) error {
	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	err := Raw(c).DeleteMulti(keys, func(err error) {
		lme.Assign(i, err)
		i++
	})
	if err == nil {
		err = lme.Get()
		if len(keys) == 1 {
			err = errors.SingleError(err)
		}
	}
	return err
}

// CompareAndSwap writes the given item that was previously returned by Get, if
// the value was neither modified or evicted between the Get and the
// CompareAndSwap calls.
//
// Example:
//   itm := memcache.NewItem(context, "aKey")
//   memcache.Get(context, itm) // check error
//   itm.SetValue(append(itm.Value(), []byte("more bytes")))
//   memcache.CompareAndSwap(context, itm) // check error
//
// If only one item is provided its error will be returned directly. If more
// than one item is provided, an errors.MultiError will be returned in the
// event of an error, with a given error index corresponding to the error
// encountered when processing the item at that index.
func CompareAndSwap(c context.Context, items ...Item) error {
	return multiCall(items, ErrNotStored, Raw(c).CompareAndSwapMulti)
}

// Increment adds delta to the uint64 contained at key. If the memcache key
// is missing, it's populated with initialValue before applying delta (i.e.
// the final value would be initialValue+delta).
//
// Underflow caps at 0, overflow wraps back to 0.
//
// The value is stored as little-endian uint64, see
// "encoding/binary".LittleEndian. If the value is not exactly 8 bytes,
// it's assumed to contain non-number data and this method will return an
// error.
func Increment(c context.Context, key string, delta int64, initialValue uint64) (uint64, error) {
	return Raw(c).Increment(key, delta, &initialValue)
}

// IncrementExisting is like Increment, except that the value must exist
// already.
func IncrementExisting(c context.Context, key string, delta int64) (uint64, error) {
	return Raw(c).Increment(key, delta, nil)
}

// Flush dumps the entire memcache state.
func Flush(c context.Context) error {
	return Raw(c).Flush()
}

// Stats gets some best-effort statistics about the current state of memcache.
func Stats(c context.Context) (*Statistics, error) {
	return Raw(c).Stats()
}
