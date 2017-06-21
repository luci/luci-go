// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"fmt"
	"path"
	"sort"

	"golang.org/x/net/context"

	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/logging"
)

// swarmingBuildLogImpl is the implementation for getting a log name from
// a swarming build via annotee.  It returns the full text of the specific log,
// and whether or not it has been closed.
func swarmingBuildLogImpl(c context.Context, svc swarmingService, taskID, logname string) (string, bool, error) {
	server := svc.getHost()
	cached, err := mc.GetKey(c, path.Join("swarmingLog", server, taskID, logname))
	switch {
	case err == mc.ErrCacheMiss:

	case err != nil:
		logging.WithError(err).Errorf(c, "failed to fetch log with key %s from memcache", cached.Key())

	default:
		logging.Debugf(c, "Cache hit for step log %s/%s/%s", server, taskID, logname)
		return string(cached.Value()), false, nil
	}

	fetchParams := swarmingFetchParams{
		fetchRes: true, // Needed so we can validate that this is a Milo build.
		fetchLog: true,
	}
	fr, err := swarmingFetch(c, svc, taskID, fetchParams)
	if err != nil {
		return "", false, err
	}

	// Decode the data using annotee.
	s, err := streamsFromAnnotatedLog(c, fr.log)
	if err != nil {
		return "", false, err
	}

	k := fmt.Sprintf("steps%s", logname)
	stream, ok := s.Streams[k]
	if !ok {
		var keys []string
		for sk := range s.Streams {
			keys = append(keys, sk)
		}
		sort.Strings(keys)
		return "", false, fmt.Errorf("stream %q not found; available streams: %q", k, keys)
	}

	if stream.Closed {
		cached.SetValue([]byte(stream.Text))
		if err := mc.Set(c, cached); err != nil {
			logging.Errorf(c, "Failed to write log with key %s to memcache: %s", cached.Key(), err)
		}
	}

	return stream.Text, stream.Closed, nil
}
