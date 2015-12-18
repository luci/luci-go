// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbucket

import (
	"testing"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateProtoMessage(t *testing.T) {
	tm := TaskManager{}

	Convey("ValidateProtoMessage passes good msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server:     strPtr("https://blah.com"),
			Bucket:     strPtr("bucket"),
			Builder:    strPtr("builder"),
			Tags:       []string{"a:b", "c:d"},
			Properties: []string{"a:b", "c:d"},
		}), ShouldBeNil)
	})

	Convey("ValidateProtoMessage passes good minimal msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server:  strPtr("https://blah.com"),
			Bucket:  strPtr("bucket"),
			Builder: strPtr("builder"),
		}), ShouldBeNil)
	})

	Convey("ValidateProtoMessage wrong type", t, func() {
		So(tm.ValidateProtoMessage(&messages.NoopTask{}), ShouldErrLike, "wrong type")
	})

	Convey("ValidateProtoMessage empty", t, func() {
		So(tm.ValidateProtoMessage(tm.ProtoMessageType()), ShouldErrLike, "field 'server' is required")
	})

	Convey("ValidateProtoMessage validates URL", t, func() {
		call := func(url string) error {
			return tm.ValidateProtoMessage(&messages.BuildbucketTask{
				Server:  &url,
				Bucket:  strPtr("bucket"),
				Builder: strPtr("builder"),
			})
		}
		So(call(""), ShouldErrLike, "field 'server' is required")
		So(call("%%%%"), ShouldErrLike, "invalid URL")
		So(call("/abc"), ShouldErrLike, "not an absolute url")
		So(call("https://host/not-root"), ShouldErrLike, "not a host root url")
	})

	Convey("ValidateProtoMessage needs bucket", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server:  strPtr("https://blah.com"),
			Builder: strPtr("builder"),
		}), ShouldErrLike, "'bucket' field is required")
	})

	Convey("ValidateProtoMessage needs builder", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server: strPtr("https://blah.com"),
			Bucket: strPtr("bucket"),
		}), ShouldErrLike, "'builder' field is required")
	})

	Convey("ValidateProtoMessage validates properties", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server:     strPtr("https://blah.com"),
			Bucket:     strPtr("bucket"),
			Builder:    strPtr("builder"),
			Properties: []string{"not_kv_pair"},
		}), ShouldErrLike, "bad property, not a 'key:value' pair")
	})

	Convey("ValidateProtoMessage validates tags", t, func() {
		So(tm.ValidateProtoMessage(&messages.BuildbucketTask{
			Server:  strPtr("https://blah.com"),
			Bucket:  strPtr("bucket"),
			Builder: strPtr("builder"),
			Tags:    []string{"not_kv_pair"},
		}), ShouldErrLike, "bad tag, not a 'key:value' pair")
	})
}

//////////////

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int32 {
	j := int32(i)
	return &j
}
