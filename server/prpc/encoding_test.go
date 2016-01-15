// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEncoding(t *testing.T) {
	t.Parallel()

	Convey("responseFormat", t, func() {
		test := func(acceptHeader string, expectedFormat format, expectedErr interface{}) {
			acceptHeader = strings.Replace(acceptHeader, "{json}", mtPRPCJSNOPB, -1)
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

		test("", formatBinary, nil)
		test(mtPRPC, formatBinary, nil)
		test(mtPRPCBinary, formatBinary, nil)
		test(mtPRPCJSNOPB, formatJSONPB, nil)
		test(mtPRPCText, formatText, nil)
		test(mtJSON, formatJSONPB, nil)

		test("application/*", formatBinary, nil)
		test("*/*", formatBinary, nil)

		// test cases with multiple types
		test("{json},{binary}", formatBinary, nil)
		test("{json},{binary};q=0.9", formatJSONPB, nil)
		test("{json};q=1,{binary};q=0.9", formatJSONPB, nil)
		test("{json},{text}", formatJSONPB, nil)
		test("{json};q=0.9,{text}", formatText, nil)
		test("{binary},{json},{text}", formatBinary, nil)

		test("{json},{binary},*/*", formatBinary, nil)
		test("{json},{binary},*/*;q=0.9", formatBinary, nil)
		test("{json},{binary},*/*;x=y", formatBinary, nil)
		test("{json},{binary};q=0.9,*/*", formatBinary, nil)
		test("{json},{binary};q=0.9,*/*;q=0.8", formatJSONPB, nil)

		// supported and unsupported mix
		test("{json},foo/bar", formatJSONPB, nil)
		test("{json};q=0.1,foo/bar", formatJSONPB, nil)
		test("foo/bar;q=0.1,{json}", formatJSONPB, nil)

		// only unsupported types
		const err406 = "HTTP 406: Accept header: specified media types are not not supported"
		test(mtPRPC+"; boo=true", 0, err406)
		test(mtPRPC+"; encoding=blah", 0, err406)
		test("x", 0, err406)
		test("x,y", 0, err406)

		test("x//y", 0, "HTTP 400: Accept header: expected token after slash")
	})

	Convey("writeMessage", t, func() {
		msg := &HelloReply{Message: "Hi"}

		test := func(f format, body []byte, contentType string) {
			Convey(contentType, func() {
				res := httptest.NewRecorder()
				err := writeMessage(res, msg, f)
				So(err, ShouldBeNil)

				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Body.Bytes(), ShouldResembleV, body)
				So(res.Header().Get("Content-Type"), ShouldEqual, contentType)
			})
		}

		msgBytes, err := proto.Marshal(msg)
		So(err, ShouldBeNil)

		test(formatBinary, msgBytes, mtPRPCBinary)
		test(formatJSONPB, []byte("{\n\t\"message\": \"Hi\"\n}\n"), mtPRPCJSNOPB)
		test(formatText, []byte("message: \"Hi\"\n"), mtPRPCText)
	})

	Convey("writeError", t, func() {
		test := func(err error, status int, body string, logMsgs ...memlogger.LogEntry) {
			Convey(err.Error(), func() {
				c := context.Background()
				c = memlogger.Use(c)
				log := logging.Get(c).(*memlogger.MemLogger)

				rec := httptest.NewRecorder()
				writeError(c, rec, err)
				So(rec.Code, ShouldEqual, status)
				So(rec.Body.String(), ShouldEqual, body)

				actualMsgs := log.Messages()
				for i := range actualMsgs {
					actualMsgs[i].CallDepth = 0
				}
				So(actualMsgs, ShouldResembleV, logMsgs)
			})
		}

		test(Errorf(http.StatusNotFound, "not found"), http.StatusNotFound, "not found\n")
		test(grpc.Errorf(codes.NotFound, "not found"), http.StatusNotFound, "not found\n")
		test(
			fmt.Errorf("unhandled"),
			http.StatusInternalServerError,
			"Internal server error\n",
			memlogger.LogEntry{Level: logging.Error, Msg: "HTTP 500: unhandled"},
		)
	})
}
