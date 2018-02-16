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

// Package cli contains the Machine Database command-line client.
package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// writer writes rows of tab-separated columns.
type writer struct {
	*tabwriter.Writer
}

// Row writes one row of tab-separated columns.
func (w *writer) Row(args ...interface{}) {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprintf(w, "\t")
		}
		switch arg.(type) {
		default:
			fmt.Fprintf(w, "%s", arg)
		}
	}
	fmt.Fprintf(w, "\n")
}

// newWriter returns a new writer.
func newWriter(w io.Writer) *writer {
	return &writer{
		tabwriter.NewWriter(w, 0, 8, 0, '\t', 0),
	}
}

// newStdoutWriter returns a new writer which writes to os.Stdout.
func newStdoutWriter() *writer {
	return newWriter(os.Stdout)
}
