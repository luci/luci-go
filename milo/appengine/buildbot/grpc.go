// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"bytes"
	"compress/gzip"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/iotools"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	milo "github.com/luci/luci-go/milo/api/proto"
	"golang.org/x/net/context"
)

// Service is a service implementation that displays BuildBot builds.
type Service struct{}

var errNotFoundGRPC = grpc.Errorf(codes.NotFound, "Master Not Found")

func (s *Service) GetBuildbotBuildJSON(
	c context.Context, req *milo.BuildbotBuildRequest) (
	*milo.BuildbotBuildJSON, error) {

	if req.Master == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No builder specified")
	}

	b, err := getBuild(c, req.Master, req.Builder, int(req.BuildNum))
	switch {
	case err == errBuildNotFound:
		return nil, grpc.Errorf(codes.NotFound, "Build not found")
	case err != nil:
		return nil, err
	}

	bs, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}

	// Marshal the build back into JSON format.
	return &milo.BuildbotBuildJSON{Data: bs}, nil
}

func (s *Service) GetBuildbotBuildsJSON(
	c context.Context, req *milo.BuildbotBuildsRequest) (
	*milo.BuildbotBuildsJSON, error) {

	if req.Master == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}
	if req.Builder == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No builder specified")
	}

	// Perform an ACL check by fetching the master.
	_, err := getMasterEntry(c, req.Master)
	switch {
	case err == errMasterNotFound:
		return nil, grpc.Errorf(codes.NotFound, "Master not found")
	case err != nil:
		return nil, err
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	q := ds.NewQuery("buildbotBuild")
	q = q.Eq("master", req.Master).
		Eq("builder", req.Builder).
		Limit(limit).
		Order("-number")
	if req.IncludeCurrent == false {
		q = q.Eq("finished", true)
	}
	builds := []*buildbotBuild{}
	err = ds.GetAll(c, q, &builds)
	if err != nil {
		return nil, err
	}

	results := make([]*milo.BuildbotBuildJSON, len(builds))
	for i, b := range builds {
		// In theory we could do this in parallel, but it doesn't actually go faster
		// since AppEngine is single-cored.
		bs, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		results[i] = &milo.BuildbotBuildJSON{Data: bs}
	}
	return &milo.BuildbotBuildsJSON{
		Builds: results,
	}, nil
}

// GetCompressedMasterJSON assembles a CompressedMasterJSON object from the
// provided MasterRequest.
func (s *Service) GetCompressedMasterJSON(
	c context.Context, req *milo.MasterRequest) (*milo.CompressedMasterJSON, error) {

	if req.Name == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}

	entry, err := getMasterEntry(c, req.Name)
	switch {
	case err == errMasterNotFound:
		return nil, errNotFoundGRPC

	case err != nil:
		return nil, err
	}
	// Decompress it so we can inject current build information.
	master := &buildbotMaster{}
	if err = decodeMasterEntry(c, entry, master); err != nil {
		return nil, err
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
		if err := ds.Get(c, slave.Runningbuilds); err != nil {
			logging.WithError(err).Errorf(c,
				"Encountered error while trying to fetch running builds for %s: %s",
				master.Name, slave.Runningbuilds)
			return nil, err
		}
	}

	// Also inject cached builds information.
	for builderName, builder := range master.Builders {
		// Get the most recent 50 buildNums on the builder to simulate what the
		// cachedBuilds field looks like from the real buildbot master json.
		q := ds.NewQuery("buildbotBuild").
			Eq("finished", true).
			Eq("master", req.Name).
			Eq("builder", builderName).
			Limit(50).
			Order("-number").
			KeysOnly(true)
		var builds []*buildbotBuild
		err := ds.GetAll(c, q, &builds)
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
		Modified: &google.Timestamp{
			Seconds: entry.Modified.Unix(),
			Nanos:   int32(entry.Modified.Nanosecond()),
		},
		Data: gzbs.Bytes(),
	}, nil
}
