// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	pc "github.com/luci/luci-go/common/prpc"
	"google.golang.org/grpc/codes"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEncoding(t *testing.T) {
	t.Parallel()

	Convey("responseFormat", t, func() {
		test := func(acceptHeader string, expectedFormat pc.Format, expectedErr interface{}) {
			acceptHeader = strings.Replace(acceptHeader, "{json}", mtPRPCJSONPB, -1)
			acceptHeader = strings.Replace(acceptHeader, "{binary}", mtPRPCBinary, -1)
			acceptHeader = strings.Replace(acceptHeader, "{text}", mtPRPCText, -1)

			Convey("Accept: "+acceptHeader, func() {
				actualFormat, err := responseFormat(acceptHeader)
				So(err, ShouldErrLike, expectedErr)
				if err == nil {
					So(actualFormat, ShouldEqual, expectedFormat)
				}
			})
		}

		test("", pc.FormatBinary, nil)
		test(pc.ContentTypePRPC, pc.FormatBinary, nil)
		test(mtPRPCBinary, pc.FormatBinary, nil)
		test(mtPRPCJSONPB, pc.FormatJSONPB, nil)
		test(mtPRPCText, pc.FormatText, nil)
		test(pc.ContentTypeJSON, pc.FormatJSONPB, nil)

		test("application/*", pc.FormatBinary, nil)
		test("*/*", pc.FormatBinary, nil)

		// test cases with multiple types
		test("{json},{binary}", pc.FormatBinary, nil)
		test("{json},{binary};q=0.9", pc.FormatJSONPB, nil)
		test("{json};q=1,{binary};q=0.9", pc.FormatJSONPB, nil)
		test("{json},{text}", pc.FormatJSONPB, nil)
		test("{json};q=0.9,{text}", pc.FormatText, nil)
		test("{binary},{json},{text}", pc.FormatBinary, nil)

		test("{json},{binary},*/*", pc.FormatBinary, nil)
		test("{json},{binary},*/*;q=0.9", pc.FormatBinary, nil)
		test("{json},{binary},*/*;x=y", pc.FormatBinary, nil)
		test("{json},{binary};q=0.9,*/*", pc.FormatBinary, nil)
		test("{json},{binary};q=0.9,*/*;q=0.8", pc.FormatJSONPB, nil)

		// supported and unsupported mix
		test("{json},foo/bar", pc.FormatJSONPB, nil)
		test("{json};q=0.1,foo/bar", pc.FormatJSONPB, nil)
		test("foo/bar;q=0.1,{json}", pc.FormatJSONPB, nil)

		// only unsupported types
		const err406 = "pRPC: Accept header: specified media types are not not supported"
		test(pc.ContentTypePRPC+"; boo=true", 0, err406)
		test(pc.ContentTypePRPC+"; encoding=blah", 0, err406)
		test("x", 0, err406)
		test("x,y", 0, err406)

		test("x//y", 0, "pRPC: Accept header: expected token after slash")
	})

	Convey("respondMessage", t, func() {
		msg := &HelloReply{Message: "Hi"}

		test := func(f pc.Format, body []byte, contentType string) {
			Convey(contentType, func() {
				res := respondMessage(msg, f)
				So(res.code, ShouldEqual, codes.OK)
				So(res.header, ShouldResembleV, http.Header{
					headerContentType: []string{contentType},
				})
				So(res.body, ShouldResembleV, body)
			})
		}

		msgBytes, err := proto.Marshal(msg)
		So(err, ShouldBeNil)

		test(pc.FormatBinary, msgBytes, mtPRPCBinary)
		test(pc.FormatJSONPB, []byte(pc.JSONPBPrefix+"{\"message\":\"Hi\"}\n"), mtPRPCJSONPB)
		test(pc.FormatText, []byte("message: \"Hi\"\n"), mtPRPCText)
	})
}
