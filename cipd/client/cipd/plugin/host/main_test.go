// Copyright 2021 The LUCI Authors.
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

package host

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
)

var pluginMains = map[string]func(context.Context, string) error{}

func registerPluginMain(name string, cb func(context.Context, string) error) {
	pluginMains[name] = cb
}

func TestMain(m *testing.M) {
	isPluginProc := len(os.Args) >= 2 && strings.HasPrefix(os.Args[1], "PLUGIN_")
	if isPluginProc {
		pluginMain := pluginMains[os.Args[1]]
		if pluginMain == nil {
			fmt.Fprintf(os.Stderr, "Unrecognized plugin entry point %q", os.Args[1])
			os.Exit(1)
		}
		ctx := context.Background()
		if err := pluginMain(ctx, os.Args[2]); err != nil {
			errors.Log(ctx, err)
			os.Exit(1)
		} else {
			os.Exit(0)
		}
	} else {
		os.Exit(m.Run())
	}
}
