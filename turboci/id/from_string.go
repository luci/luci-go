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

package id

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

func require[T proto.Message](msg proto.Message, errP *error) T {
	var zero T
	if *errP != nil {
		return zero
	}
	if msg == nil {
		*errP = fmt.Errorf("missing required %q",
			zero.ProtoReflect().Descriptor().Name())
		return zero
	}
	ret, ok := msg.(T)
	if !ok {
		*errP = fmt.Errorf(
			"expected %q, got %q",
			zero.ProtoReflect().Descriptor().Name(),
			msg.ProtoReflect().Descriptor().Name())
	}
	return ret
}

func requireInt(trimmed string, errP *error) *int32 {
	if *errP != nil {
		return nil
	}
	ret, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		*errP = fmt.Errorf("bad idx: %w", err)
		return nil
	} else if ret < 0 {
		*errP = fmt.Errorf("bad idx: must be > 0")
		return nil
	}
	ret32 := int32(ret)
	return &ret32
}

func requireTs(trimmed string, errP *error) *timestamppb.Timestamp {
	if *errP != nil {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		*errP = fmt.Errorf("bad timestamp: needed seconds/nanos")
		return nil
	}
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		*errP = fmt.Errorf("bad timestamp: parsing seconds %q: %w", parts[0], err)
		return nil
	}
	nanos, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		*errP = fmt.Errorf("bad timestamp: parsing nanos %q: %w", parts[1], err)
		return nil
	}
	return &timestamppb.Timestamp{Seconds: seconds, Nanos: int32(nanos)}
}

// FromString converts a canonical string representation of a TurboCI identifier
// into its protobuf message form.
//
// The string format is defined in [identifier.proto].
//
// Returns an error if the string is malformed.
//
// [identifier.proto]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/ids/v1/identifier.proto
func FromString(id string) (*idspb.Identifier, error) {
	toks := strings.Split(id, ":")

	// current is the currently parsed identifier so far.
	// This will always either be nil or one of the types in [Identifier].
	var current proto.Message

	for i, tok := range toks {
		if len(tok) == 0 {
			if i == 0 {
				// workplan allowed to be empty
			} else {
				return nil, fmt.Errorf("token %d in %q was empty?", i, id)
			}
		} else {
			key := tok[0]
			trimmed := tok[1:]
			var err error
			badKeyErr := func() error { return fmt.Errorf("unexpected key %q", key) }

			if i != 0 && len(trimmed) == 0 {
				err = fmt.Errorf("unexpected empty token")
			} else {
				switch key {
				case 'L':
					if current != nil {
						err = badKeyErr()
					} else if len(trimmed) > 0 {
						current = idspb.WorkPlan_builder{Id: &trimmed}.Build()
					}

				case 'C':
					var wp *idspb.WorkPlan
					if current != nil {
						wp = require[*idspb.WorkPlan](current, &err)
					}
					current = idspb.Check_builder{
						WorkPlan: wp,
						Id:       &trimmed,
					}.Build()

				case 'R':
					current = idspb.CheckResult_builder{
						Check: require[*idspb.Check](current, &err),
						Idx:   requireInt(trimmed, &err),
					}.Build()

				case 'D':
					current = idspb.CheckResultDatum_builder{
						Result: require[*idspb.CheckResult](current, &err),
						Idx:    requireInt(trimmed, &err),
					}.Build()

				case 'V':
					switch x := current.(type) {
					case *idspb.Check:
						current = idspb.CheckEdit_builder{
							Check:   x,
							Version: requireTs(trimmed, &err),
						}.Build()

					case *idspb.Stage:
						current = idspb.StageEdit_builder{
							Stage:   x,
							Version: requireTs(trimmed, &err),
						}.Build()

					default:
						err = badKeyErr()
					}

				case 'O':
					switch x := current.(type) {
					case *idspb.Check:
						current = idspb.CheckOption_builder{
							Check: x,
							Idx:   requireInt(trimmed, &err),
						}.Build()

					case *idspb.CheckEdit:
						current = idspb.CheckEditOption_builder{
							CheckEdit: x,
							Idx:       requireInt(trimmed, &err),
						}.Build()

					default:
						err = badKeyErr()
					}

				case 'N', 'S', '?':
					var wp *idspb.WorkPlan
					if current != nil {
						wp = require[*idspb.WorkPlan](current, &err)
					}
					stg := idspb.Stage_builder{
						WorkPlan: wp,
						Id:       &trimmed,
					}.Build()
					if key != '?' {
						stg.SetIsWorknode(key == 'N')
					}
					current = stg

				case 'A':
					current = idspb.StageAttempt_builder{
						Stage: require[*idspb.Stage](current, &err),
						Idx:   requireInt(trimmed, &err),
					}.Build()

				default:
					err = badKeyErr()
				}
			}

			if err != nil {
				return nil, fmt.Errorf("token %d in %q: %w", i, id, err)
			}
		}
	}

	return wrap(current), nil
}
