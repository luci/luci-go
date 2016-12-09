// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"
)

const (
	teeFlagAnnotations = "annotations"
	teeFlagText        = "text"
)

var teeFlagOptions = strings.Join([]string{teeFlagAnnotations, teeFlagText}, ", ")

type teeFlag struct {
	annotations bool
	text        bool
}

func (f *teeFlag) String() string {
	parts := make([]string, 0, 2)
	if f.annotations {
		parts = append(parts, teeFlagAnnotations)
	}
	if f.text {
		parts = append(parts, teeFlagText)
	}
	return strings.Join(parts, ",")
}

func (f *teeFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		switch part {
		case teeFlagAnnotations:
			f.annotations = true
		case teeFlagText:
			f.text = true
		default:
			return fmt.Errorf("unknown tee type: %q", v)
		}
	}
	return nil
}

func (f *teeFlag) enabled() bool {
	return f.annotations || f.text
}
