// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"flag"

	"golang.org/x/net/context"
)

// Config is a logging configuration structure.
type Config struct {
	Level Level
}

// AddFlags adds common flags to a supplied FlagSet.
func (c *Config) AddFlags(fs *flag.FlagSet) {
	fs.Var(&c.Level, "log-level",
		"The logging level. Valid options are: debug, info, warning, error.")
}

// Set installs a logger that wraps the currently-installed Logger and sets
// the level via the command-line level flag.
func (c *Config) Set(ctx context.Context) context.Context {
	return SetLevel(ctx, c.Level)
}
