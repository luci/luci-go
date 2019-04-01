package main

import (
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/buildbucket/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	app := cli.Application(cli.Params{
		Auth:                   chromeinfra.DefaultAuthOptions(),
		DefaultBuildbucketHost: chromeinfra.BuildbucketHost,
	})
	os.Exit(subcommands.Run(app, os.Args[1:]))
}
