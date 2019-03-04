// Copyright 2017 The LUCI Authors.
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

package streamclient

import (
	"io"
	"net"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/types"
)

func tcpProtocolClientFactory(netType string) ClientFactory {
	return func(spec string, ns types.StreamName) (Client, error) {
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
			ns: ns,
		}, nil
	}
}
