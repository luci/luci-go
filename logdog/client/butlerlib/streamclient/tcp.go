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
			return nil, errors.Annotate(err).Reason("could not resolve %(net)q address from %(spec)q").
				D("net", netType).
				D("spec", spec).
				Err()
		}

		if raddr.IP == nil || raddr.IP.IsUnspecified() {
			return nil, errors.Reason("a valid %(net)q address must be provided").
				D("net", netType).
				Err()
		}

		if raddr.Port <= 0 {
			return nil, errors.New("a valid port must be provided")
		}

		return &clientImpl{
			factory: func() (io.WriteCloser, error) {
				conn, err := net.DialTCP(netType, nil, raddr)
				if err != nil {
					return nil, errors.Annotate(err).Reason("failed to dial %(net)q address %(raddr)q").
						D("net", netType).
						D("raddr", raddr).
						Err()
				}
				return conn, nil
			},
		}, nil
	}
}
