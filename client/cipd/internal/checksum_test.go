// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"testing"

	"github.com/luci/luci-go/client/cipd/internal/messages"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChecksumCheckingWorks(t *testing.T) {
	msg := messages.TagCache{
		Entries: []*messages.TagCache_Entry{
			{
				Package:    strPtr("package"),
				Tag:        strPtr("tag"),
				InstanceId: strPtr("instance_id"),
			},
		},
	}

	Convey("Works", t, func(c C) {
		buf, err := MarshalWithSHA1(&msg)
		So(err, ShouldBeNil)
		out := messages.TagCache{}
		So(UnmarshalWithSHA1(buf, &out), ShouldBeNil)
		So(out, ShouldResemble, msg)
	})

	Convey("Rejects bad msg", t, func(c C) {
		buf, err := MarshalWithSHA1(&msg)
		So(err, ShouldBeNil)
		buf[10] = 0
		out := messages.TagCache{}
		So(UnmarshalWithSHA1(buf, &out), ShouldNotBeNil)
	})
}

func strPtr(s string) *string {
	return &s
}
