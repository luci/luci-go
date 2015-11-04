// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testResponseWriter struct {
	status int
	data   bytes.Buffer
	err    error
	header http.Header
}

func (w *testResponseWriter) Header() http.Header {
	return w.header
}

func (w *testResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *testResponseWriter) Write(d []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.data.Write(d)
}

func TestErrorResponseWriter(t *testing.T) {
	Convey(`An errorResponseWriter bound to a testing ResponseWriter`, t, func() {
		rw := &testResponseWriter{}
		erw := &errorResponseWriter{
			ResponseWriter: rw,
		}

		Convey(`When no error is encountered, data is written directly.`, func() {
			erw.WriteHeader(http.StatusOK)
			c, err := erw.Write([]byte("test"))
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 4)

			So(rw.status, ShouldEqual, http.StatusOK)
			So(rw.data.String(), ShouldResemble, "test")
			So(erw.forwardError(), ShouldBeFalse)
		})

		Convey(`When an error is encountered, status is translated and data is buffered.`, func() {
			erw.WriteHeader(http.StatusNotAcceptable) // 406 => 404
			c, err := erw.Write([]byte("{}"))
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 2)

			So(rw.status, ShouldEqual, http.StatusNotFound) // (404)
			So(rw.data.String(), ShouldEqual, "")
		})

		Convey(`All HTTP error codes translate to error codes.`, func() {
			for _, r := range []struct {
				lb int
				ub int
			}{
				// 400s
				{http.StatusBadRequest, http.StatusTeapot},

				// 500s
				{http.StatusInternalServerError, http.StatusHTTPVersionNotSupported},

				// Not real error code.
				{1024, 1024},
			} {
				for i := r.lb; i <= r.ub; i++ {
					r, inst := erw.translateReason(i)
					So(r, ShouldBeGreaterThanOrEqualTo, http.StatusBadRequest)
					So(inst, ShouldNotBeNil)

					So(inst.Domain, ShouldEqual, "global")
					So(inst.Reason, ShouldNotEqual, "")
				}
			}
		})

		Convey(`An error set by the frontend is forwarded.`, func() {
			erw.setError(errors.New("test error"))
			So(erw.forwardError(), ShouldBeTrue)

			So(rw.status, ShouldEqual, http.StatusServiceUnavailable)
			So(rw.data.String(), ShouldEqual, strings.Join([]string{
				`{`,
				` "error": {`,
				`  "code": 503,`,
				`  "errors": [`,
				`   {`,
				`    "domain": "global",`,
				`    "message": "test error",`,
				`    "reason": "backendError"`,
				`   }`,
				`  ],`,
				`  "message": "unspecified error"`,
				` }`,
				`}`,
			}, "\n"))
		})

		Convey(`A valid error set by the backend is forwarded.`, func() {
			erw.WriteHeader(http.StatusNotFound)
			_, err := erw.Write([]byte(`{"state": "APPLICATION_ERROR", "error_message": "test error"}`))
			So(err, ShouldBeNil)
			So(erw.forwardError(), ShouldBeTrue)

			So(rw.status, ShouldEqual, http.StatusNotFound)
			So(rw.data.String(), ShouldEqual, strings.Join([]string{
				`{`,
				` "error": {`,
				`  "code": 404,`,
				`  "errors": [`,
				`   {`,
				`    "domain": "global",`,
				`    "message": "test error",`,
				`    "reason": "notFound"`,
				`   }`,
				`  ],`,
				`  "message": "test error"`,
				` }`,
				`}`,
			}, "\n"))
		})

		Convey(`An invalid error set by the backend is forwarded.`, func() {
			erw.WriteHeader(http.StatusNotFound)
			_, err := erw.Write([]byte(`DEFINITELY NOT JSON`))
			So(err, ShouldBeNil)
			So(erw.forwardError(), ShouldBeTrue)

			So(rw.status, ShouldEqual, http.StatusNotFound)
			So(rw.data.String(), ShouldEqual, strings.Join([]string{
				`{`,
				` "error": {`,
				`  "code": 404,`,
				`  "errors": [`,
				`   {`,
				`    "domain": "global",`,
				`    "message": "Failed to decode error JSON (invalid character 'D' looking for beginning of value): ",`,
				`    "reason": "notFound"`,
				`   }`,
				`  ],`,
				`  "message": "unspecified error"`,
				` }`,
				`}`,
			}, "\n"))
		})
	})
}
