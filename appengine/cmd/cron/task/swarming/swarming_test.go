// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"testing"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateProtoMessage(t *testing.T) {
	tm := TaskManager{}

	Convey("ValidateProtoMessage passes good msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:     strPtr("https://blah.com"),
			Command:    []string{"echo", "Hi!"},
			Env:        []string{"A=B", "C=D"},
			Dimensions: []string{"OS:Linux"},
			Tags:       []string{"a:b", "c:d"},
			Priority:   intPtr(50),
		}), ShouldBeNil)
	})

	Convey("ValidateProtoMessage passes good minimal msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:  strPtr("https://blah.com"),
			Command: []string{"echo", "Hi!"},
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
			return tm.ValidateProtoMessage(&messages.SwarmingTask{
				Server:  &url,
				Command: []string{"echo", "Hi!"},
			})
		}
		So(call(""), ShouldErrLike, "field 'server' is required")
		So(call("%%%%"), ShouldErrLike, "invalid URL")
		So(call("/abc"), ShouldErrLike, "not an absolute url")
		So(call("https://host/not-root"), ShouldErrLike, "not a host root url")
	})

	Convey("ValidateProtoMessage validates environ", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:  strPtr("https://blah.com"),
			Command: []string{"echo", "Hi!"},
			Env:     []string{"not_kv_pair"},
		}), ShouldErrLike, "bad environment variable, not a 'key=value' pair")
	})

	Convey("ValidateProtoMessage validates dimensions", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:     strPtr("https://blah.com"),
			Command:    []string{"echo", "Hi!"},
			Dimensions: []string{"not_kv_pair"},
		}), ShouldErrLike, "bad dimension, not a 'key:value' pair")
	})

	Convey("ValidateProtoMessage validates tags", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:  strPtr("https://blah.com"),
			Command: []string{"echo", "Hi!"},
			Tags:    []string{"not_kv_pair"},
		}), ShouldErrLike, "bad tag, not a 'key:value' pair")
	})

	Convey("ValidateProtoMessage validates priority", t, func() {
		call := func(priority int32) error {
			return tm.ValidateProtoMessage(&messages.SwarmingTask{
				Server:   strPtr("https://blah.com"),
				Command:  []string{"echo", "Hi!"},
				Priority: &priority,
			})
		}
		So(call(-1), ShouldErrLike, "bad priority")
		So(call(256), ShouldErrLike, "bad priority")
	})

	Convey("ValidateProtoMessage accepts input_ref or command, not both", t, func() {
		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server: strPtr("https://blah.com"),
		}), ShouldErrLike, "one of 'command' or 'isolated_ref' is required")

		So(tm.ValidateProtoMessage(&messages.SwarmingTask{
			Server:      strPtr("https://blah.com"),
			Command:     []string{"echo", "Hi!"},
			IsolatedRef: &messages.SwarmingTask_IsolatedRef{},
		}), ShouldErrLike, "only one of 'command' or 'isolated_ref' must be specified")
	})
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int32 {
	j := int32(i)
	return &j
}
