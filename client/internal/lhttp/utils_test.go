// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lhttp

import (
	"errors"
	"testing"

	"github.com/maruel/ut"
)

func TestCheckURL(t *testing.T) {
	const onlyHttpsErr = "Only https:// scheme is accepted for appspot hosts. " +
		"It can be omitted."

	data := []struct {
		in       string
		expected string
		err      error
	}{
		{"foo", "https://foo", nil},
		{"https://foo", "https://foo", nil},
		{"http://foo.example.com", "http://foo.example.com", nil},
		{"http://foo.appspot.com", "", errors.New(onlyHttpsErr)},
	}
	for i, line := range data {
		out, err := CheckURL(line.in)
		ut.AssertEqualIndex(t, i, line.expected, out)
		ut.AssertEqualIndex(t, i, line.err, err)
	}
}
