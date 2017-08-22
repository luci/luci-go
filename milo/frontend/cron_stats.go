package frontend

import (
	"net/http"

	"github.com/luci/luci-go/milo/buildsource/buildbot"
	"github.com/luci/luci-go/server/router"
)

func StatsHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := buildbot.StatsHandler(c)
	if err != nil {
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
