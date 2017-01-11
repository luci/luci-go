// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package format implements a default FormatterRegistry configuration and
// FormatterRegistry Context embedding.
package format

import (
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
)

// Default returns a new default FormatterRegistry configuration.
func Default() *cfgclient.FormatterRegistry {
	var fr cfgclient.FormatterRegistry
	textproto.RegisterFormatter(&fr)
	return &fr
}
