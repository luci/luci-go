// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/maruel/interrupt"
)

// URLToHTTPS ensures the url is https://.
func URLToHTTPS(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" && u.Scheme != "https" {
		return "", errors.New("Only https:// scheme is accepted. It can be omitted.")
	}
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	if _, err = url.Parse(s); err != nil {
		return "", err
	}
	return s, nil
}

// IsDirectory returns true if path is a directory and is accessible.
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// Semaphore is a classic semaphore.
type Semaphore interface {
	// Wait waits for the Semaphore to have 1 item available.
	//
	// Returns interrupt.ErrInterrupted if interrupt was triggered. It may return
	// nil even if the interrupt signal is set.
	Wait() error
	// Signal adds 1 to the semaphore.
	Signal()
}

type semaphore chan bool

// NewSemaphore returns a Semaphore of specified size.
func NewSemaphore(size int) Semaphore {
	s := make(semaphore, size)
	for i := 0; i < cap(s); i++ {
		s <- true
	}
	return s
}

func (s semaphore) Wait() error {
	// "select" randomly selects a channel when both channels are available. It
	// is still possible that the other channel is also available.
	select {
	case <-interrupt.Channel:
		return interrupt.ErrInterrupted
	case <-s:
		return nil
	}
}

func (s semaphore) Signal() {
	s <- true
}
