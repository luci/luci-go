package casviewer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHandlers(t *testing.T) {
	t.Parallel()

	Convey("InstallHandlers", t, func() {
		r := router.New()
		InstallHandlers(r)

		srv := httptest.NewServer(r)
		client := &http.Client{}

		resp, err := client.Get(srv.URL)
		So(err, ShouldBeNil)
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})
}
