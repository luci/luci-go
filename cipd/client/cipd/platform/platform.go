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
	"runtime/debug"
)

var (
	currentArchitecture = ""
	currentOS           = ""
	currentPlat         = ""
)

func init() {
	var vars []debug.BuildSetting
	if info, ok := debug.ReadBuildInfo(); ok {
		vars = info.Settings
	}

	getEnv := func(key string) string {
		switch key {
		case "GOOS":
			return runtime.GOOS
		case "GOARCH":
			return runtime.GOARCH
		}
		for _, kv := range vars {
			if kv.Key == key {
				return kv.Value
			}
		}
		return ""
	}

	currentOS = getEnv("GOOS")
	if currentOS == "darwin" {
		currentOS = "mac"
	}

	currentArchitecture = getEnv("GOARCH")
	if currentArchitecture == "arm" {
		if getEnv("GOARM") == "7" {
			currentArchitecture = "armv7l"
		} else {
			currentArchitecture = "armv6l"
		}
	}

	currentPlat = fmt.Sprintf("%s-%s", currentOS, currentArchitecture)
}

// CurrentArchitecture returns the current CIPD architecture that the current go
// binary conforms to.
//
// See the package doc for the list of possible values.
func CurrentArchitecture() string {
	return currentArchitecture
}

// CurrentOS returns the current CIPD os that the current go binary conforms to.
//
// See the package doc for the list of possible values.
func CurrentOS() string {
	return currentOS
}

// CurrentPlatform returns the current CIPD platform which is just
// "${os}-${arch}" string.
func CurrentPlatform() string {
	return currentPlat
}
