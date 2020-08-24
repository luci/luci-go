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

package exe

import (
	"compress/zlib"
	"context"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

type config struct {
	ctx context.Context

	zlibLevel int
	lim       rate.Limit

	ldClient *streamclient.Client
}

// RunOption is a type that allows you to modify the behavior of Run.
//
// See Opt* methods for available RunOptions.
type RunOption func(*config)

// OptZlibCompression returns a RunOption; If unspecified, no compression will be
// applied to the outgoing Build.proto stream. Otherwise zlib compression at
// `level` will be used.
//
// level is capped between NoCompression and BestCompression.
//
// If level is NoCompression, it's the same as not specifying this option (i.e.
// no zlib wrapper will be used at all).
func OptZlibCompression(level int) RunOption {
	if level <= zlib.NoCompression {
		return nil
	}
	if level > zlib.BestCompression {
		level = zlib.BestCompression
	}
	return func(c *config) {
		c.zlibLevel = level
	}
}

// OptBuildRateLimit allows you to adjust the maximum rate of build updates
// which will be sent.
//
// If unspecified, Run defaults to a maximum of 1qps.
//
// If <= 0, no build stream will be opened, and build updates will not be sent.
func OptBuildRateLimit(lim rate.Limit) RunOption {
	if lim < 0 {
		lim = 0
	}

	return func(c *config) {
		c.lim = lim
	}
}

// OptLogdogClient allows you to explicitly provide a logdog client to Run.
//
// By default, Run will will bootstrap a logdog client from the current
// environment (i.e. using `bootstrap.Get()`).
func OptLogdogClient(ldClient *streamclient.Client) RunOption {
	if ldClient == nil {
		return nil
	}
	return func(c *config) {
		c.ldClient = ldClient
	}
}

// OptTweakContext allows you to arbitrarially tweak the context before Run
// executes your MainFn.
//
// If this function installs a 'go.chromium.org/luci/common/logging' compatible
// logger, then Run will not set up any additional logging. By default Run will
// install a logger which outputs to stderr.
func OptTweakContext(cb func(context.Context) context.Context) RunOption {
	if cb == nil {
		return nil
	}
	return func(c *config) {
		c.ctx = cb(c.ctx)
	}
}
