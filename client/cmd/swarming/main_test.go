package main

import (
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func TestBotsCommand(t *testing.T) {
	Convey(`ok`, t, func() {
		app := getApplication(chromeinfra.DefaultAuthOptions())
		exitcode := subcommands.Run(app, []string{
			"bots", "-server", "chromium-swarm-dev.appspot.com",
		})
		So(exitcode, ShouldEqual, 0)
	})
}

func TestTaskssCommand(t *testing.T) {
	Convey(`ok`, t, func() {
		app := getApplication(chromeinfra.DefaultAuthOptions())
		exitcode := subcommands.Run(app, []string{
			"tasks", "-server", "chromium-swarm-dev.appspot.com", "-limit", "1",
		})
		So(exitcode, ShouldEqual, 0)
	})
}
