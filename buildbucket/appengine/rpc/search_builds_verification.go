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

// TODO(crbug/1090540): remove after verification is done.
package rpc

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// bbClientErr is tagged into errors caused by bb client creation.
var bbClientErr = errors.BoolTag{Key: errors.NewTagKey("BB client")}

type pyBuildBucketClient struct {
	client pb.BuildsClient
}

type searchResponseBundle struct {
	res *pb.SearchBuildsResponse
	err error
}

// newPyBBClient constructs a BuildBucket python client.
func newPyBBClient(ctx context.Context, host string) (*pyBuildBucketClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get RPC transport to BB default service").Tag(bbClientErr).Err()
	}
	pClient := pb.NewBuildsPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: t},
		Host: host,
	})
	return &pyBuildBucketClient{client: pClient}, nil
}

// verifySearch makes RPC calls to Py service, compares Py and Go results and returns Py results.
func verifySearch(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
	// create Py BB client.
	pyHost := "default-dot-cr-buildbucket.appspot.com"
	if ctx.Value("env") == "Dev" {
		pyHost = "default-dot-cr-buildbucket-dev.appspot.com"
	}
	pyClient, err := newPyBBClient(ctx, pyHost)
	if err != nil {
		return nil, err
	}

	// get Go results.
	goResCh := make(chan *searchResponseBundle)
	goCtx, cancelGoCtx := context.WithCancel(ctx)
	defer cancelGoCtx()
	go func() {
		defer close(goResCh)
		goRes, goErr := searchBuildsGoPath(goCtx, req)
		select {
		case <-goCtx.Done():
			logging.Debugf(goCtx, "search verification: go context is done. goCtx err: %v", goCtx.Err())
			return
		case goResCh <- &searchResponseBundle{res: goRes, err: goErr}:
		}
	}()

	// get Py results.
	logging.Debugf(ctx, "search verification: calling pyHost:%s", pyHost)
	pyStartedAt := clock.Now(ctx)
	pyRes, pyErr := pyClient.client.SearchBuilds(ctx, req)
	logDuration(ctx, pyStartedAt, "search verification: py RPC calls for SearchBuilds")

	// convert the gRPC error to the appstatus error.
	if pyErr != nil {
		if e, ok := status.FromError(pyErr); ok {
			pyErr = appstatus.Error(e.Code(), e.Message())
		} else {
			logging.Debugf(ctx, "search verification: not able to parse rpc errors from Py service - %v", pyErr)
		}
	}

	// compare search results.
	select {
	case goResBundle := <-goResCh:
		compareSearchResults(ctx, goResBundle, &searchResponseBundle{res: pyRes, err: pyErr})
	case <-time.After(5 * time.Second):
		cancelGoCtx()
	}
	return pyRes, pyErr
}

// compareSearchResults is to compareSearchResults py and go responses for SearchBuilds.
func compareSearchResults(ctx context.Context, goResBundle *searchResponseBundle, pyResBundle *searchResponseBundle) {
	if goResBundle == nil {
		logging.Warningf(ctx, "search verification: diff found - goResBundle is nil, while pyResBundle res is %s and err is %v", pyResBundle.res.String(), pyResBundle.err)
		return
	}
	// only compare error code as some go error messages are different from py.
	if goResBundle.err != nil || pyResBundle.err != nil {
		gStatus, gOk := appstatus.Get(goResBundle.err)
		pyStatus, pyOk := appstatus.Get(pyResBundle.err)
		if gOk && pyOk && gStatus.Code() == pyStatus.Code() {
			logging.Debugf(ctx, "search verification: no diff for err responses")
		} else {
			logging.Warningf(ctx, "search verification: diff found - err is not equal\n go - %v\n py - %v", goResBundle.err, pyResBundle.err)
		}
		return
	}

	var miss, extra []int64
	// using two-pointer algorithm, as build ids are sorted in ascending order.
	i, j := 0, 0
	for i < len(goResBundle.res.Builds) && j < len(pyResBundle.res.Builds) {
		if goResBundle.res.Builds[i].Id == pyResBundle.res.Builds[j].Id {
			i++
			j++
		} else if goResBundle.res.Builds[i].Id < pyResBundle.res.Builds[j].Id {
			extra = append(extra, goResBundle.res.Builds[i].Id)
			i++
		} else {
			miss = append(miss, pyResBundle.res.Builds[j].Id)
			j++
		}
	}
	for ; i < len(goResBundle.res.Builds); i++ {
		extra = append(extra, goResBundle.res.Builds[i].Id)
	}
	for ; j < len(pyResBundle.res.Builds); j++ {
		miss = append(miss, pyResBundle.res.Builds[i].Id)
	}

	if len(miss) == 0 && len(extra) == 0 {
		logging.Debugf(ctx, "search verification: no diff for normal responses")
	} else {
		logging.Warningf(ctx, "search verification: diff found - \n"+
			"Go is missing %v, and has extra %v \nids in Go: %v \nids in Py: %v",
			miss, extra, getBuildIDs(goResBundle.res.Builds), getBuildIDs(pyResBundle.res.Builds))
	}
}

func getBuildIDs(builds []*pb.Build) []int64 {
	ids := make([]int64, len(builds))
	for i, b := range builds {
		ids[i] = b.Id
	}
	return ids
}

// searchBuildsGoPath executes the normal Go search path.
// All code is copied from `SearchBuilds` function.
func searchBuildsGoPath(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
	startedAt := clock.Now(ctx)
	defer logDuration(ctx, startedAt, "search verification: Go path for SearchBuilds")
	logging.Debugf(ctx, "search verification: searching in Go")
	if err := validateSearch(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	mask, err := getBuildsSubMask(req.GetFields())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "fields").Err())
	}

	rsp, err := search.NewQuery(req).Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if err = model.LoadBuildBundles(ctx, rsp.Builds, mask); err != nil {
		return nil, err
	}
	return rsp, nil
}

func logDuration(ctx context.Context, start time.Time, msg string) {
	logging.Debugf(ctx, "%s took %s", msg, time.Since(start))
}
