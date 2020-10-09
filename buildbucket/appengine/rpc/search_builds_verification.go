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
	go func() {
		goRes, goErr := searchBuildsGoPath(ctx, req)
		goResCh <- &searchResponseBundle{res: goRes, err: goErr}
	}()

	// get Py results.
	logging.Debugf(ctx, "search verification: calling pyHost:%s", pyHost)
	pyRes, pyErr := pyClient.client.SearchBuilds(ctx, req)
	if pyErr != nil {
		logging.Errorf(ctx, "search verification: Python response error - %v", pyErr)
	}
	logging.Debugf(ctx, "search verification: successfully get Python response")

	// compare search results.
	select {
	case goResBundle := <-goResCh:
		compareSearchResults(ctx, goResBundle, &searchResponseBundle{res: pyRes, err: pyErr})
	case <-time.After(5 * time.Second):
		logging.Debugf(ctx, "search verification: timeout in waiting Go results")
	}
	return pyRes, pyErr
}

// compareSearchResults is to compareSearchResults py and go responses for SearchBuilds.
func compareSearchResults(ctx context.Context, goResBundle *searchResponseBundle, pyResBundle *searchResponseBundle) {
	if goResBundle == nil {
		logging.Warningf(ctx, "search verification: diff found - goResBundle is nil, while pyResBundle %v", pyResBundle)
		return
	}
	if goResBundle.err != pyResBundle.err {
		logging.Warningf(ctx, "search verification: diff found - err is not equal\n go - %v\n py - %v", goResBundle.err, pyResBundle.err)
		return
	}

	gIDs := getBuildIDs(goResBundle.res.Builds)
	pIDs := getBuildIDs(pyResBundle.res.Builds)
	isSame := len(gIDs) == len(pIDs)
	for i := 0; i < len(gIDs) && isSame; {
		isSame = gIDs[i] == pIDs[i]
		i++
	}
	if isSame {
		logging.Debugf(ctx, "search verification: no diff")
	} else {
		logging.Warningf(ctx, "search verification: diff found - \nids in Go: %v \nids in Py: %v", gIDs, pIDs)
	}
}

func getBuildIDs(builds []*pb.Build) []int64 {
	ids := make([]int64, len(builds))
	for _, b := range builds {
		ids = append(ids, b.Id)
	}
	return ids
}

// searchBuildsGoPath executes the normal Go search path.
// All code is copied from `SearchBuilds` function.
func searchBuildsGoPath(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
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
