package frontend

import (
	"context"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/router"
	"net/http"
	"time"
)

// This file contains a cron handler that fixes the datastore.
//
// TODO(nodir): remove this file

func fixDatastore(c context.Context) error {
	// TODO(nodir): remove this handler once datastore is cleanup.
	deadline := clock.Now(c).Add(-24 * 2 * time.Hour)
	return parallel.FanOutIn(func(ch chan<- func() error) {
		for _, status := range []model.Status{model.NotRun, model.Running} {
			status := status
			q := datastore.NewQuery("BuildSummary").
				Eq("Summary.Status", status)
			ch <- func() error {
				dsCtx, _ := clock.WithTimeout(c, 9*time.Minute)
				deleted := 0
				err := datastore.Run(dsCtx, q, func(b *model.BuildSummary) error {
					if b.Created.Before(deadline) {
						if err := datastore.Delete(dsCtx, b); err != nil {
							return errors.Annotate(err, "failed to delete %s", b.BuildID).Err()
						}
						deleted++
					}
					return nil
				})
				if err == dsCtx.Err() {
					err = nil
				}
				logging.Infof(c, "deleted %d %s builds", deleted, status)
				return err
			}
		}
	})
}

func cronFixDatastore(c *router.Context) {
	err := fixDatastore(c.Context)
	if err != nil {
		logging.Errorf(c.Context, "%s", err)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}
