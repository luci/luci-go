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

package policy

import (
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEntitiesWork(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		c := gaetesting.TestingContext()

		hdr, err := getImportedPolicyHeader(c, "policy name")
		So(err, ShouldBeNil)
		So(hdr, ShouldBeNil)

		body, err := getImportedPolicyBody(c, "policy name")
		So(err, ShouldBeNil)
		So(body, ShouldBeNil)

		So(updateImportedPolicy(c, "policy name", "rev", "sha256", []byte("body")), ShouldBeNil)

		hdr, err = getImportedPolicyHeader(c, "policy name")
		So(err, ShouldBeNil)
		So(hdr, ShouldResemble, &importedPolicyHeader{
			Name:     "policy name",
			Revision: "rev",
			SHA256:   "sha256",
		})

		body, err = getImportedPolicyBody(c, "policy name")
		So(err, ShouldBeNil)
		So(body, ShouldResemble, &importedPolicyBody{
			Parent:   datastore.KeyForObj(c, hdr),
			Revision: "rev",
			SHA256:   "sha256",
			Data:     []byte("body"),
		})
	})
}
