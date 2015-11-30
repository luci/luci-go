// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package main

import (
	"errors"
	"fmt"

	"github.com/luci/luci-go/client/internal/logdog/butler/streamserver"
	"golang.org/x/net/context"
)

const (
	// An example stream server URI.
	exampleStreamServerURI = streamServerURI("unix:/var/run/butler.sock")
)

type streamServerURI string

func (u streamServerURI) Parse() (string, error) {
	typ, value := parseStreamServer(string(u))
	if typ != "unix" {
		return "", fmt.Errorf("unsupported URI scheme: [%s]", typ)
	}
	if value == "" {
		return "", errors.New("empty stream server path")
	}
	return value, nil
}

// Validates that the URI is correct for Windows.
func (u streamServerURI) Validate() (err error) {
	_, err = u.Parse()
	return
}

// Create a POSIX (UNIX named pipe) stream server
func createStreamServer(ctx context.Context, uri streamServerURI) streamserver.StreamServer {
	path, err := uri.Parse()
	if err != nil {
		panic("Failed to parse stream server URI.")
	}
	return streamserver.NewNamedPipeServer(ctx, path)
}
