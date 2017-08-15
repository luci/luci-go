// Copyright 2016 The LUCI Authors.
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

package cloud

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/gae/service/info"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	Convey(`A testing Info service`, t, func() {
		const maxNamespaceLen = 100

		gi := serviceInstanceGlobalInfo{
			Config: &Config{
				IsDev:              false,
				ProjectID:          "project-id",
				ServiceName:        "service-name",
				VersionName:        "version-name",
				InstanceID:         "instance-id",
				ServiceAccountName: "service-account@example.com",
				ServiceProvider:    nil,
			},
			Request: &Request{
				TraceID: "trace",
			},
		}
		c := useInfo(context.Background(), &gi)

		Convey(`Can set valid namespaces.`, func() {
			for _, v := range []string{
				"",
				"test",
				"0123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_",
				strings.Repeat("X", maxNamespaceLen),
			} {
				Convey(fmt.Sprintf(`Rejects %q`, v), func() {
					c, err := info.Namespace(c, v)
					So(err, ShouldBeNil)
					So(info.GetNamespace(c), ShouldEqual, v)
				})
			}
		})

		Convey(`Rejects invalid namespaces on the client.`, func() {
			for _, v := range []string{
				" ",
				strings.Repeat("X", maxNamespaceLen+1),
			} {
				Convey(fmt.Sprintf(`Rejects %q`, v), func() {
					_, err := info.Namespace(c, v)
					So(err, ShouldErrLike, "does not match")
				})
			}
		})
	})
}
