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

	"golang.org/x/time/rate"
)

type config struct {
	zlibLevel int
	lim       rate.Limit
}

// Option is a type that allows you to modify the behavior of Run.
//
// See With* methods for available Options.
type Option func(*config)

// WithZlibCompression returns an Option; If unspecified, no compression will be
// applied to the outgoing Build.proto stream. Otherwise zlib compression at
// `level` will be used.
//
// level is capped between NoCompression and BestCompression.
//
// If level is NoCompression, it's the same as not specifying this option (i.e.
// no zlib wrapper will be used at all).
func WithZlibCompression(level int) Option {
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

// BuildRateLimit allows you to adjust the maximum rate of build updates which
// will be sent.
//
// If unspecified, this defaults to a maximum of 1qps.
//
// If <= 0, no build stream will be opened, and build updates will not be sent.
func BuildRateLimit(lim rate.Limit) Option {
	if lim < 0 {
		lim = 0
	}

	return func(c *config) {
		c.lim = lim
	}
}
