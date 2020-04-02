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
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/server/router"
)

const botUsername = "luci-notify@chromium.org"
const legacyBotUsername = "buildbot@chromium.org"

type treeStatus struct {
	username  string
	message   string
	key       string
	status    config.TreeCloserStatus
	timestamp time.Time
}

type treeStatusClient interface {
	getStatus(c context.Context, host string) (*treeStatus, error)
	putStatus(c context.Context, host, message, prevKey string) error
}

// TODO: Write a real implementation.
type dummyTreeStatusClient struct{}

// For the dummy impl we return a closed status, set by someone else. We won't
// even attempt to re-open in this case.
func (dummy *dummyTreeStatusClient) getStatus(c context.Context, host string) (*treeStatus, error) {
	return &treeStatus{"someone.else", "", "", config.Closed, time.Now()}, nil
}
func (dummy *dummyTreeStatusClient) putStatus(c context.Context, host, message, prevKey string) error {
	return nil
}

// UpdateTreeStatus is the HTTP handler triggered by cron when it's time to
// check tree closers and update tree status if necessary.
func UpdateTreeStatus(c *router.Context) {
	ctx, w := c.Context, c.Writer
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := updateTrees(ctx, &dummyTreeStatusClient{}); err != nil {
		logging.WithError(err).Errorf(ctx, "error while updating tree status")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// updateTrees fetches all TreeClosers from datastore, uses this to determine if
// any trees should be opened or closed, and makes the necessary updates.
func updateTrees(c context.Context, ts treeStatusClient) error {
	var treeClosers []*config.TreeCloser
	if err := datastore.GetAll(c, datastore.NewQuery("TreeCloser"), &treeClosers); err != nil {
		return err
	}

	return parallel.WorkPool(32, func(ch chan<- func() error) {
		for host, treeClosers := range groupTreeClosers(treeClosers) {
			host, treeClosers := host, treeClosers
			ch <- func() error { return updateHost(c, ts, host, treeClosers) }
		}
	})
}

func groupTreeClosers(treeClosers []*config.TreeCloser) map[string][]*config.TreeCloser {
	byHost := map[string][]*config.TreeCloser{}
	for _, tc := range treeClosers {
		byHost[tc.TreeStatusHost] = append(byHost[tc.TreeStatusHost], tc)
	}

	return byHost
}

func updateHost(c context.Context, ts treeStatusClient, host string, treeClosers []*config.TreeCloser) error {
	var oldestClosed *config.TreeCloser
	for _, tc := range treeClosers {
		if tc.Status == config.Closed && (oldestClosed == nil || tc.Timestamp.Before(oldestClosed.Timestamp)) {
			oldestClosed = tc
		}
	}

	var overallStatus config.TreeCloserStatus
	if oldestClosed == nil {
		overallStatus = config.Open
	} else {
		overallStatus = config.Closed
	}

	treeStatus, err := ts.getStatus(c, host)
	switch {
	case err != nil:
		return err
	case treeStatus.status == overallStatus:
		// Nothing to do, status is already correct.
		return nil
	case treeStatus.status == config.Closed && treeStatus.username != botUsername && treeStatus.username != legacyBotUsername:
		// Don't reopen the tree if it wasn't automatically closed.
		return nil
	}

	var message string
	if overallStatus == config.Open {
		message = fmt.Sprintf("Tree is open (Automatic: %s)", randomEmoji())
	} else {
		// TODO: We actually want to render the template from oldestClosed.
		// We can't do this yet as we don't store the necessary info in the
		// TreeCloser config struct.
		message = fmt.Sprintf("Tree is closed (Automatic: %s)", "TODO")
	}
	// TODO: We also need to compare the TreeCloser timestamps against the
	// existing status timestamp, to ensure we're only acting on new builds.

	return ts.putStatus(c, host, message, treeStatus.key)
}

func randomEmoji() string {
	// TODO: Import the emojis from Gatekeeper.
	return "Yes!"
}
