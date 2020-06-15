// Copyright 2020 The LUCI Authors.
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

package sink

import (
	"context"
	"testing"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactChannel(t *testing.T) {
	t.Parallel()

	Convey("schedule works", t, func() {
		ctx := context.Background()
		cfg := testServerConfig(nil, "127.0.0.1:123", "secret")
		ac := newArtifactChannel(ctx, &cfg)

		// send a sample request
		tr, cleanup := validTestResult()
		defer cleanup()
		art := &sinkpb.Artifact{Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}
		tr.Artifacts = map[string]*sinkpb.Artifact{"art1": art}
		ac.schedule(tr)
		ac.closeAndDrain(ctx)

		// TODO(ddoman): validate the uploaded artifacts
	})
}
