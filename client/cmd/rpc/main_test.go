// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/url"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMain(t *testing.T) {
	t.Parallel()

	Convey("parseServer", t, func() {
		test := func(host string, expectedErr interface{}, expectedURL *url.URL) {
			Convey(host, func() {
				u, err := parseServer(host)
				So(err, ShouldErrLike, expectedErr)
				So(u, ShouldResembleV, expectedURL)
			})
		}

		test("", "unspecified", nil)

		// Localhost is http, everything else is https.
		test("localhost", nil, &url.URL{
			Scheme: "http",
			Host:   "localhost",
		})
		test("localhost:8080", nil, &url.URL{
			Scheme: "http",
			Host:   "localhost:8080",
		})
		test("127.0.0.1", nil, &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1",
		})
		test("127.0.0.1:8080", nil, &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:8080",
		})
		test("example.com", nil, &url.URL{
			Scheme: "https",
			Host:   "example.com",
		})
		test("example.com:8080", nil, &url.URL{
			Scheme: "https",
			Host:   "example.com:8080",
		})

		// Short syntax for localhost.
		test(":", nil, &url.URL{
			Scheme: "http",
			Host:   "localhost:",
		})
		test(":8080", nil, &url.URL{
			Scheme: "http",
			Host:   "localhost:8080",
		})

		// No explicit scheme.
		test("http://example.com", "must not have scheme", nil)
		test("https://example.com", "must not have scheme", nil)
		test("ftp://example.com", "must not have scheme", nil)

		// Extra URL components.
		test("example.com/a", "must not have query, path or fragment", nil)
		test("example.com?a", "must not have query, path or fragment", nil)
		test("example.com#a", "must not have query, path or fragment", nil)
	})
}
