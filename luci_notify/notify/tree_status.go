// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package notify

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// UpdateTreeStatus is the HTTP handler triggered by cron when it's time to
// check tree closers and update tree status if necessary.
func UpdateTreeStatus(c *router.Context) {
	ctx, w := c.Context, c.Writer
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := updateTrees(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "error while updating tree status")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// updateTrees fetches all TreeClosers from datastore, uses this to determine if
// any trees should be opened or closed, and makes the necessary updates.
//
// TODO: Or at least it will - it's just a placeholder for now.
func updateTrees(c context.Context) error {
	return nil
}
