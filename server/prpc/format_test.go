// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"mime"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFormat(t *testing.T) {
	t.Parallel()

	Convey("parseFormat", t, func() {
		test := func(mediaType string, expectedFormat format, expectedErr interface{}) {
			Convey("mediaType="+mediaType, func() {
				mediaType, params, err := mime.ParseMediaType(mediaType)
				So(err, ShouldBeNil)
				actualFormat, err := parseFormat(mediaType, params)
				So(err, ShouldErrLike, expectedErr)
				if err == nil {
					So(actualFormat, ShouldEqual, expectedFormat)
				}
			})
		}

		test(mtGRPC, formatBinary, nil)
		test(mtPRPC, formatBinary, nil)
		test(mtPRPCBinary, formatBinary, nil)
		test(mtPRPCJSNOPB, formatJSONPB, nil)
		test(mtPRPCText, formatText, nil)
		test(
			mtPRPC+"; encoding=blah",
			0,
			`encoding parameter: invalid value "blah". Valid values: "json", "binary", "text"`)
		test(mtPRPC+"; boo=true", 0, `unexpected parameter "boo"`)

		test(mtJSON, formatJSONPB, nil)
		test(mtJSON+"; whatever=true", formatJSONPB, nil)

		test("x", formatUnrecognized, nil)

		Convey("mediaType=", func() {
			actualFormat, err := parseFormat("", nil)
			So(err, ShouldBeNil)
			So(actualFormat, ShouldEqual, formatUnspecified)
		})
	})
}
