// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/hardcoded/chromeinfra"
	"github.com/luci/luci-go/logdog/client/cli"

	"golang.org/x/net/context"
)

func main() {
	mathrand.SeedRandomly()
	os.Exit(cli.Main(context.Background(), cli.Parameters{
		Args:               os.Args[1:],
		DefaultAuthOptions: chromeinfra.DefaultAuthOptions(),
	}))
}
