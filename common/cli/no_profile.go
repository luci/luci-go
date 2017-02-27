// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !include_profiler

package cli

import (
	"github.com/maruel/subcommands"
)

type profilingExt struct{}

func (p profilingExt) addProfiling(cmds []*subcommands.Command) {}
