// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"runtime"
	"strings"

	"go.chromium.org/luci/common/system/environ"
)

// Platforms is the cross-compile platform tuple.
type Platforms struct {
	Build  Platform
	Host   Platform
	Target Platform
}

// Platform defines the environment outside the build system, which can't be
// identified in the environment variables or commands, but will affect the
// outputs. Platform must includes operating system and architecture, but
// also support any attributes that may affect the results (e.g. glibcABI).
type Platform struct {
	// (ab)use the Env implementation since it's almost same as the key-value
	// platform attributes.
	// TODO(fancl): properly implement it.
	attributes environ.Env
}

// NewPlatform returns a Platform with os and arch set.
func NewPlatform(os, arch string) Platform {
	p := Platform{attributes: environ.New(nil)}
	p.Set("os", os)
	p.Set("arch", arch)
	return p
}

// Get will return the value of the attribute
func (p Platform) Get(key string) string {
	return p.attributes.Get(key)
}

// Set will set the attribute.
func (p Platform) Set(key, val string) {
	if strings.Contains(key, ",") || strings.Contains(val, ",") {
		panic("comma is not allowd in the platform attributes")
	}
	p.attributes.Set(key, val)
}

func (p Platform) OS() string {
	return p.attributes.Get("os")
}

func (p Platform) Arch() string {
	return p.attributes.Get("arch")
}

func (p Platform) String() string {
	attrs := p.attributes.Sorted()
	return strings.Join(attrs, ",")
}

// CurrentPlatform returns a Platform with os and arch set to runtime.GOOS and
// runtime.GOARCH.
func CurrentPlatform() Platform {
	return NewPlatform(runtime.GOOS, runtime.GOARCH)
}

// PlatformFromCIPD converts cipd platform to Platform.
func PlatformFromCIPD(cipdPlat string) Platform {
	ss := strings.SplitN(cipdPlat, "-", 2)
	os, arch := ss[0], ss[1]
	if os == "mac" {
		os = "darwin"
	}

	// From https://source.chromium.org/chromium/infra/infra/+/main:recipes/recipe_modules/support_3pp/create.py;l=61;drc=3e18f7231543c975ecb0a620bb466471b0ea2c2b
	if arch == "armv6l" {
		arch = "arm"
	}
	return NewPlatform(os, arch)
}
