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
	"os"
	"runtime"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

type netDialer struct {
	addr net.Addr
}

var _ dialer = (*netDialer)(nil)

func (u *netDialer) conn(f streamproto.Flags) (net.Conn, error) {
	conn, err := net.Dial(u.addr.Network(), u.addr.String())
	if err != nil {
		return nil, errors.Annotate(err, "opening socket %q", u.addr).Err()
	}
	if err = f.WriteHandshake(conn); err != nil {
		conn.Close()
		return nil, errors.Annotate(err, "writing handshake").Err()
	}
	return conn, nil
}

func (u *netDialer) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	conn, err := u.conn(f)
	if err != nil {
		return nil, err
	}

	// Exclude windows from File here, since Windows:
	//   1. does not allow OVERLAPPED network sockets (i.e. the kind opened by
	//       Golang's guts) to be attached as stdout/stderr HANDLEs.
	//   2. returns syscall.EWINDOWS from `TCPConn.File()` (at least as of Go 1.13)
	//
	// If someone cares to fix this, they could change 'conn' above to manually
	// create a SOCKET with WSASocket without overlapped, connect the socket to
	// `address`, and then convert the SOCKET (which is a HANDLE in disguise) to
	// an *os.File.
	//
	// For an example, see ConnectSocketHandle here (MIT/LGPL dual license):
	// https://github.com/cubiclesoft/createprocess-windows/blob/c102c9b1c1d8f43d1cac7b64944cbecdfe49ee6b/createprocess.cpp#L235
	//
	// Essentially:
	//
	//   once {
	//     globalWSData = WSAStartup(...)
	//   }
	//   if network == "tcp4":
	//     addr := parse address to sockaddr_in
	//     sock := WSASocket(AF_INET, SOCK_STREAM, IPPROTO_TCP, NULL, 0, 0)
	//     connect(sock, &addr)
	//   elif network == "tcp6":
	//     addr := parse address to sockaddr_in6
	//     sock := WSASocket(AF_INET6, SOCK_STREAM, IPPROTO_TCP, NULL, 0, 0)
	//     connect(sock, &addr)
	//   return sock
	if !forProcess || runtime.GOOS == "windows" {
		return conn, nil
	}

	if fi, ok := conn.(interface{ File() (*os.File, error) }); ok {
		fd, err := fi.File()
		// either File dup'd our connection, or it failed; either way conn must be
		// closed here.
		conn.Close()
		return fd, errors.Annotate(err, "converting to os.File").Err()
	}

	return conn, nil
}

func (u *netDialer) DialDgramStream(f streamproto.Flags) (DatagramStream, error) {
	conn, err := u.conn(f)
	if err != nil {
		return nil, err
	}
	return &datagramStreamWriter{conn}, nil
}

// newNetDialer creates a new dialer instance bound to a network address.
func newNetDialer(network string, resolver func(address string) (net.Addr, error)) dialFactory {
	return func(address string) (dialer, error) {
		raddr, err := resolver(address)
		if err != nil {
			return nil, errors.Annotate(err, "resolving address %q", address).Err()
		}
		return &netDialer{raddr}, nil
	}
}

func init() {
	resolver := func(network string) func(address string) (net.Addr, error) {
		return func(address string) (net.Addr, error) {
			raddr, err := net.ResolveTCPAddr(network, address)
			if err != nil {
				return nil, errors.Annotate(err, "could not resolve %q address from %q", address, network).Err()
			}
			if raddr.IP == nil || raddr.IP.IsUnspecified() {
				return nil, errors.Reason("a valid %q address must be provided", network).Err()
			}
			if raddr.Port <= 0 {
				return nil, errors.New("a valid port must be provided")
			}
			return raddr, nil
		}
	}

	protocolRegistry["tcp4"] = newNetDialer("tcp4", resolver("tcp4"))
	protocolRegistry["tcp6"] = newNetDialer("tcp6", resolver("tcp6"))
}
