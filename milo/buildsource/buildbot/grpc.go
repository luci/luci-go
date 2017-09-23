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
	"bytes"
	"compress/gzip"
	"encoding/json"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/timestamp"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
)

var apiUsage = metric.NewCounter(
	"luci/milo/api/buildbot/usage",
	"The number of received buildbot API requests",
	nil,
	field.String("method"),
	field.String("master"),
	field.String("builder"),
)

// Service is a service implementation that displays BuildBot builds.
type Service struct{}

var errNotFoundGRPC = grpc.Errorf(codes.NotFound, "Master Not Found")

// GetBuildbotBuildJSON implements milo.BuildbotServer.
func (s *Service) GetBuildbotBuildJSON(c context.Context, req *milo.BuildbotBuildRequest) (
	*milo.BuildbotBuildJSON, error) {

	apiUsage.Add(c, 1, "GetBuildbotBuildJSON", req.Master, req.Builder)

	if req.Master == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No builder specified")
	}

	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is requesting %s/%s/%d",
		cu.Identity, req.Master, req.Builder, req.BuildNum)

	b, err := getBuild(c, req.Master, req.Builder, int(req.BuildNum))
	if err != nil {
		switch common.ErrorTag.In(err) {
		case common.CodeNotFound:
			return nil, grpc.Errorf(codes.NotFound, "Build not found")
		case common.CodeUnauthorized:
			return nil, grpc.Errorf(codes.Unauthenticated, "Unauthenticated request")
		default:
			return nil, err
		}
	}

	if req.ExcludeDeprecated {
		excludeDeprecatedFromBuild(b)
	}

	updatePostProcessBuild(b)
	bs, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}

	// Marshal the build back into JSON format.
	return &milo.BuildbotBuildJSON{Data: bs}, nil
}

// GetBuildbotBuildsJSON implements milo.BuildbotServer.
func (s *Service) GetBuildbotBuildsJSON(c context.Context, req *milo.BuildbotBuildsRequest) (
	*milo.BuildbotBuildsJSON, error) {

	apiUsage.Add(c, 1, "GetBuildbotBuildsJSON", req.Master, req.Builder)
	if req.Cursor != "" {
		logging.Warningf(
			c,
			"%q requested GetBuildbotBuildsJSON with cursor for builder %q:%q",
			auth.CurrentIdentity(c), req.Master, req.Builder)
	}

	if req.Master == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No builder specified")
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is requesting %s/%s (limit %d, cursor %s)",
		cu.Identity, req.Master, req.Builder, limit, req.Cursor)

	if err := canAccessMaster(c, req.Master); err != nil {
		switch common.ErrorTag.In(err) {
		case common.CodeNotFound:
			return nil, grpc.Errorf(codes.NotFound, "Master not found")
		case common.CodeUnauthorized:
			return nil, grpc.Errorf(codes.Unauthenticated, "Unauthenticated request")
		default:
			return nil, err
		}
	}

	q := datastore.NewQuery("buildbotBuild")
	q = q.Eq("master", req.Master).
		Eq("builder", req.Builder).
		Limit(limit).
		Order("-number")
	if req.IncludeCurrent == false {
		q = q.Eq("finished", true)
	}
	// Insert the cursor or offset.
	if req.Cursor != "" {
		cursor, err := datastore.DecodeCursor(c, req.Cursor)
		if err != nil {
			return nil, grpc.Errorf(codes.InvalidArgument, "Invalid cursor: %s", err.Error())
		}
		q = q.Start(cursor)
	}
	builds, nextCursor, err := runBuildsQuery(c, q)
	if err != nil {
		return nil, err
	}

	results := make([]*milo.BuildbotBuildJSON, len(builds))
	for i, b := range builds {
		updatePostProcessBuild(b)

		// In theory we could do this in parallel, but it doesn't actually go faster
		// since AppEngine is single-cored.
		bs, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		results[i] = &milo.BuildbotBuildJSON{Data: bs}
	}
	buildsJSON := &milo.BuildbotBuildsJSON{
		Builds: results,
	}
	if nextCursor != nil {
		buildsJSON.Cursor = nextCursor.String()
	}
	return buildsJSON, nil
}

