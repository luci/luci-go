// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iotools

import (
	"errors"
	"io"
	"net"
	"time"
)

// ErrTimeout is an error returned if the DeadlineReader times out.
var ErrTimeout = errors.New("deadlinereader: timeout")

// DeadlineReader is a wrapper around a net.Conn that applies an idle timeout
// deadline to the Conn's Read() method.
type DeadlineReader struct {
	net.Conn

	Deadline time.Duration // The read deadline to apply prior to each 'Read()'
}

var _ io.ReadCloser = (*DeadlineReader)(nil)

// Read implements io.Reader.
func (r *DeadlineReader) Read(b []byte) (int, error) {
	// If we have a deadline, apply it before the 'Read()'
	if r.Deadline > 0 {
		deadline := time.Now().Add(r.Deadline)
		if err := r.Conn.SetDeadline(deadline); err != nil {
			return 0, err
		}
	}

	amount, err := r.Conn.Read(b)
	if e, ok := err.(net.Error); ok && e.Timeout() {
		// Don't report back read timeout errors.
		err = ErrTimeout
	}
	return amount, err
}
