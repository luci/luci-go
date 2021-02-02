package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// IntegrationTestEnvVar is the name of the environment variable which controls
// whether integaration tests are executed.
// The value must be "1" for integration tests to run.
const IntegrationTestEnvVar = "INTEGRATION_TESTS"

// runIntegrationTests true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
}

// runCmd runs swarming commands appending common flags.
// It skips if integration should not run.
func runCmd(t *testing.T, cmd string, args ...string) int {
	if !runIntegrationTests() {
		t.Skipf("Skip integration tests")
	}
	args = append([]string{cmd, "-server", "chromium-swarm-dev.appspot.com"}, args...)
	return subcommands.Run(getApplication(chromeinfra.DefaultAuthOptions()), args)
}

func TestBotsCommand(t *testing.T) {
	Convey(`ok`, t, func() {
		dir, err := ioutil.TempDir("", "test-bots-cmd")
		So(err, ShouldBeEmpty)
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "bots", "-quiet", "-json", jsonPath), ShouldEqual, 0)
	})
}

func TestTasksCommand(t *testing.T) {
	Convey(`ok`, t, func() {
		dir, err := ioutil.TempDir("", "test-tasks-cmd")
		So(err, ShouldBeEmpty)
		jsonPath := filepath.Join(dir, "out.json")

		So(runCmd(t, "tasks", "-limit", "1", "-quiet", "-json", jsonPath), ShouldEqual, 0)
	})
}

func TestTriggerCommand(t *testing.T) {
	Convey(`ok`, t, func() {
		args := []string{
			"-d", "pool=chromium.tests",
			"-d", "os=Linux",
			"-idempotent",
			"--", "/bin/echo", "hi",
		}
		So(runCmd(t, "trigger", args...), ShouldEqual, 0)
	})
}
