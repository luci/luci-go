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

// +build darwin dragonfly freebsd linux netbsd openbsd

package main

import (
	"os"
	"syscall"

	"github.com/luci/luci-go/logdog/client/butler/streamserver"

	"golang.org/x/net/context"
)

var platformStreamServerExamples = []string{
	"unix:/var/run/butler.sock",
}

// interruptSignals is the set of signals to handle gracefully (e.g., flush,
// shutdown).
var interruptSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
}

func resolvePlatform(ctx context.Context, typ, spec string) (streamserver.StreamServer, error) {
	switch typ {
	case "unix":
		return streamserver.NewUNIXDomainSocketServer(ctx, spec)

	default:
		// Not a known platform type.
		return nil, nil
	}
}
