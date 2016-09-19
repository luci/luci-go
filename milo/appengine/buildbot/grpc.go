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

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/iotools"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	milo "github.com/luci/luci-go/milo/api/proto"
	"golang.org/x/net/context"
)

type BuildbotService struct{}

var errNotFoundGRPC = grpc.Errorf(codes.NotFound, "Master Not Found")

func (s *BuildbotService) GetCompressedMasterJSON(
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
	ds := datastore.Get(c)
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
		if err := ds.Get(slave.Runningbuilds); err != nil {
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
		q := datastore.NewQuery("buildbotBuild").
			Eq("finished", true).
			Eq("master", req.Name).
			Eq("builder", builderName).
			Limit(50).
			Order("-number").
			KeysOnly(true)
		var builds []*buildbotBuild
		err := ds.GetAll(q, &builds)
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
