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

package common

import (
	"context"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/runtime/tracer"
)

// Flags contains values parsed from command line arguments.
type Flags struct {
	Quiet     bool
	Verbose   bool
	TracePath string
}

// Init registers flags in a given flag set.
func (d *Flags) Init(f *flag.FlagSet) {
	f.BoolVar(&d.Quiet, "quiet", false, "Get less output")
	f.BoolVar(&d.Verbose, "verbose", false, "Get more output")
	f.StringVar(&d.TracePath, "trace", "", "Name of trace file to generate")
}

// Parse applies changes specified by command line flags.
func (d *Flags) Parse() error {
	if !d.Verbose {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	if d.Quiet && d.Verbose {
		return errors.New("can't use both -quiet and -verbose")
	}
	if d.TracePath != "" {
		p, err := filepath.Abs(d.TracePath)
		if err != nil {
			return err
		}
		d.TracePath = p
	}
	return nil
}

// StartTracing enables tracing and returns a closer that must be called on
// process termination.
func (d *Flags) StartTracing() (io.Closer, error) {
	if d.TracePath == "" {
		return &tracingState{}, nil
	}
	f, err := os.Create(d.TracePath)
	if err != nil {
		return &tracingState{}, err
	}
	if err = tracer.Start(f, 0); err != nil {
		_ = f.Close()
		return &tracingState{}, err
	}
	return &tracingState{f}, nil
}

// MakeLoggingContext makes a luci-go/common/logging compatible context using
// gologger onto the given writer.
//
// The default logging level will be Info, with Warning and Debug corresponding
// to quiet/verbose respectively.
func (d *Flags) MakeLoggingContext(out io.Writer) context.Context {
	ret := (&gologger.LoggerConfig{
		Out:    out,
		Format: gologger.PickStdFormat(out),
	}).Use(context.Background())
	if d.Quiet {
		ret = logging.SetLevel(ret, logging.Warning)
	} else if d.Verbose {
		ret = logging.SetLevel(ret, logging.Debug)
	} else {
		ret = logging.SetLevel(ret, logging.Info)
	}
	return ret
}

// Private stuff.

type tracingState struct {
	c io.Closer
}

func (t *tracingState) Close() error {
	if t.c != nil {
		tracer.Stop()
		err := t.c.Close()
		t.c = nil
		return err
	}
	return nil
}
