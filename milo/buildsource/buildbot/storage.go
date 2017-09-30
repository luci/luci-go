// Copyright 2016 The LUCI Authors.
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

package buildbot

import (
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
)

// buildQueryBatchSize is the batch size to use when querying build.
//
// This should be tuned to the observed query timeout for build loading. Since
// loading is CPU-bound, this will probably be low. If build queries start
// encountering datastore timeouts, reduce this value.
const buildQueryBatchSize = int32(50)

// masterQueryBatchSize is the batch size to use when querying masters.
const masterQueryBatchSize = int32(500)

type trinary int

const (
	either trinary = iota
	yes
	no
)

type query struct {
	master   string
	builder  string
	limit    int
	finished trinary
	cursor   string
}

// getBuilds takes a build query and returns a list of builds
// along with a cursor.
func getBuilds(c context.Context, q query) ([]*buildbot.Build, string, error) {
	dsq := datastore.NewQuery("buildbotBuild").
		//Eq("finished", q.finishedOnly)
		Eq("master", q.master).
		Eq("builder", q.builder).
		Order("-number")
	switch q.finished {
	case yes:
		dsq = dsq.Eq("finished", true)
	case no:
		dsq = dsq.Eq("finished", false)
	}

	if q.cursor != "" {
		var number int
		if _, err := fmt.Sscanf(q.cursor, "<%d", &number); err == nil {
			dsq = dsq.Lt("number", number)
		} else if cur, err := datastore.DecodeCursor(c, q.cursor); err == nil {
			dsq = dsq.Start(cur)
		} else {
			return nil, "", errors.Annotate(err, "bad cursor").Err()
		}
	}

	if q.limit > 0 {
		dsq = dsq.Limit(int32(q.limit))
	}

	allocSize := buildQueryBatchSize
	if q.limit > 0 {
		allocSize = int32(q.limit)
	}
	builds := make([]*buildbot.Build, 0, allocSize)

	err := datastore.GetAll(c, dsq, &builds)
	if err != nil {
		return nil, "", err
	}

	var curs string
	if len(builds) > 0 {
		curs = fmt.Sprintf("<%d", builds[len(builds)-1].Number)
	}
	return builds, curs, nil
}

// getBuild fetches a buildbot build from the datastore and checks ACLs.
// The return code matches the master responses.
func getBuild(c context.Context, master, builder string, buildNum int) (*buildbot.Build, error) {
	if err := canAccessMaster(c, master); err != nil {
		return nil, err
	}

	result := &buildbot.Build{
		Master:      master,
		Buildername: builder,
		Number:      buildNum,
	}

	err := datastore.Get(c, result)
	if err == datastore.ErrNoSuchEntity {
		err = errors.New("build not found", common.CodeNotFound)
	}

	return result, err
}

func queryAllMasters(c context.Context) ([]*buildbotMasterEntry, error) {
	q := datastore.NewQuery(buildbotMasterEntryKind)
	entries := make([]*buildbotMasterEntry, 0, masterQueryBatchSize)
	err := datastore.RunBatch(c, masterQueryBatchSize, q, func(e *buildbotMasterEntry) {
		entries = append(entries, e)
	})
	return entries, err
}
