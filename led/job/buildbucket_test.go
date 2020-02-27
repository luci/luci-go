// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"testing"

	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBBEnsureBasics(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.EnsureBasics`, t, func() {
		jd := testBBJob()
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild(), ShouldBeNil)

		jd.GetBuildbucket().EnsureBasics()

		So(jd.GetBuildbucket().BbagentArgs.Build.Infra, ShouldNotBeNil)
	})
}

func TestWriteProperties(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.WriteProperties`, t, func() {
		jd := testBBJob()
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldBeNil)

		jd.GetBuildbucket().WriteProperties(map[string]interface{}{
			"hello": "world",
		})
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldResemble, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"hello": {Kind: &structpb.Value_StringValue{StringValue: "world"}},
			},
		})
	})
}
