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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run(`ok`, func(t *testing.T) {
		t.Parallel()

		vw, err := Write(&emptypb.Empty{})
		assert.NoErr(t, err)
		assert.That(t, vw, should.Match(orchestratorpb.ValueWrite_builder{
			Data:  &anypb.Any{TypeUrl: URL[*emptypb.Empty]()},
			Realm: proto.String(RealmFromContainer),
		}.Build()))
	})

	t.Run(`ok_realm`, func(t *testing.T) {
		t.Parallel()

		vw, err := Write(&emptypb.Empty{}, "project:realm")
		assert.NoErr(t, err)
		assert.That(t, vw, should.Match(orchestratorpb.ValueWrite_builder{
			Data:  &anypb.Any{TypeUrl: URL[*emptypb.Empty]()},
			Realm: proto.String("project:realm"),
		}.Build()))
	})

	t.Run(`err_two_realms`, func(t *testing.T) {
		t.Parallel()

		_, err := Write(&emptypb.Empty{}, "project:realm", "what")
		assert.ErrIsLike(t, err, "realm provided more than once")
	})

	t.Run(`err_any`, func(t *testing.T) {
		t.Parallel()

		_, err := Write(&anypb.Any{})
		assert.ErrIsLike(t, err, "cannot handle google.protobuf.Any")
	})
}
