// Copyright 2017 The LUCI Authors.
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

package srcman

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/proto/milo"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/fakelogs"
	srcman_pb "go.chromium.org/luci/milo/api/proto/manifest"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
)

func init() {
	rawpresentation.AcceptableLogdogHosts.Add("example.com")
}

func TestGet(t *testing.T) {
	t.Parallel()

	demoID, _ := hex.DecodeString(strings.Repeat("deadbeef", 8))

	Convey(`Test srcman.Get`, t, func() {
		ctx := memory.Use(context.Background())

		demoData := &srcman_pb.Manifest{
			Directories: map[string]*srcman_pb.Manifest_Directory{
				"something": {
					GitCheckout: &srcman_pb.Manifest_GitCheckout{
						RepoUrl:  "https://example.com/repo/path.git",
						Revision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					},
				},
			},
		}
		demo := newSrcManCacheEntry(demoID)

		putDemo := func() {
			var err error
			demo.Data, err = proto.Marshal(demoData)
			So(err, ShouldErrLike, nil)
			So(datastore.Put(ctx, demo), ShouldErrLike, nil)
		}

		Convey(`cache`, func() {
			Convey(`read OK`, func() {
				putDemo()
				m, hsh, err := Get(ctx, &milo.Step_ManifestLink{Sha256: demoID})
				So(err, ShouldErrLike, nil)
				So(m, ShouldResemble, demoData)
				So(bytes.Equal(demoID, hsh), ShouldBeTrue)

				er, err := datastore.Exists(ctx, demo)
				So(err, ShouldErrLike, nil)
				So(er.All(), ShouldBeTrue)
			})

			Convey(`bad Read`, func() {
				demo.Data = []byte("I am a banana, and not a proto")
				So(datastore.Put(ctx, demo), ShouldErrLike, nil)

				_, _, err := Get(ctx, &milo.Step_ManifestLink{Sha256: demoID, Url: "banana://fweep"})
				So(err, ShouldErrLike, `unsupported URL scheme`)

				// However, bad data gets deleted as a side effect
				er, err := datastore.Exists(ctx, demo)
				So(err, ShouldErrLike, nil)
				So(er.Any(), ShouldBeFalse)
			})
		})

		Convey(`uncached`, func() {
			ldc := fakelogs.NewClient()
			ctx = rawpresentation.InjectFakeLogdogClient(ctx, ldc)

			data, err := proto.Marshal(demoData)
			So(err, ShouldErrLike)

			s, err := ldc.OpenBinaryStream("something", "whatever")
			So(err, ShouldErrLike)
			s.Write(data)
			s.Close()

			manifest, hsh, err := Get(ctx, &milo.Step_ManifestLink{
				Url: fmt.Sprintf("logdog://example.com/%s/something/+/whatever", fakelogs.Project),
			})
			So(err, ShouldErrLike)
			So(manifest, ShouldResemble, demoData)

			Convey(`cached after logdog read`, func() {
				manifest2, hsh2, err := Get(ctx, &milo.Step_ManifestLink{
					Sha256: hsh,
					Url:    "monkeyspit",
				})
				So(err, ShouldErrLike)
				So(hsh2, ShouldResemble, hsh)
				So(manifest2, ShouldResemble, demoData)
			})
		})
	})
}
