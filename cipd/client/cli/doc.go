// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package cli implements command line interface for CIPD client.
//
// It's main exported function is GetApplication(...) that takes a bundle with
// default parameters and returns a subcommands.Application configured with this
// defaults.
//
// There's also Main(...) that does some additional arguments manipulation. It
// can be used to build a copy of 'cipd' tool with some defaults tweaked.
package cli
