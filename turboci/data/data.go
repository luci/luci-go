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

// Package data contains helpers for working with turboci `data` types.
//
// This covers:
//   - Easy extraction of protobuf Any type_url values given proto messages.
//   - Packing of proto Messages into TurboCI Value messages.
//   - Extraction of proto Messages from TurboCI CheckView messages.
package data

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

const typePrefix = "type.googleapis.com/"

// URL returns the full proto `type_url` for a given message type.
func URL[T proto.Message]() string {
	var msg T
	return URLMsg(msg)
}

// URLMsg returns the full proto `type_url` for a given message.
//
// It's allowed for `msg` to be a nil pointer (e.g. `(*MyMessage)(nil)`).
func URLMsg(msg proto.Message) string {
	return fmt.Sprintf("%s%s", typePrefix, msg.ProtoReflect().Descriptor().FullName())
}

// FromMultiple retrieves the value of the first Datum whose type matches T.
//
// If T is not in the data, returns nil.
//
// Panics if the data is present but fails to unmarshal. This is considered
// an invariant violation, as the data should have been marshaled from the same
// type.
func FromMultiple[T proto.Message](data ...*orchestratorpb.Datum) (ret T) {
	typeURL := URL[T]()
	for _, datum := range data {
		if datum.GetValue().GetValue().GetTypeUrl() == typeURL {
			return ExtractValue[T](datum.GetValue())
		}
	}
	return
}

// GetOption retrieves an option of the given type from a CheckView.
//
// If T is not in the CheckView, returns the zero value for T (e.g. nil for
// a pointer-to-struct type).
//
// Panics if the data is present but fails to unmarshal. This is considered
// an invariant violation, as the data should have been marshaled from the same
// type.
func GetOption[T proto.Message](c *orchestratorpb.Check) (ret T) {
	return FromMultiple[T](c.GetOptions()...)
}

// GetResults retrieves all result data of the given type from a Check.
//
// Returns a mapping of CheckResult.identifier.result.idx -> T.
//
// Panics if the data is present but fails to unmarshal. This is considered an
// invariant violation, as the data must have been marshaled from the same type.
func GetResults[T interface {
	comparable
	proto.Message
}](c *orchestratorpb.Check) map[int32]T {
	ret := make(map[int32]T, len(c.GetResults()))

	var zero T

	for _, result := range c.GetResults() {
		dat := FromMultiple[T](result.GetData()...)
		if dat != zero {
			ret[result.GetIdentifier().GetIdx()] = dat
		}
	}

	return ret
}

// ValueErr wraps a proto.Message into a `*orchestratorpb.Value`, returning
// any error encountered during marshaling.
func ValueErr(msg proto.Message) (*orchestratorpb.Value, error) {
	val, err := anypb.New(msg)
	if err != nil {
		return nil, err
	}
	return orchestratorpb.Value_builder{Value: val}.Build(), nil
}

// ValuesErr wraps proto.Messages into `*orchestratorpb.Value`s, returning any
// error encountered during marshaling.
func ValuesErr(msgs ...proto.Message) ([]*orchestratorpb.Value, error) {
	var errs []error
	ret := make([]*orchestratorpb.Value, len(msgs))
	for i, msg := range msgs {
		val, err := ValueErr(msg)
		if err != nil {
			errs = append(errs, fmt.Errorf("msgs[%d]: %w", i, err))
		} else {
			ret[i] = val
		}
	}
	return ret, errors.Join(errs...)
}

// Value wraps a proto.Message into a `*orchestratorpb.Value`, panicking on any
// error.
func Value(msg proto.Message) *orchestratorpb.Value {
	ret, err := ValueErr(msg)
	if err != nil {
		panic(err)
	}
	return ret
}

// ExtractValue unwraps a proto.Message from a `*orchestratorpb.Value`.
//
// Returns a nil message if `val` is not of a matching type.
func ExtractValue[T proto.Message](val *orchestratorpb.Value) T {
	retMsg, err := val.GetValue().UnmarshalNew()
	if err != nil {
		panic(
			fmt.Sprintf("impossible: proto type %q was unresolvable, but we have it at compile time",
				URL[T]()))
	}

	ret, _ := retMsg.(T)
	return ret
}
