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
	Level  Level
	Filter Filter
}

// AddFlags adds common flags to a supplied FlagSet.
func (c *Config) AddFlags(fs *flag.FlagSet) {
	fs.Var(&c.Level, "log_level",
		"The logging level. Valid options are: debug, info, warning, error.")
	fs.Var(&c.Filter, "log_filter",
		"Log filter keywords. Can be specified multiple times.")
}

// Set installs a filter-aware logger that wraps the currently-installed Logger
// and selectively discards messages based on the logging configuration.
func (c *Config) Set(ctx context.Context) context.Context {
	filterFunc := c.Filter.Get()
	baseFactory := GetFactory(ctx)

	return SetFactory(ctx, func(ctx context.Context) Logger {
		if value, ok := GetFields(ctx)[FilterOnKey]; ok {
			if !filterFunc(value) {
				return nil
			}
		}
		return baseFactory(ctx)
	})
}
