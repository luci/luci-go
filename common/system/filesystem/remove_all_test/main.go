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

package main

import (
	"context"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/filesystem"
)

func main() {
	c := context.Background()
	gologger.StdConfig.Use(c)
	if err := filesystem.RemoveAll(os.Args[1]); err != nil {
		err = errors.Annotate(err, "failed").Err()
		errors.Log(c, err)
		os.Exit(2)
	}
	os.Exit(0)
}
