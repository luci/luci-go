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

package cfgclient

import (
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgclient/backend"
)

// Resolver resolves configuration data into a native type.
type Resolver interface {
	// Resolve resolves a single Item.
	Resolve(it *backend.Item) error
}

// MultiResolver resolves a slice of Item.
//
// If it resolves into a slice (which it should), it must preserve Item ordering
// such that resolved Item "n" appears in the slice at index "n".
//
// Any individual resolution failures should be
type MultiResolver interface {
	// PrepareMulti indicates that items are about to be loaded, as well as the
	// number of resolved values. The MultiResolver should allocate and export its
	// output value.
	//
	// The value's contents will be populated in a series of successive
	// ResolveItemAt calls for indexes between zero and (size-1).
	PrepareMulti(size int)

	// ResolveItemAt resolves an individual item at the specified index.
	// PrepareMulti with a size greater than i must be called prior to using
	// ResolveItemAt.
	ResolveItemAt(i int, it *backend.Item) error
}

// FormattingResolver is a Resolver that changes the format of its contents.
// If a Resolver does this, it self-describes the new format so that it can be
// associated with the format later.
type FormattingResolver interface {
	// Format returns the FormatterRegistry key and associated data for this
	// Resolver.
	//
	// An empty format represents no Formatter, meaning that this Resolver only
	// supports the raw config service result.
	Format() backend.FormatSpec
}

func assertEmptyFormat(it *backend.Item) error {
	if !it.FormatSpec.Unformatted() {
		return errors.Reason("unknown format: %q", it.FormatSpec.Formatter).Err()
	}
	return nil
}

// Bytes is a Resolver that resolves config data into a byte slice.
func Bytes(out *[]byte) Resolver { return byteSliceResolver{out} }

// BytesSlice is a MultiResolver that resolves condig data into a slice of byte
// slices.
func BytesSlice(out *[][]byte) MultiResolver { return multiByteSliceResolver{out} }

type byteSliceResolver struct {
	out *[]byte
}

func (r byteSliceResolver) Resolve(it *backend.Item) error {
	if err := assertEmptyFormat(it); err != nil {
		return err
	}
	*r.out = []byte(it.Content)
	return nil
}

type multiByteSliceResolver struct {
	out *[][]byte
}

func (r multiByteSliceResolver) PrepareMulti(size int) {
	switch size {
	case 0:
		*r.out = nil

	default:
		*r.out = make([][]byte, size)
	}
}

func (r multiByteSliceResolver) ResolveItemAt(i int, it *backend.Item) error {
	if err := assertEmptyFormat(it); err != nil {
		return err
	}
	(*r.out)[i] = []byte(it.Content)
	return nil
}

// String is a Resolver that resolves config data into a string.
func String(out *string) Resolver { return stringResolver{out} }

// StringSlice is a MultiResolver that resolves config data into a slice of
// strings.
func StringSlice(out *[]string) MultiResolver { return multiStringResolver{out} }

type stringResolver struct {
	out *string
}

func (r stringResolver) Resolve(it *backend.Item) error {
	if err := assertEmptyFormat(it); err != nil {
		return err
	}
	*r.out = it.Content
	return nil
}

type multiStringResolver struct {
	out *[]string
}

func (r multiStringResolver) PrepareMulti(size int) {
	switch size {
	case 0:
		*r.out = nil

	default:
		*r.out = make([]string, size)
	}
}

func (r multiStringResolver) ResolveItemAt(i int, it *backend.Item) error {
	if err := assertEmptyFormat(it); err != nil {
		return err
	}
	(*r.out)[i] = it.Content
	return nil
}
