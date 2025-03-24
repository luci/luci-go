// Copyright 2019 The LUCI Authors.
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
	"os"
	"strings"

	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/resultdb/cli"
)

func main() {
	os.Stderr.WriteString("Use of LUCI is subject to the Google [Terms of Service](https://policies.google.com/terms) and [Privacy Policy](https://policies.google.com/privacy)\n\n")
	p := cli.Params{
		Auth:                chromeinfra.DefaultAuthOptions(),
		DefaultResultDBHost: chromeinfra.ResultDBHost,
	}
	// TODO(ddoman): remove this hack after updating all recipes to use the new delimiter.
	for i := 1; i < len(os.Args)-1; i++ {
		if os.Args[i] == "-var" {
			os.Args[i+1] = strings.Replace(os.Args[i+1], "=", ":", 1)
			i += 1
		} else if os.Args[i] == "--" {
			break
		}
	}
	os.Exit(cli.Main(p, os.Args[1:]))
}
