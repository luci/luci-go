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

package logging

import (
	"context"
	"flag"
)

// Config is a logging configuration structure.
type Config struct {
	Level Level
}

// AddFlags adds common flags to a supplied FlagSet.
func (c *Config) AddFlags(fs *flag.FlagSet) {
	c.AddFlagsPrefix(fs, "")
}

// AddFlagsPrefix adds common flags to a supplied FlagSet with a prefix.
//
// A string prefix must be supplied which will be prepended to
// each added flag verbatim.
func (c *Config) AddFlagsPrefix(fs *flag.FlagSet, prefix string) {
	fs.Var(&c.Level, prefix+"log-level",
		"The logging level. Valid options are: debug, info, warning, error.")
}

// Set returns a new context configured to use logging level passed via the
// command-line level flag.
func (c *Config) Set(ctx context.Context) context.Context {
	return SetLevel(ctx, c.Level)
}
