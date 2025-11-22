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

package delta

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"google.golang.org/protobuf/proto"
)

// Template allows you to create Diffs for a given proto message type using the
// 'Opaque' API.
type Template[B interface{ Build() T }, T proto.Message] struct {
	RawTemplate[T]
}

// MakeTemplate returns a new Template[B, T], can be used like:
//
//	var template = delta.NewTemplate[MyMessage_builder](map[string]ApplyMode{
//	  "field": delta.ModeAppend,
//	})
//
// Where the modeMap is mapping from proto field name to the ApplyMode to use
// for that field. Providing nil is fine (all fields will be set to their
// default [ApplyMode]).
//
// This will panic if any provided field name is not in `T`.
func MakeTemplate[B interface{ Build() T }, T proto.Message](modeMap map[string]ApplyMode) Template[B, T] {
	return Template[B, T]{MakeRawTemplate[T](modeMap)}
}

// RawTemplate allows you to create Diffs for a given proto message type using
// the 'Open' API.
type RawTemplate[T proto.Message] struct {
	applyMode []ApplyMode
}

// MakeRawTemplate returns a new RawTemplate[T], can be used like:
//
//	var template = delta.NewTemplate[*MyMessage](map[string]ApplyMode{
//	  "field": delta.ModeAppend,
//	})
//
// Where the modeMap is mapping from proto field name to the ApplyMode to use
// for that field. Providing nil is fine (all fields will be set to their
// default [ApplyMode]).
//
// This will panic if any provided field name is not in `T`.
func MakeRawTemplate[T proto.Message](modeMap map[string]ApplyMode) RawTemplate[T] {
	return RawTemplate[T]{applyMode: computeApplyModeSlice[T](modeMap)}
}

// New constructs a Diff from the builder type of the Template's message.
//
// Builder structs are generated for Opaque API proto stubs. If your protos are
// proto3 or otherwise using the older Open API, see [RawTemplate.NewRaw].
//
// Example:
//
//	template.New(MyMessage_builder{
//	  Field: proto.String("hello"),
//	}, anError, anotherErr)
//
// Yields a `*Diff[*MyMessage]` with the given changes, or an error, if anError
// or anotherErr were nil.
func (t Template[B, T]) New(builder B, errs ...error) *Diff[T] {
	return t.NewRaw(builder.Build(), errs...)
}

// NewRaw constructs a Diff from an instance of a message.
//
// Useful for constructing Diffs from Open API proto messages.
// If you're using Opaque API messages, see [Template.New].
//
// Example:
//
//	template.NewRaw(&MyLegacyMessage{
//	  Field: "hello",
//	}, anError, anotherErr)
//
// Yields a `*Diff[*MyMessage]` with the given changes, or an error, if anError
// or anotherErr were nil.
func (t *RawTemplate[T]) NewRaw(msg T, errs ...error) *Diff[T] {
	err := errors.Join(errs...)
	if err != nil {
		return &Diff[T]{err: err}
	}
	return &Diff[T]{
		msg:       msg,
		applyMode: t.applyMode,
	}
}

func computeApplyModeSlice[T proto.Message](modeMap map[string]ApplyMode) []ApplyMode {
	modeMap = maps.Clone(modeMap)

	var zero T
	fields := zero.ProtoReflect().Descriptor().Fields()
	applyMode := make([]ApplyMode, fields.Len())
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		applyMode[i] = computeMode(fd, modeMap)
		delete(modeMap, string(fd.Name()))
	}
	if len(modeMap) > 0 {
		panic(fmt.Errorf("delta.MakeRawTemplate[%T]: no such fields: %v",
			zero, slices.Sorted(maps.Keys(modeMap))))
	}
	return applyMode
}
