package buildstore

import (
	"testing"

	"go.chromium.org/luci/milo/api/buildbot"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	Convey(`TestBuild`, t, func() {
		Convey(`Parse buildbucket property correctly`, func() {
			b := &buildbot.Build{
				Properties: []*buildbot.Property{
					{
						Name:   "buildbucket",
						Value:  "{\"build\": {\"bucket\": \"master.tryserver.chromium.linux\", \"created_by\": \"user:5071639625-1lppvbtck1morgivc6sq4dul7klu27sd@developer.gserviceaccount.com\", \"created_ts\": \"1512513476931350\", \"id\": \"8961008384372116384\", \"lease_key\": \"1020766456\", \"tags\": [\"builder:linux_chromium_rel_ng\", \"buildset:patch/gerrit/chromium-review.googlesource.com/809606/3\", \"cq_experimental:false\", \"master:master.tryserver.chromium.linux\", \"user_agent:cq\"]}}",
						Source: "buildbucket",
					},
				},
			}
			id, ok := getBuildbucketURI(b)
			So(ok, ShouldBeTrue)
			So(id, ShouldEqual, "buildbucket://cr-buildbucket.appspot.com/build/8961008384372116384")
		})
	})
}
