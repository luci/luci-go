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

//go:build unix
// +build unix

package streamclient

import (
	"io"
	"net"
	"os"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

type unixDialer struct {
	path string
}

var _ dialer = (*unixDialer)(nil)

func (u *unixDialer) conn(f streamproto.Flags) (*net.UnixConn, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: u.path})
	if err != nil {
		return nil, errors.Annotate(err, "opening socket %q", u.path).Err()
	}
	if err = f.WriteHandshake(conn); err != nil {
		conn.Close()
		return nil, errors.Annotate(err, "writing handshake").Err()
	}
	return conn, nil
}

func (u *unixDialer) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	conn, err := u.conn(f)
	if err != nil {
		return nil, err
	}

	if !forProcess {
		return conn, nil
	}

	fd, err := conn.File()
	// either File dup'd our connection, or it failed; either way conn must be
	// closed here.
	conn.Close()
	return fd, errors.WrapIf(err, "converting to os.File")
}

func (u *unixDialer) DialDgramStream(f streamproto.Flags) (DatagramStream, error) {
	conn, err := u.conn(f)
	if err != nil {
		return nil, err
	}
	return &datagramStreamWriter{conn}, nil
}

func init() {
	protocolRegistry["unix"] = func(address string) (dialer, error) {
		// Ensure that the supplied address exists and is a named pipe.
		info, err := os.Lstat(address)
		if err != nil {
			return nil, errors.Reason("failed to stat file [%s]: %s", address, err).Err()
		}
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.Reason("not a named pipe: [%s]", address).Err()
		}
		return &unixDialer{address}, nil
	}
}
