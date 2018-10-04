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
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
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
	field.Bool("exclude_deprecated"),
)

// Service is a service implementation that displays BuildBot builds.
type Service struct{}

var errNotFoundGRPC = status.Errorf(codes.NotFound, "Not found")

// GetBuildbotBuildJSON implements milo.BuildbotServer.
func (s *Service) GetBuildbotBuildJSON(c context.Context, req *milo.BuildbotBuildRequest) (
	*milo.BuildbotBuildJSON, error) {

	apiUsage.Add(c, 1, "GetBuildbotBuildJSON", req.Master, req.Builder, req.ExcludeDeprecated)

	if req.Master == "" {
		return nil, status.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, status.Errorf(codes.InvalidArgument, "No builder specified")
	}
	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is requesting %s/%s/%d",
		cu.Identity, req.Master, req.Builder, req.BuildNum)

	if err := grpcCanAccessMaster(c, req.Master); err != nil {
		return nil, err
	}

	b, err := buildstore.GetBuild(c, buildbot.BuildID{
		Master:  req.Master,
		Builder: req.Builder,
		Number:  int(req.BuildNum),
	})
	switch {
	case common.ErrorCodeIn(err) == common.CodeNoAccess:
		return nil, errNotFoundGRPC
	case err != nil:
		return nil, err
	case b == nil:
		return nil, errNotFoundGRPC
	}

	if req.ExcludeDeprecated {
		excludeDeprecatedFromBuild(b)
	}

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

	apiUsage.Add(c, 1, "GetBuildbotBuildsJSON", req.Master, req.Builder, req.ExcludeDeprecated)

	if req.Master == "" {
		return nil, status.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, status.Errorf(codes.InvalidArgument, "No builder specified")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 20
	}

	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is requesting %s/%s (limit %d)",
		cu.Identity, req.Master, req.Builder, limit)

	if err := grpcCanAccessMaster(c, req.Master); err != nil {
		return nil, err
	}

	q := buildstore.Query{
		Master:  req.Master,
		Builder: req.Builder,
		Limit:   limit,
	}
	if !req.IncludeCurrent {
		q.Finished = buildstore.Yes
	}
	res, err := buildstore.GetBuilds(c, q)
	switch {
	case common.ErrorCodeIn(err) == common.CodeNoAccess:
		return nil, errNotFoundGRPC
	case err != nil:
		return nil, err
	}

	buildsJSON := &milo.BuildbotBuildsJSON{
		Builds: make([]*milo.BuildbotBuildJSON, len(res.Builds)),
	}
	for i, b := range res.Builds {
		if req.ExcludeDeprecated {
			excludeDeprecatedFromBuild(b)
		}
		bs, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		buildsJSON.Builds[i] = &milo.BuildbotBuildJSON{Data: bs}
	}
	return buildsJSON, nil
}

// GetCompressedMasterJSON assembles a CompressedMasterJSON object from the
// provided MasterRequest.
func (s *Service) GetCompressedMasterJSON(c context.Context, req *milo.MasterRequest) (
	*milo.CompressedMasterJSON, error) {

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "No master specified")
	}

	apiUsage.Add(c, 1, "GetCompressedMasterJSON", req.Name, "", req.ExcludeDeprecated)

	cu := auth.CurrentUser(c)
	logging.Debugf(c, "%s is making a master request for %s", cu.Identity, req.Name)

	if err := grpcCanAccessMaster(c, req.Name); err != nil {
		return nil, err
	}

	master, err := buildstore.GetMaster(c, req.Name, true)

	switch code := common.ErrorCodeIn(err); {
	case code == common.CodeNotFound, code == common.CodeNoAccess:
		return nil, errNotFoundGRPC
	case err != nil:
		return nil, err
	}

	for _, builder := range master.Builders {
		// Nobody uses PendingBuildStates.
		// Exclude them from the response
		// because they are hard to emulate.
		builder.PendingBuildStates = nil
	}

	if req.ExcludeDeprecated {
		excludeDeprecatedFromMaster(&master.Master)
	}

	// Compress it.
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
		Internal: master.Internal,
		Modified: google.NewTimestamp(master.Modified),
		Data:     gzbs.Bytes(),
	}, nil
}

func excludeDeprecatedFromMaster(m *buildbot.Master) {
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

func excludeDeprecatedFromBuild(b *buildbot.Build) {
	b.Slave = ""
}

func grpcCanAccessMaster(c context.Context, master string) error {
	err := buildstore.CanAccessMaster(c, master)
	switch common.ErrorCodeIn(err) {
	case common.CodeNotFound:
		return errNotFoundGRPC
	case common.CodeUnauthorized:
		return status.Errorf(codes.Unauthenticated, "Unauthenticated request")
	default:
		return err
	}
}
