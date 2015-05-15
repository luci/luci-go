// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/luci/luci-go/client/internal/lhttp"
	"github.com/luci/luci-go/client/internal/tracer"
	"github.com/maruel/subcommands"
)

type commonFlags struct {
	quiet     bool
	verbose   bool
	tracePath string
	traceFile io.Closer
}

func (c *commonFlags) Init(b *subcommands.CommandRunBase) {
	b.Flags.BoolVar(&c.quiet, "quiet", false, "Get less output")
	b.Flags.BoolVar(&c.verbose, "verbose", false, "Get more output")
	b.Flags.StringVar(&c.tracePath, "trace", "", "Name of trace file")
}

func (c *commonFlags) Parse() error {
	if !c.verbose {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	if c.quiet && c.verbose {
		return errors.New("can't use both -quiet and -verbose")
	}
	if c.tracePath != "" {
		f, err := os.Create(c.tracePath)
		if err != nil {
			return err
		}
		if err := tracer.Start(f, 0); err != nil {
			_ = f.Close()
			return err
		}
		c.traceFile = f
	}
	return nil
}

func (c *commonFlags) Close() error {
	if c.traceFile == nil {
		return errors.New("was already closed")
	}
	tracer.Stop()
	return c.traceFile.Close()
}

type commonServerFlags struct {
	serverURL   string
	namespace   string
	compression string
	hashing     string
}

func (c *commonServerFlags) Init(b *subcommands.CommandRunBase) {
	b.Flags.StringVar(&c.serverURL, "isolate-server", "", "Isolate server to use")
	b.Flags.StringVar(&c.serverURL, "I", "", "Alias for -isolate-server")
	b.Flags.StringVar(&c.namespace, "namespace", "default-gzip", "")
}

func (c *commonServerFlags) Parse() error {
	if c.serverURL == "" {
		return errors.New("-isolate-server must be specified")
	}
	if s, err := lhttp.URLToHTTPS(c.serverURL); err != nil {
		return err
	} else {
		c.serverURL = s
	}
	if c.namespace == "" {
		return errors.New("-namespace must be specified.")
	}
	return nil
}
