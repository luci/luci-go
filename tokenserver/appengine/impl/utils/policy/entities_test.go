// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/gaetesting"

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
