// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package streamserver

import (
	"net"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamclient"

	"github.com/Microsoft/go-winio"
)

// maxWindowsNamedPipeLength is the maximum length of a Windows named pipe.
const maxWindowsNamedPipeLength = 256

// NewNamedPipeServer instantiates a new Windows named pipe server instance.
func NewNamedPipeServer(ctx context.Context, name string) (StreamServer, error) {
	switch l := len(name); {
	case l == 0:
		return nil, errors.New("cannot have empty name")
	case l > maxWindowsNamedPipeLength:
		return nil, errors.Reason("name exceeds maximum length %(max)d").
			D("name", name).
			D("max", maxWindowsNamedPipeLength).
			Err()
	}

	ctx = log.SetField(ctx, "name", name)
	return &listenerStreamServer{
		Context: ctx,
		gen: func() (net.Listener, string, error) {
			address := "net.pipe:" + name
			pipePath := streamclient.LocalNamedPipePath(name)
			log.Fields{
				"addr":     address,
				"pipePath": pipePath,
			}.Debugf(ctx, "Creating Windows server socket Listener.")

			l, err := winio.ListenPipe(pipePath, nil)
			if err != nil {
				return nil, "", errors.Annotate(err).Reason("failed to listen on named pipe").Err()
			}
			return l, address, nil
		},
	}, nil
}
