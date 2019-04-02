package main

import (
	"os"

	"go.chromium.org/luci/buildbucket/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	p := cli.Params{
		Auth:                   chromeinfra.DefaultAuthOptions(),
		DefaultBuildbucketHost: chromeinfra.BuildbucketHost,
	}
	os.Exit(cli.Main(p, os.Args[1:]))
}