// GetCompressedMasterJSON assembles a CompressedMasterJSON object from the
// provided MasterRequest.
func (s *Service) GetCompressedMasterJSON(c context.Context, req *milo.MasterRequest) (
	*milo.CompressedMasterJSON, error) {

	if req.Name == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}

	apiUsage.Add(c, 1, "GetCompressedMasterJSON", req.Name, "")

	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is making a master request for %s", cu.Identity, req.Name)

	entry, err := getMasterEntry(c, req.Name)
	if err != nil {
		switch common.ErrorTag.In(err) {
		case common.CodeNotFound:
			return nil, errNotFoundGRPC
		case common.CodeUnauthorized:
			return nil, grpc.Errorf(codes.Unauthenticated, "Unauthenticated request")
		default:
			return nil, err
		}
	}

	// Decompress it so we can inject current build information.
	master := &buildbotMaster{}
	if err = decodeMasterEntry(c, entry, master); err != nil {
		return nil, err
	}

	if req.ExcludeDeprecated {
		excludeDeprecatedFromMaster(master)
	}

	for _, slave := range master.Slaves {
		numBuilds := 0
		for _, builds := range slave.RunningbuildsMap {
			numBuilds += len(builds)
		}
		slave.Runningbuilds = make([]*buildbotBuild, 0, numBuilds)
		for builderName, builds := range slave.RunningbuildsMap {
			for _, buildNum := range builds {
				slave.Runningbuilds = append(slave.Runningbuilds, &buildbotBuild{
					Master:      req.Name,
					Buildername: builderName,
					Number:      buildNum,
				})
			}
		}
		if err := datastore.Get(c, slave.Runningbuilds); err != nil {
			logging.WithError(err).Errorf(c,
				"Encountered error while trying to fetch running builds for %s: %v",
				master.Name, slave.Runningbuilds)
			return nil, err
		}

		for _, b := range slave.Runningbuilds {
			updatePostProcessBuild(b)
		}
	}

	// Also inject cached builds information.
	for builderName, builder := range master.Builders {
		// Get the most recent 50 buildNums on the builder to simulate what the
		// cachedBuilds field looks like from the real buildbot master json.
		q := datastore.NewQuery("buildbotBuild").
			Eq("finished", true).
			Eq("master", req.Name).
			Eq("builder", builderName).
			Limit(50).
			Order("-number").
			KeysOnly(true)
		var builds []*buildbotBuild
		err := datastore.RunBatch(c, buildQueryBatchSize, q, func(b *buildbotBuild) {
			if req.ExcludeDeprecated {
				excludeDeprecatedFromBuild(b)
			}
			builds = append(builds, b)
		})
		if err != nil {
			return nil, err
		}
		builder.CachedBuilds = make([]int, len(builds))
		for i, b := range builds {
			builder.CachedBuilds[i] = b.Number
		}
	}

	// And re-compress it.
	gzbs := bytes.Buffer{}
	gsw := gzip.NewWriter(&gzbs)
	cw := iotools.CountingWriter{Writer: gsw}
	e := json.NewEncoder(&cw)
	if err := e.Encode(master); err != nil {
		gsw.Close()
		return nil, err
	}
	gsw.Close()

	logging.Infof(c, "Returning %d bytes", cw.Count)

	return &milo.CompressedMasterJSON{
		Internal: entry.Internal,
		Modified: &timestamp.Timestamp{
			Seconds: entry.Modified.Unix(),
			Nanos:   int32(entry.Modified.Nanosecond()),
		},
		Data: gzbs.Bytes(),
	}, nil
}

func excludeDeprecatedFromMaster(m *buildbotMaster) {
	m.Slaves = nil
	for _, builder := range m.Builders {
		builder.Slaves = nil
	}
	for _, builder := range m.Buildstate.Builders {
		builder.Slaves = nil
	}
	for _, builder := range m.Varz.Builders {
		builder.ConnectedSlaves = 0
		builder.TotalSlaves = 0
	}
}

func excludeDeprecatedFromBuild(b *buildbotBuild) {
	b.Slave = ""
}
