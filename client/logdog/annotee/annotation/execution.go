// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package annotation

import (
	"os"
	"strings"
)

// Execution describes the high-level execution metadata.
type Execution struct {
	Name    string
	Command []string
	Dir     string
	Env     map[string]string
}

// ProbeExecution loads Execution parameters by probing the current runtime
// environment.
func ProbeExecution(argv []string) *Execution {
	cwd, _ := os.Getwd()
	return probeExecutionImpl(argv, os.Environ(), cwd)
}

func probeExecutionImpl(argv []string, env []string, cwd string) *Execution {
	e := &Execution{
		Dir: cwd,
	}
	e.Command = make([]string, len(argv))
	copy(e.Command, argv)

	e.Env = make(map[string]string, len(env))
	for _, v := range env {
		p := strings.SplitN(v, "=", 2)
		switch len(p) {
		case 1:
			e.Env[p[0]] = ""
		case 2:
			e.Env[p[0]] = p[1]
		}
	}

	return e
}
