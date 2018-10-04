// Copyright 2015 The LUCI Authors.
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
	"flag"
	"os"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/tools/internal/apigen"
)

func main() {
	a := apigen.Application{}
	lc := log.Config{
		Level: log.Warning,
	}

	fs := flag.CommandLine
	a.AddToFlagSet(fs)
	lc.AddFlags(fs)
	fs.Parse(os.Args[1:])

	ctx := context.Background()
	ctx = lc.Set(gologger.StdConfig.Use(ctx))

	if err := a.Run(ctx); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(ctx, "An error occurred during execution.")
		os.Exit(1)
	}
}
