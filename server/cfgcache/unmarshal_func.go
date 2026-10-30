// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
)

type (
	// UnmarshalFunc is the type of function used to unmarshal raw config data
	// from LUCI Config into a `T`.
	UnmarshalFunc[T any] func(raw string) (T, error)

	// genericUnmarshalFunc is a type-erased UnmarshalFunc.
	genericUnmarshalFunc = UnmarshalFunc[any]
)

func (u UnmarshalFunc[T]) asGeneric() (genericUnmarshalFunc, error) {
	if u != nil {
		return func(raw string) (any, error) {
			return u(raw)
		}, nil
	}

	var zero T
	if pm, ok := any(zero).(proto.Message); ok && pm.ProtoReflect() != nil {
		mt := pm.ProtoReflect().Type()
		return func(raw string) (any, error) {
			ret := mt.New().Interface()
			if err := prototext.Unmarshal([]byte(raw), ret); err != nil {
				return nil, errors.Fmt("as text proto: %w", err)
			}
			return ret, nil
		}, nil
	}

	switch any(zero).(type) {
	case string:
		return func(raw string) (any, error) {
			return raw, nil
		}, nil
	case []byte:
		return func(raw string) (any, error) {
			return []byte(raw), nil
		}, nil
	default:
		return nil, errors.Fmt("Unmarshal must be set for non-proto type %T", zero)
	}
}
