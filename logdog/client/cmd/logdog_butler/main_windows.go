// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/luci/luci-go/logdog/client/butler/streamserver"
	"golang.org/x/net/context"
)

var platformStreamServerExamples = []string{
	// This expands to: //localhost/pipe/<name>
	"net.pipe:logdog-butler",
}

// interruptSignals is the set of signals to handle gracefully (e.g., flush,
// shutdown).
var interruptSignals = []os.Signal{
	os.Interrupt,
}

func resolvePlatform(ctx context.Context, typ, spec string) (streamserver.StreamServer, error) {
	switch typ {
	case "net.pipe":
		return streamserver.NewNamedPipeServer(ctx, spec)

	default:
		// Not a known platform type.
		return nil, nil
	}
}
