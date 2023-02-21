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

package pubsub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

func TestBuildBucketPubsub(t *testing.T) {
	t.Parallel()

	Convey("Buildbucket Pubsub Handler", t, func() {
		c := context.Background()
		buildExp := bbv1.LegacyApiCommonBuildMessage{
			Project: "fake",
			Bucket:  "luci.fake.bucket",
			Id:      87654321,
			Status:  bbv1.StatusCompleted,
		}
		r := &http.Request{Body: makeBBReq(buildExp, "bb-hostname")}
		err := buildbucketPubSubHandlerImpl(c, r)
		So(err, ShouldBeNil)
	})
}

func makeBBReq(build bbv1.LegacyApiCommonBuildMessage, hostname string) io.ReadCloser {
	bmsg := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
	}{build, hostname}
	bm, _ := json.Marshal(bmsg)

	msg := struct {
		Message struct {
			Data []byte
		}
		Attributes map[string]any
	}{struct{ Data []byte }{Data: bm}, nil}
	jmsg, _ := json.Marshal(msg)
	return io.NopCloser(bytes.NewReader(jmsg))
}
