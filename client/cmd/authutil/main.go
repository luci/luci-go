// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Command authutil can be used to interact with OAuth2 token cache on disk.
//
// It hardcodes chrome-infra specific defaults.
//
// Use "github.com/luci/luci-go/client/authcli/authutil" package to implement
// a binary with different defaults.
package main

import (
	"os"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/client/authcli/authutil"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	app := authutil.GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, nil))
}
