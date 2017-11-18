package frontend

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/common/model"
)

// This file contains a cron handler that fixes the datastore.
//
// TODO(nodir): delete this file

func fixDatastore(c context.Context) error {
	// delete all incomplete builds that were created >2 days ago
	now := clock.Now(c)
	buildDeadline := now.Add(-24 * 2 * time.Hour)
	runDeadline := now.Add(50 * time.Second)
	return parallel.FanOutIn(func(ch chan<- func() error) {
		for _, status := range []model.Status{model.NotRun, model.Running} {
			status := status
			ch <- func() error {
				deleted := 0
				q := datastore.NewQuery("BuildSummary").Eq("Summary.Status", status)
				err := datastore.Run(c, q, func(b *model.BuildSummary) error {
					switch {
					case clock.Now(c).After(runDeadline):
						return datastore.Stop // enough for now
					case !b.Created.Before(buildDeadline):
						return nil
					}
					if err := datastore.Delete(c, b); err != nil {
						return errors.Annotate(err, "failed to delete %s", b.BuildID).Err()
					}
					deleted++
					return nil
				})
				logging.Infof(c, "deleted %d %s builds", deleted, status)
				return err
			}
		}
	})
}

func cronFixDatastore(c *router.Context) {
	if err := fixDatastore(c.Context); err != nil {
		logging.Errorf(c.Context, "%s", err)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}
