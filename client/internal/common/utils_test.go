// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"testing"

	"github.com/maruel/ut"
)

func TestURLToHTTPS(t *testing.T) {
	data := []struct {
		in       string
		expected string
		err      error
	}{
		{"foo", "https://foo", nil},
		{"https://foo", "https://foo", nil},
		{"http://foo", "", errors.New("Only https:// scheme is accepted. It can be omitted.")},
	}
	for i, line := range data {
		out, err := URLToHTTPS(line.in)
		ut.AssertEqualIndex(t, i, line.expected, out)
		ut.AssertEqualIndex(t, i, line.err, err)
	}
}
