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

//go:build windows
// +build windows

package streamclient

import (
	"context"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

func init() {
	protocolRegistry["net.pipe"] = newNamedPipeClient
}

const (
	securitySQOSPresent = 0x100000
	securityAnonymous   = 0

	errorPipeBusy = syscall.Errno(231)
)

// dialFilePipe attempts to dial the pipe at `path` until we get an error other
// than "PipeBusy".
//
// This is adapted from 'go-winio', which is MIT licensed.
//
// https://github.com/microsoft/go-winio/blob/84b4ab48a50763fe7b3abcef38e5205c12027fac/pipe.go
//
// The main difference here is that it opens the pipe in non-OVERLAPPED mode.
// This is required because windows doesn't permit OVERLAPPED handles to be
// attached to stdout/stderr of a new process, and winio always dials its
// handles in OVERLAPPED mode.
//
// We still want to dial non-ForProcess handles in OVERLAPPED mode (with
// additional winio machinery), to allow go to conserve system threads.
// Otherwise go will do regular thread-pool IO for every in-process connection.
func dialFilePipe(path string) (*os.File, error) {
	pPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.Fmt("unable to render %q to UTF16: %w", path, err)
	}

	for {
		h, err := syscall.CreateFile(
			pPath,                 // lpFileName
			syscall.GENERIC_WRITE, // dwDesiredAccess - Only need to write stuff
			0,                     // dwShareMode - "exclusive"
			nil,                   // lpSecurityAttributes - ignored for existing files
			syscall.OPEN_EXISTING, // dwCreationDisposition
			(securitySQOSPresent | securityAnonymous), // dwFlagsAndAttributes
			// Security SQOS Present
			//    - Tell the server how much they can impersonate us.
			// SecurityAnonymous
			//    - The server cannot impersonate or identify us.
			0, // hTemplateFile - ignored for existing files
		)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
		if err != errorPipeBusy {
			return nil, &os.PathError{Err: err, Op: "open", Path: path}
		}
		// TODO(iannucci): do exponential backoff/timeout?
		time.Sleep(time.Millisecond * 10)
	}
}

type winPipeDialer struct {
	pipeName string
}

func (u *winPipeDialer) conn(forProcess bool, f streamproto.Flags) (conn io.WriteCloser, err error) {
	if forProcess {
		if conn, err = dialFilePipe(u.pipeName); err != nil {
			return nil, errors.Fmt("opening named pipe (synchronous) %q: %w", u.pipeName, err)
		}
	} else {
		// For consistency, we dial without any timeout here.
		//
		// TODO(iannucci): Support timeouts for ALL connection methods as part of
		// the New*Stream public interface.
		if conn, err = winio.DialPipeContext(context.Background(), u.pipeName); err != nil {
			return nil, errors.Fmt("opening named pipe %q: %w", u.pipeName, err)
		}
	}

	if err = f.WriteHandshake(conn); err != nil {
		conn.Close()
		return nil, errors.Fmt("writing handshake: %w", err)
	}
	return
}

func (u *winPipeDialer) DialStream(forProcess bool, f streamproto.Flags) (io.WriteCloser, error) {
	return u.conn(forProcess, f)
}

func (u *winPipeDialer) DialDgramStream(f streamproto.Flags) (DatagramStream, error) {
	conn, err := u.conn(false, f)
	if err != nil {
		return nil, err
	}
	return &datagramStreamWriter{conn}, nil
}

// newNamedPipeClient creates a new dialer instance bound to a named pipe stream
// server.
func newNamedPipeClient(path string) (dialer, error) {
	if path == "" {
		return nil, errors.New("streamclient: cannot have empty named pipe path")
	}
	return &winPipeDialer{streamproto.LocalNamedPipePath(path)}, nil
}
