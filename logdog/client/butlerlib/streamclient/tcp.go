// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package streamclient

import (
	"io"
	"net"

	"github.com/luci/luci-go/common/errors"
)

func tcpProtocolClientFactory(netType string) ClientFactory {
	return func(spec string) (Client, error) {
		raddr, err := net.ResolveTCPAddr(netType, spec)
		if err != nil {
			return nil, errors.Annotate(err, "could not resolve %q address from %q", netType, spec).Err()
		}

		if raddr.IP == nil || raddr.IP.IsUnspecified() {
			return nil, errors.Reason("a valid %q address must be provided", netType).Err()
		}

		if raddr.Port <= 0 {
			return nil, errors.New("a valid port must be provided")
		}

		return &clientImpl{
			factory: func() (io.WriteCloser, error) {
				conn, err := net.DialTCP(netType, nil, raddr)
				if err != nil {
					return nil, errors.Annotate(err, "failed to dial %q address %q", netType, raddr).Err()
				}
				return conn, nil
			},
		}, nil
	}
}
