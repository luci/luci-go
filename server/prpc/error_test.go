// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	. "github.com/smartystreets/goconvey/convey"
)

func TestError(t *testing.T) {
	t.Parallel()

	Convey("ErrorCode", t, func() {
		status := ErrorStatus(nil)
		So(status, ShouldEqual, http.StatusOK)

		status = ErrorStatus(grpc.Errorf(codes.NotFound, "Not found"))
		So(status, ShouldEqual, http.StatusNotFound)

		status = ErrorStatus(withStatus(fmt.Errorf(""), http.StatusBadRequest))
		So(status, ShouldEqual, http.StatusBadRequest)

		status = ErrorStatus(fmt.Errorf("unhandled"))
		So(status, ShouldEqual, http.StatusInternalServerError)
	})

	Convey("ErrorDesc", t, func() {
		So(ErrorDesc(nil), ShouldEqual, "")

		err := Errorf(http.StatusNotFound, "not found")
		So(ErrorDesc(err), ShouldEqual, "not found")

		err = grpc.Errorf(codes.NotFound, "not found")
		So(ErrorDesc(err), ShouldEqual, "not found")

		err = withStatus(grpc.Errorf(codes.NotFound, "not found"), http.StatusNotFound)
		So(ErrorDesc(err), ShouldEqual, "not found")

		err = fmt.Errorf("not found")
		So(ErrorDesc(err), ShouldEqual, "not found")
	})

	Convey("withStatus", t, func() {
		Convey("withStatus(nil, *) returns nil", func() {
			So(withStatus(nil, http.StatusBadRequest), ShouldBeNil)
		})
	})
}
