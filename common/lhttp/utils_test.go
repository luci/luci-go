// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lhttp

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckURL(t *testing.T) {
	Convey(`Verifies that CheckURL properly checks a URL.`, t, func() {
		data := []struct {
			in       string
			expected string
			err      error
		}{
			{"foo", "https://foo", nil},
			{"https://foo", "https://foo", nil},
			{"http://foo.example.com", "http://foo.example.com", nil},
			{"http://foo.appspot.com", "", errors.New("only https:// scheme is accepted for appspot hosts, it can be omitted")},
		}
		for _, line := range data {
			out, err := CheckURL(line.in)
			So(out, ShouldResemble, line.expected)
			So(err, ShouldResemble, line.err)
		}
	})
}
