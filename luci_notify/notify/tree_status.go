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

const botUsername = "buildbot@chromium.org"

type TreeStatus struct {
	username string
	message  string
	key      string
	status   config.TreeCloserStatus
}

type TreeStatusClient interface {
	getStatus(host string) (*TreeStatus, error)
	putStatus(host, message, prevKey string) error
}

// TODO: Write a real implementation.
type DummyTreeStatusClient struct{}

// For the dummy impl we return a closed status, set by someone else. We won't
// even attempt to re-open in this case.
func (dummy *DummyTreeStatusClient) getStatus(host string) (*TreeStatus, error) {
	return &TreeStatus{"someone.else", "", "", config.Closed}, nil
}
func (dummy *DummyTreeStatusClient) putStatus(host, message, prevKey string) error {
	return nil
}

// UpdateTreeStatus is the HTTP handler triggered by cron when it's time to
// check tree closers and update tree status if necessary.
func UpdateTreeStatus(c *router.Context) {
	ctx, w := c.Context, c.Writer
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := updateTrees(ctx, &DummyTreeStatusClient{}); err != nil {
		logging.WithError(err).Errorf(ctx, "error while updating tree status")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// updateTrees fetches all TreeClosers from datastore, uses this to determine if
// any trees should be opened or closed, and makes the necessary updates.
func updateTrees(c context.Context, ts TreeStatusClient) error {
	var treeClosers []*config.TreeCloser
	if err := datastore.GetAll(c, datastore.NewQuery("TreeCloser"), &treeClosers); err != nil {
		return err
	}

	return parallel.FanOutIn(func(ch chan<- func() error) {
		for host, treeClosers := range groupTreeClosers(treeClosers) {
			host, treeClosers := host, treeClosers
			ch <- func() error { return updateHost(ts, host, treeClosers) }
		}
	})
}

func groupTreeClosers(treeClosers []*config.TreeCloser) map[string][]*config.TreeCloser {
	byHost := map[string][]*config.TreeCloser{}
	for _, tc := range treeClosers {
		slice, exists := byHost[tc.TreeStatusHost]
		if !exists {
			slice = []*config.TreeCloser{}
		}

		byHost[tc.TreeStatusHost] = append(slice, tc)
	}

	fmt.Printf("groupTreeClosers(%+v) -> %+v\n", treeClosers, byHost)
	return byHost
}

func updateHost(ts TreeStatusClient, host string, treeClosers []*config.TreeCloser) error {
	var oldestClosed *config.TreeCloser = nil
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

	treeStatus, err := ts.getStatus(host)
	if err != nil {
		return err
	}
	if treeStatus.status == overallStatus {
		// Nothing to do, status is already correct.
		return nil
	}
	if treeStatus.status == config.Closed && treeStatus.username != botUsername {
		// Don't reopen the tree if we weren't the one to close it.
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
	fmt.Printf("Putting status %q for host %s\n", message, host)
	return ts.putStatus(host, message, treeStatus.key)
}

func randomEmoji() string {
	// TODO: Import the emojis from Gatekeeper.
	return "Yes!"
}
