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

import (
	"sync/atomic"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	handleNum atomic.Int64
)

// DataHandle is a process wide unique number which can be used as a key for
// ExecuteWithData.
type DataHandle[T any] struct{ idx int64 }

// Value creates a new DataValue binding for use with [DynamicWalker.Execute]
// or [Walker[M].Execute].
func (d DataHandle[T]) Value(val T) DataValue {
	if d.idx == 0 {
		panic("DataHandle was improperly initialized - use NewDataHandle[T].")
	}
	return DataValue{d.idx, val}
}

// Get retrieves the value from DataMap, or T's zero value if the entry is
// not present.
func (d DataHandle[T]) Get(m DataMap) T {
	retT, _ := m.dat[d.idx].(T)
	return retT
}

// NewDataHandle returns a new DataHandle which can be used with
// ExecuteWithData.
//
// Just like [NewWalker]/[NewDynamicWalker], this should be called at init()
// time to get a handle which is used for the lifetime of the process.
//
// See the example for [DataMap].
func NewDataHandle[T any]() DataHandle[T] {
	idx := handleNum.Add(1)
	return DataHandle[T]{idx + 1}
}

// DataValue is a value bound with a specific [DataHandle[T]].
type DataValue struct {
	idx int64
	val any
}

// DataMap contains zero or more DataValue bindings from specific [DataHandles].
//
// Use a specific [DataHandle[T]] to retrieve a value from this map in
// [FieldProcessor.Process].
type DataMap struct{ dat map[int64]any }

// FieldProcessor allows processing a set of proto message fields in conjunction
// with [NewWalker].
//
// Typically FieldProcessor implementations will apply to fields with particular
// annotations, but a FieldProcessor can technically react to any field(s) that
// it wants to.
type FieldProcessor interface {
	// ShouldProcess is called once per field descriptor in
	// [NewWalker]/[NewDynamicWalker].
	//
	// Note that this is NOT called in [Walker.Execute]/[DynamicWalker.Execute].
	// The return value of this is cached in the [Walker]/[DynamicWalker].
	//
	// Returns an enum of how this processor wants to handle the provided field.
	ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr

	// Process is called when examining a message with [Walker.Execute] or
	// [DynamicWalker.Execute].
	//
	// It will only be called on fields where ShouldProcess returned
	// a [ProcessAttr] value other than [ProcessNever].
	//
	// Process will never be invoked for a field on a nil message. That is,
	// technically, someMessage.someField is 'unset', even if someMessage is nil.
	// Even if the FieldSelector returned ProccessUnset, it would still not be
	// called on someField.
	//
	// If `applied` == true, `data` will be included in the Results from
	// protowalk.Fields.
	//
	// It is allowed for Process to mutate the value of `field` in `msg`, but
	// mutating other fields is undefined behavior.
	//
	// When processing a given message, an instance of FieldProcessor will have
	// its Process method called sequentially per affected field, interspersed
	// with other FieldProcessors in the same Fields call. For example, if you
	// process a message with FieldProcessors A and B, where A processes evenly-
	// numbered fields, and B processes oddly-numbered fields, the calls would
	// look like:
	//   * B.Process(1)
	//   * A.Process(2)
	//   * B.Process(3)
	//
	// If two processors apply to the same field in a message, they'll be called
	// in the order specified to Fields (i.e. NewWalker(..., A{}, B{}) would call A
	// then B, and NewWalker(..., B{}, A{}) would call B then A).
	//
	// `data` is the same data as provided to ExecuteWithData. You can retrieve
	// values form it using a [DataHandle[T]].
	Process(data DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (result ResultData, applied bool)
}
