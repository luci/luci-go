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

package lucicfg

import (
	"fmt"

	"go.starlark.net/starlark"
)

const (
	// Version is the version of lucicfg tool.
	//
	// It ends up in CLI output and in User-Agent headers.
	Version = "1.0.0"

	// UserAgent is used for User-Agent header in HTTP requests from lucicfg.
	UserAgent = "lucicfg v" + Version
)

func init() {
	// See //internal/meta.star.
	declNative("version", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		var major, minor, rev int
		_, err := fmt.Sscanf(Version, "%d.%d.%d", &major, &minor, &rev)
		if err != nil {
			panic(err)
		}
		return starlark.Tuple{
			starlark.MakeInt(major),
			starlark.MakeInt(minor),
			starlark.MakeInt(rev),
		}, nil
	})
}
