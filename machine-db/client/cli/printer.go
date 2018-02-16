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

package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// printer writes rows of tab-separated columns.
type printer struct {
	*tabwriter.Writer
}

// Row prints one row of tab-separated columns.
func (p *printer) Row(args ...interface{}) {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprint(p, "\t")
		}
		switch arg.(type) {
		default:
			fmt.Fprint(p, arg)
		}
	}
	fmt.Fprint(p, "\n")
}

// newPrinter returns a printer.
func newPrinter(w io.Writer) *printer {
	return &printer{
		tabwriter.NewWriter(w, 0, 8, 0, '\t', 0),
	}
}

// newStdoutPrinter returns a printer which writes to os.Stdout.
func newStdoutPrinter() *printer {
	return newPrinter(os.Stdout)
}
