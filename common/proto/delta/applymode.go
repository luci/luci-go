// Copyright 2025 The LUCI Authors.
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

//go:generate stringer -type ApplyMode

package delta

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ApplyMode indicates the way in which [Diff.Apply] will apply a specific
// field mutation.
//
// See [MakeTemplate] and [MakeRawTemplate].
type ApplyMode byte

const (
	// MergeDefault is:
	//   * MergeAppend for list fields
	//   * MergeUpdate for map fields
	//   * MergeSet for all other field types
	ModeDefault ApplyMode = iota

	// ModeSet will replace the field in the target message.
	//
	// Applies to all field types.
	ModeSet

	// ModeUpdate will act like python's dict.update for map fields.
	//
	// That is; any key set in the diff will be set verbatim into the target map.
	//
	// Applies to map field types.
	ModeUpdate

	// ModeAppend will replace the field with the concatenation of the target
	// message's field content and the new content.
	//
	// Applies to repeated field types.
	ModeAppend

	// ModeMaxEnum will pick the greater of the old and new field values.
	//
	// Applies to enum field types.
	ModeMaxEnum

	// ModeMerge uses `proto.Merge` for this field.
	//
	// Applies to Message field types.
	ModeMerge
)

func (m ApplyMode) assertValidFor(fd protoreflect.FieldDescriptor) {
	switch m {
	case ModeSet:
		return // always valid

	case ModeMerge:
		if fd.Kind() != protoreflect.MessageKind {
			panic(fmt.Errorf("mode %q does not apply to field %s of kind %s", m, fd.FullName(), fd.Kind()))
		}

	case ModeMaxEnum:
		if fd.Kind() != protoreflect.EnumKind {
			panic(fmt.Errorf("mode %q does not apply to field %s of kind %s", m, fd.FullName(), fd.Kind()))
		}

	case ModeAppend:
		if !fd.IsList() {
			panic(fmt.Errorf("mode %q does not apply to non-list field %s", m, fd.FullName()))
		}

	default:
		panic(fmt.Errorf("unknown mode: %q", m))
	}
}

func defaultMode(fd protoreflect.FieldDescriptor) ApplyMode {
	if fd.IsList() {
		return ModeAppend
	} else if fd.IsMap() {
		return ModeUpdate
	}
	return ModeSet
}

func computeMode(fd protoreflect.FieldDescriptor, modeMap map[string]ApplyMode) ApplyMode {
	requested := modeMap[string(fd.Name())]
	if requested == ModeDefault {
		return defaultMode(fd)
	}
	requested.assertValidFor(fd)
	return requested
}
