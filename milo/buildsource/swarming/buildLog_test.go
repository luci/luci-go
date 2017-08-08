package swarming

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

var testSvc = &debugSwarmingService{
	tc: &testCase{
		name:        "build-patch-failure",
		swarmResult: "build-patch-failure.swarm",
		swarmOutput: "build-patch-failure",
	},
}

func TestBuildLogs(t *testing.T) {
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
	c = memory.UseWithAppID(c, "dev~luci-milo")
	Convey(`Build log tests`, t, func() {
		_, _, err := swarmingBuildLogImpl(c, testSvc, "12340", "/update_scripts/0/stdout")
		So(err, ShouldBeNil)
	})
	Convey(`List available streams`, t, func() {
		_, _, err := swarmingBuildLogImpl(c, testSvc, "12340", "/notexist")
		So(strings.HasPrefix(err.Error(), "stream \"steps/notexist\" not found"), ShouldEqual, true)
	})
}
