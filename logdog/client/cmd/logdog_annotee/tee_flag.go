// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
