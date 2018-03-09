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
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"go.chromium.org/luci/machine-db/api/common/v1"
)

// tablePrinter defines an interface for printing tabular data.
type tablePrinter interface {
	Row(args ...interface{})
	Flush() error
}

// humanReadable writes rows of space-separated columns.
// To ensure proper alignment of columns Flush should be called once after the last Row.
type humanReadable struct {
	*tabwriter.Writer
}

// Row prints one row of space-separated columns.
// Aligns mixed-length columns regardless of how many spaces it takes.
func (p *humanReadable) Row(args ...interface{}) {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprint(p, "\t")
		}
		switch a := arg.(type) {
		case common.State:
			fmt.Fprint(p, a.Name())
		default:
			fmt.Fprint(p, arg)
		}
	}
	fmt.Fprint(p, "\n")
}

// newHumanReadable returns a humanReadable tablePrinter.
func newHumanReadable(w io.Writer) *humanReadable {
	return &humanReadable{
		tabwriter.NewWriter(w, 0, 1, 4, ' ', 0),
	}
}

// machineReadable writes rows of tab-separated columns.
type machineReadable struct {
	*csv.Writer
}

// Flushes the underlying buffer.
func (p *machineReadable) Flush() error {
	p.Writer.Flush()
	return p.Error()
}

// Row prints one row of tab-separated columns.
// Does not attempt to align mixed-length columns.
func (p *machineReadable) Row(args ...interface{}) {
	row := make([]string, len(args))
	for i, arg := range args {
		switch a := arg.(type) {
		case common.State:
			row[i] = a.Name()
		default:
			row[i] = fmt.Sprint(arg)
		}
	}
	p.Write(row)
}

func newMachineReadable(w io.Writer) *machineReadable {
	m := &machineReadable{csv.NewWriter(w)}
	m.Comma = '\t'
	return m
}

// newStdoutPrinter returns a tablePrinter which writes to os.Stdout.
func newStdoutPrinter(forMachines bool) tablePrinter {
	if forMachines {
		return newMachineReadable(os.Stdout)
	}
	return newHumanReadable(os.Stdout)
}
