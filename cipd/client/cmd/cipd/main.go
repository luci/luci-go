// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package main contains CIPD CLI implementation that uses Chrome Infrastructure
// defaults.
//
// It hardcodes default CIPD backend URL, OAuth client ID, location of the token
// cache, etc.
//
// See github.com/luci/luci-go/cipd/client/cli if you want to build your own
// version with different defaults.
package main

import (
	"os"

	"github.com/luci/luci-go/cipd/client/cli"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	params := cli.Parameters{
		DefaultAuthOptions: chromeinfra.DefaultAuthOptions(),
		ServiceURL:         chromeinfra.CIPDServiceURL,
	}
	os.Exit(cli.Main(params, os.Args[1:]))
}
