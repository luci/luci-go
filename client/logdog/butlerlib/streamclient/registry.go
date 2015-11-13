// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamclient

import (
	"fmt"
	"strings"
	"sync"
)

var (
	// defaultRegistry is the default protocol registry.
	defaultRegistry = &protocolRegistry{}
)

// protocolRegistry maps protocol prefix strings to their Client generator
// functions.
//
// This allows multiple Butler stream protocols (e.g., "unix:", "net.pipe:",
// etc.) to be parsed from string.
type protocolRegistry struct {
	sync.Mutex

	// protocols is the set of registered protocols. Each client should register
	// via registerProtocol in its init() method.
	protocols map[string]clientFactory
}

func (r *protocolRegistry) register(name string, f clientFactory) {
	r.Lock()
	defer r.Unlock()

	if _, ok := r.protocols[name]; ok {
		panic(fmt.Errorf("streamclient: protocol already registered for [%s]", name))
	}
	if r.protocols == nil {
		r.protocols = make(map[string]clientFactory)
	}
	r.protocols[name] = f
}

// newClient invokes the protocol clientFactory generator for the
// supplied protocol/address string, returning the generated Client.
func (r *protocolRegistry) newClient(path string) (Client, error) {
	parts := strings.SplitN(path, ":", 2)
	params := ""
	if len(parts) == 2 {
		params = parts[1]
	}

	r.Lock()
	defer r.Unlock()

	if f, ok := r.protocols[parts[0]]; ok {
		return f(params)
	}
	return nil, fmt.Errorf("streamclient: no protocol registered for [%s]", parts[0])
}

// registerProtocol registers a protocol with the default (global) protocol
// registry.
func registerProtocol(name string, f clientFactory) {
	defaultRegistry.register(name, f)
}
