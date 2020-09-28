// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
	"flag"
	"fmt"
	"os"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
)

// Main evaluates an RTS algorithm for Chromium, prints results and exits the
// process.
func Main(ctx context.Context, backend Backend, algo Algorithm) {
	ctx, cancel := context.WithCancel(ctx)
	defer signals.HandleInterrupt(cancel)

	ev := &Eval{
		Algorithm: algo,
		Backend:   backend,
	}
	if err := ev.RegisterFlags(flag.CommandLine); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	flag.Parse()

	var logCfg = gologger.LoggerConfig{
		Format: `%{message}`,
		Out:    os.Stderr,
	}
	ctx = logCfg.Use(ctx)

	res, err := ev.Run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	res.Print(os.Stdout)
	os.Exit(0)
}
