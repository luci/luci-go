// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// These constants are special values which can be used with [Write].
//
// See the documentation on [orchestratorpb.ValueWrite] for more info.
const (
	// RealmFromToken can be used in place of a concrete realm to instruct the
	// orchestrator to use the realm from the writer's token (e.g. WorkPlan
	// creation token or Stage Attempt token).
	RealmFromToken string = "$from_token"

	// RealmFromContainer can be used in place of a concrete realm to instruct
	// the orchestrator to use the realm from the container where this ValueWrite
	// will be used:
	//   - Checks and Stages are contained in the WorkPlan.
	//   - Check Options, Check Result Data, etc. are contained in their
	//     Check.
	//   - Stage Attempt Details, Stage Attempt Progress Details, etc. are
	//     contained in their Stage.
	//   - Edit Reasons for a Check or Stage are contained in their
	//     respective Edit, which always has the same realm as the Check or
	//     Stage to which the Edit belongs.
	RealmFromContainer string = "$from_container"
)

// Write returns a [ValueWrite] for use with TurboCI write APIs (e.g.
// WriteNodes)
//
// Realm should be a full global realm (e.g. `proj:realm`), or may be one of the
// two special values [RealmFromToken] and [RealmFromContainer].
//
// This can only return an error on a proto marshaling error (e.g. if `msg` is
// nil). If you statically know that this cannot be problematic (e.g. your
// binary is known to have functioning proto support), consider [MustWrite].
//
// `realm` is a SINGULAR optional field. If omitted, RealmFromContainer
// is used. If provided, this will be used as the realm. Providing it multiple
// times is an error.
func Write(msg proto.Message, realm ...string) (*orchestratorpb.ValueWrite, error) {
	apb, err := anypb.New(msg)
	if err != nil {
		return nil, err
	}
	actualRealm := RealmFromContainer
	if len(realm) == 1 {
		actualRealm = realm[0]
	} else if len(realm) > 1 {
		return nil, fmt.Errorf("value.Write: realm provided more than once")
	}

	return orchestratorpb.ValueWrite_builder{
		Data:  apb,
		Realm: &actualRealm,
	}.Build(), nil
}

// MustWrite is like [Write], but panics on error.
//
// Like [Write], `realm` must be provided 0 or 1 times, and defaults to
// RealmFromContainer if omitted.
func MustWrite(msg proto.Message, realm ...string) *orchestratorpb.ValueWrite {
	ret, err := Write(msg, realm...)
	if err != nil {
		panic(err)
	}
	return ret
}

// Writes is the same as [Write], except it processes multiple messages, giving
// them all the same realm.
func Writes(msgs []proto.Message, realm ...string) ([]*orchestratorpb.ValueWrite, error) {
	ret := make([]*orchestratorpb.ValueWrite, len(msgs))
	for i, msg := range msgs {
		var err error
		ret[i], err = Write(msg, realm...)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

// MustWrites is the same as [Writes], except it panics on error.
func MustWrites(msgs []proto.Message, realm ...string) []*orchestratorpb.ValueWrite {
	ret, err := Writes(msgs, realm...)
	if err != nil {
		panic(err)
	}
	return ret
}
