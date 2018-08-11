// Copyright 2018 The LUCI Authors.
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

// Package platform contains definition of what ${os} and ${arch} mean for
// the current platform.
package platform

import (
	"fmt"
	"runtime"
)

var (
	currentArchitecture = ""
	currentOS           = ""
	currentPlat         = ""
)

func init() {
	// TODO(iannucci): rationalize these to just be exactly GOOS and GOARCH.
	currentArchitecture = runtime.GOARCH
	if currentArchitecture == "arm" {
		currentArchitecture = "armv6l"
	}

	currentOS = runtime.GOOS
	if currentOS == "darwin" {
		currentOS = "mac"
	}

	currentPlat = fmt.Sprintf("%s-%s", currentOS, currentArchitecture)
}

// CurrentArchitecture returns the current cipd-style architecture that the
// current go binary conforms to.
//
// Possible values:
//   - "armv6l" (if GOARCH=arm)
//   - other GOARCH values
func CurrentArchitecture() string {
	return currentArchitecture
}

// CurrentOS returns the current cipd-style os that the current go binary
// conforms to.
//
// Possible values:
//   - "mac" (if GOOS=darwin)
//   - other GOOS values
func CurrentOS() string {
	return currentOS
}

// CurrentPlatform returns the current cipd-style platform which is just
// "${os}-${arch}" string.
func CurrentPlatform() string {
	return currentPlat
}
