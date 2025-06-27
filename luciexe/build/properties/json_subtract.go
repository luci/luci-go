// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

var null = structpb.NewNullValue()

func subtractList(a, b *structpb.ListValue) (empty bool) {
	if a == nil || len(b.Values) == 0 {
		return true
	}
	if len(a.Values) != len(b.Values) {
		// This should never happen, but if it does, assume whatever is leftover in
		// `a` stays in `a`
		return len(a.Values) == 0
	}
	var leftover []*structpb.Value
	for i, lhsValue := range a.Values {
		switch lhs := lhsValue.Kind.(type) {
		case *structpb.Value_ListValue:
			if rhs := b.Values[i].GetListValue(); rhs != nil {
				if empty := subtractList(lhs.ListValue, rhs); !empty {
					if leftover == nil {
						leftover = make([]*structpb.Value, i, len(a.Values))
						for j := range i {
							leftover[j] = null
						}
					}
					leftover = append(leftover, lhsValue)
				}
			}

		case *structpb.Value_StructValue:
			if rhs := b.Values[i].GetStructValue(); rhs != nil {
				if empty := subtractStruct(lhs.StructValue, rhs); !empty {
					if leftover == nil {
						leftover = make([]*structpb.Value, i, len(a.Values))
						for j := range i {
							leftover[j] = null
						}
					}
					leftover = append(leftover, lhsValue)
				}
			}

		default:
			// everything else is scalar type
			if leftover != nil {
				leftover = append(leftover, null)
			}
		}

	}

	a.Values = leftover
	return len(a.Values) == 0
}

// subtractStruct removes all fields in `a` which are present in `b`,
// recursively.
func subtractStruct(a, b *structpb.Struct) (empty bool) {
	if a == nil {
		return true
	}
	for key, value := range b.Fields {
		if sub := value.GetStructValue(); sub != nil {
			if empty := subtractStruct(a.Fields[key].GetStructValue(), sub); empty {
				delete(a.Fields, key)
			}
		} else if lst := value.GetListValue(); lst != nil {
			if empty := subtractList(a.Fields[key].GetListValue(), lst); empty {
				delete(a.Fields, key)
			}
		} else {
			// scalar
			delete(a.Fields, key)
		}
	}
	return len(a.Fields) == 0
}

func handleInputLogging(ctx context.Context, ns string, buf []byte, unknown unknownFieldSetting, original *structpb.Struct, targets []*structpb.Struct) (bool, error) {
	leftovers := proto.Clone(original).(*structpb.Struct)
	for _, target := range targets {
		if empty := subtractStruct(leftovers, target); empty {
			return false, nil
		}
	}

	level := logging.Warning
	if unknown == rejectUnknownFields {
		level = logging.Error
	}
	leftoverJSON, err := protojson.MarshalOptions{}.MarshalAppend(buf[:0], leftovers)
	if err != nil {
		return false, errors.Fmt("impossible - could not marshal leftover: %w", err)
	}

	logging.Logf(ctx, level, "Unknown fields while parsing property namespace %q: %s", ns, leftoverJSON)
	return unknown == rejectUnknownFields, nil
}
