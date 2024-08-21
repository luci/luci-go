// Copyright 2024 The LUCI Authors.
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

// Rewriter will take one or more existing Go test files on the command-line and
// rewrite "goconvey" tests in them to use
// go.chromium.org/luci/common/testing/ftt.
//
// This makes the following changes:
//   - Converts all Convey function calls to ftt.Run calls.
//     This will also rewrite the callback from `func() {...}` to
//     `func(t *ftt.Test) {...}`. If the test already had an explicit
//     context variable (e.g. `func(c C)`), this will preserve that name.
//   - Removes dot-imported convey.
//   - Adds imports for go.chromium.org/luci/testing/common/truth/assert and
//     go.chromium.org/luci/testing/common/truth/should.
//   - Rewrites So and SoMsg calls.
//
// Note that 'fancy' uses of Convey/So will likely be missed by this tool, and
// will require hand editing... however this does complete successfully for all
// code in luci-go, infra and infra_internal at the time of writing.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt/rewriter/rewriteimpl"
)

var dryrun = flag.Bool("dry", false, "if set, only does a dry run (doesn't overwrite any .go files)")
var stopwarn = flag.Bool("x", false, "if set, stop on first warning")

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		lines := []string{
			"This tool makes the following changes:",
			"  - Converts all Convey function calls to ftt.Run calls.",
			"    This will also rewrite the callback from `func() {...}` to",
			"    `func(t *ftt.Test) {...}`. If the test already had an explicit",
			"    context variable (e.g. `func(c C)`), this will preserve that name.",
			"  - Removes dot-imported convey.",
			"  - Adds imports for go.chromium.org/luci/testing/common/truth/assert and",
			"    go.chromium.org/luci/testing/common/truth/should.",
			"  - Rewrites So assertion calls.",
			"",
			"Note that 'fancy' uses of Convey/So will likely be missed by this tool, and",
			"will require hand editing... however this does complete successfully for all",
			"code in luci-go, infra and infra_internal at the time of writing.",
		}
		for _, line := range lines {
			fmt.Fprintln(flag.CommandLine.Output(), line)
		}
		flag.PrintDefaults()
	}
	flag.Parse()
}

func main() {
	adaptedAssertions := stringset.New(0)
	for _, fname := range flag.Args() {
		dec, file, err := rewriteimpl.ParseOne(fname)
		if err != nil {
			log.Printf("ERROR: %s: %s\n", fname, err)
		}

		newFile, rewrote, warn, err := rewriteimpl.Rewrite(dec, file, adaptedAssertions)
		if err != nil {
			log.Fatal(fmt.Errorf("error encountered while rewriting, doing nothing: %s", err))
		}
		if !rewrote {
			log.Printf("SKIP: %s: no convey import\n", fname)
			continue
		}

		log.Println("REWRITE:", fname)
		if !*dryrun {
			ofile, err := os.Create(fname)
			if err != nil {
				log.Fatal(err)
			}
			defer ofile.Close()
			if err := format.Node(ofile, dec.Fset, newFile); err != nil {
				log.Fatal(err)
			}
		}

		if *stopwarn && warn {
			log.Printf("STOP: %s: Stopping on first warning due to -x.\n", fname)
			return
		}
	}
}
