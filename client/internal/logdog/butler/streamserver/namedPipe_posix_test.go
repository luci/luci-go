// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package streamserver

import (
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/context"
)

type posixNamedPipeTester struct {
	t *testing.T
}

// Decorates a test function with a NamedPipeServer serving from a temporary
// path.
func (npt *posixNamedPipeTester) withServer(f func(i *namedPipeTestInstance)) func() {
	return func() {
		// Create a temporary testing directory.
		tempdir, err := ioutil.TempDir(os.TempDir(), "named_pipe_posix_test")
		if err != nil {
			panic("Failed to create testing temporary directory.")
		}
		defer func() {
			// Cleanup the temporary directory.
			os.RemoveAll(tempdir)
		}()

		path := filepath.Join(tempdir, "butler.sock")
		s := NewNamedPipeServer(context.Background(), path).(*namedPipeServer)
		npt.t.Logf("Created testing server for: %s", path)

		// Start listening.
		err = s.Listen()
		if err != nil {
			panic("Failed to listen on server instance.")
		}
		defer func() {
			// Close the server. Some of our tests might mess with this, so check
			// first to make sure it's still open.
			if s.l != nil {
				s.Close()
			}
		}()

		// goroutine-safe connection creation.
		inst := &namedPipeTestInstance{
			S: s,
			connect: func() io.WriteCloser {
				npt.t.Logf("Connecting to testing server at: %s", path)
				conn, err := net.Dial("unix", path)
				if err != nil {
					npt.t.Logf("Failed to connect client instance: %s", err)
					return nil
				}
				return conn
			},
		}

		// Execute the function using this valid server!
		f(inst)
	}
}

func TestNamedPipeServer(t *testing.T) {
	testNamedPipeServer(t, &posixNamedPipeTester{t})
}
