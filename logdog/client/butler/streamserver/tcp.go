// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package streamserver

import (
	"fmt"
	"net"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

// tcpStreamServer is a StreamServer implementation that binds to a TCP/IP
// socket.
type tcpStreamServer struct {
	net  string
	addr *net.TCPAddr
}

// NewTCP4Server creates a new TCP/IP4 stream server.
//
// spec is a string of the form [addr][:port].
//
// If addr is not empty, it will be an IPv4 network address to bind to. If it
// is empty, the StreamServer will bind exclusively to localhost.
//
// port must be a valid, available port. It may be omitted or <=0, in which case
// an ephemeral port will be chosen by the system. Note that, in this case, the
// caller cannot prescribe the port in advance, and must discover it via
// exported stream server parameters (externally) or by calling Address
// (internally).
func NewTCP4Server(ctx context.Context, spec string) (StreamServer, error) {
	return newTCPServerImpl(ctx, "tcp4", spec, net.IPv4(127, 0, 0, 1))
}

// NewTCP6Server creates a new TCP/IP6 stream server.
//
// spec is a string of the form [addr][:port].
//
// If addr is not empty, it will be an IPv6 network address to bind to. If it
// is empty, the StreamServer will bind exclusively to localhost.
//
// port must be a valid, available port. It may be omitted or <=0, in which case
// an ephemeral port will be chosen by the system. Note that, in this case, the
// caller cannot prescribe the port in advance, and must discover it via
// exported stream server parameters (externally) or by calling Address
// (internally).
func NewTCP6Server(ctx context.Context, spec string) (StreamServer, error) {
	return newTCPServerImpl(ctx, "tcp6", spec, net.IPv6loopback)
}

// Listen implements StreamServer.
func newTCPServerImpl(ctx context.Context, netType, spec string, loopback net.IP) (StreamServer, error) {
	tcpAddr, err := net.ResolveTCPAddr(netType, spec)
	if err != nil {
		return nil, errors.Annotate(err, "could not resolve %q address %q", netType, spec).Err()
	}

	if tcpAddr.IP == nil {
		tcpAddr.IP = loopback
	}

	return &listenerStreamServer{
		Context: ctx,
		address: fmt.Sprintf("%s:%s", netType, tcpAddr.String()),
		gen: func() (net.Listener, string, error) {
			l, err := net.ListenTCP(netType, tcpAddr)
			if err != nil {
				return nil, "", errors.Annotate(err, "failed to listen to %q address %q", netType, tcpAddr).Err()
			}

			addr := fmt.Sprintf("%s:%s", netType, l.Addr().String())
			log.Fields{
				"addr": addr,
			}.Debugf(ctx, "Listening on %q stream server...", netType)
			return l, addr, nil
		},
	}, nil
}
