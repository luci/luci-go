// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"golang.org/x/net/context"
)

type servicesKeyType int

// WithServices installs the supplied Services instance into a Context.
func WithServices(c context.Context, s Services) context.Context {
	return context.WithValue(c, servicesKeyType(0), s)
}

// GetServices gets the Services instance installed in the supplied Context.
//
// If no Services has been installed, it will panic.
func GetServices(c context.Context) Services {
	s, ok := c.Value(servicesKeyType(0)).(Services)
	if !ok {
		panic("no Services instance is installed")
	}
	return s
}
