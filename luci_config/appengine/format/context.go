// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package format

import (
	"github.com/luci/luci-go/luci_config/server/cfgclient"

	"golang.org/x/net/context"
)

var formatRegistryKey = "github.com/luci/luci-go/luci_config/appengine/format:FormatRegistry"

// WithRegistry returns a Context with the supplied FormatterRegistry embedded
// into it.
func WithRegistry(c context.Context, fr *cfgclient.FormatterRegistry) context.Context {
	return context.WithValue(c, &formatRegistryKey, fr)
}

// GetRegistry is used to retrieve the config service format registry from the
// supplied Context.
func GetRegistry(c context.Context) *cfgclient.FormatterRegistry {
	return c.Value(&formatRegistryKey).(*cfgclient.FormatterRegistry)
}
