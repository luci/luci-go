package frontend

import (
	"net/http"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/server/router"
)

func StatsHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := buildbot.StatsHandler(c)
	if err != nil {
		logging.Errorf(c, "failed to send stats: %s", err)
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
