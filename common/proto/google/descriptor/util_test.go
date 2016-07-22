// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package descriptor

import (
	"testing"

	"io/ioutil"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestUtil(t *testing.T) {
	t.Parallel()

	Convey("Util", t, func() {
		descFileBytes, err := ioutil.ReadFile("util_test.desc")
		So(err, ShouldBeNil)

		var desc FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		So(err, ShouldBeNil)

		So(desc.File, ShouldHaveLength, 2)
		file := desc.File[1]
		So(file.GetName(), ShouldEqual, "github.com/luci/luci-go/common/proto/google/descriptor/util_test.proto")

		Convey("Resolve works", func() {
			names := []string{
				"pkg.E1",
				"pkg.E1.V0",

				"pkg.M1",
				"pkg.M1.f1",

				"pkg.M2.f1",
				"pkg.M2.f2",

				"pkg.M3.O1",
				"pkg.M3.f1",
				"pkg.M3.O2",

				"pkg.S1",
				"pkg.S1.R1",
				"pkg.S2.R2",

				"pkg.NestedMessageParent",
				"pkg.NestedMessageParent.NestedMessage",
				"pkg.NestedMessageParent.NestedMessage.f1",
				"pkg.NestedMessageParent.NestedEnum",
				"pkg.NestedMessageParent.NestedEnum.V0",
			}
			for _, n := range names {
				Convey(n, func() {
					actualFile, obj, _ := desc.Resolve(n)
					So(actualFile, ShouldEqual, file)
					So(obj, ShouldNotBeNil)
				})
			}

			Convey("wrong name", func() {
				actualFile, obj, path := desc.Resolve("foo")
				So(actualFile, ShouldBeNil)
				So(obj, ShouldBeNil)
				So(path, ShouldBeNil)
			})
		})
	})
}
