// Copyright 2022 The LUCI Authors.
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

package protowalk

import "google.golang.org/protobuf/reflect/protoreflect"

// recurseAttrs is a cache data value which indicates that Fields() should
// recurse into the field (because some deeper message has indicated that it
// needs to process something).
//
// This also encodes the TYPE of recursion which should occur, to avoid
// unnecessary protoreflect calls.
//
// This byte holds multiple values; The least significant 2 bits are used
// to indicate {None, One, Repeated, Map}. In the case of Map, an additional
// 3 bits are used as a value to indicate what proto kind the map key is
// (bool, int, uint, string). The map key kinds are explicitly enumerated here,
// because we want to do sorted map iteration for deterministic error messages.
//
// Note; If someone wants to process the keys of a map, they should annotate the
// map field itself.
//
// All values other than recurseNone imply that the field is a Message type.
type recurseAttr byte

const (
	recurseNone      recurseAttr = 0b000_00                  // No recursion
	recurseOne       recurseAttr = 0b000_01                  // Singular message field
	recurseRepeated  recurseAttr = 0b000_10                  // Repeated message field
	recurseMapKind   recurseAttr = 0b000_11                  // map<*, Message>
	recurseMapBool   recurseAttr = 0b001_00 | recurseMapKind // map<bool, Message>
	recurseMapInt    recurseAttr = 0b010_00 | recurseMapKind // map<int{32,64}, Message>
	recurseMapUint   recurseAttr = 0b011_00 | recurseMapKind // map<uint{32,64}, Message>
	recurseMapString recurseAttr = 0b100_00 | recurseMapKind // map<string, Message>
)

// isMap returns true if recurseAttr is any kind of map recursion.
func (a recurseAttr) isMap() bool { return a&recurseMapKind == recurseMapKind }

// Converts this recurseAttr for a Map into a protoreflect Kind for the map's
// key.
func (a recurseAttr) mapKeyKind() protoreflect.Kind {
	switch a {
	case recurseMapBool:
		return protoreflect.BoolKind
	case recurseMapInt:
		return protoreflect.Int32Kind // also counts as int64
	case recurseMapUint:
		return protoreflect.Uint32Kind // also counts as uint64
	case recurseMapString:
		return protoreflect.StringKind
	}
	return 0 // invalid
}

// set will replace this recurseAttr with `other`, if `other` has a non-zero
// value.
//
// Returns `true` if `other` was non-zero (i.e. replacement or equivalence).
//
// Panics if `a` is already set and `other` is a different non-zero value.
// This should NEVER happen unless this package has a bug because a given
// field cannot have multiple recursion types (i.e. it's either single valued,
// repeated, or a map... not some combination of the above).
func (a *recurseAttr) set(other recurseAttr) (wasSet bool) {
	wasSet = other != recurseNone
	if wasSet {
		switch myval := *a; {
		case myval == recurseNone:
			*a = other
		case myval != other:
			panic("BUG: recurseAttr.set called with incompatible recursion types.")
		}
	}
	return
}

// ProcessAttr is a cache data value which indicates that this field needs to be
// processed, whether for set fields, unset fields, or both.
type ProcessAttr byte

const (
	// ProcessNever indicates that this processor doesn't want to process this
	// field.
	ProcessNever ProcessAttr = 0b00

	// ProcessIfSet indicates that this processor only applies to this field when
	// the field has a value (i.e. protoreflect.Message.Has(field)).
	ProcessIfSet ProcessAttr = 0b01

	// ProcessIfUnset indicates that this processor only applies to this field when
	// the field does not have a value (i.e. !protoreflect.Message.Has(field)).
	ProcessIfUnset ProcessAttr = 0b10

	// ProcessAlways indicates that this processor always applies to this field
	// regardless of value status.
	ProcessAlways = ProcessIfSet | ProcessIfUnset
)

// applies returns true if this ProcessAttr should process a field which `isSet`.
func (a ProcessAttr) applies(isSet bool) bool {
	return ((isSet && (a&ProcessIfSet == ProcessIfSet)) ||
		(!isSet && (a&ProcessIfUnset == ProcessIfUnset)))
}

// Valid returns true if `a` is a known ProcessAttr value.
func (a ProcessAttr) Valid() bool {
	switch a {
	case ProcessNever, ProcessIfSet, ProcessIfUnset, ProcessAlways:
		return true
	}
	return false
}
