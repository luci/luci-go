// Copyright 2015 The LUCI Authors.
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

package swarming

import (
	"context"
	"fmt"
	"path"
	"sort"

	mc "go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/logging"
)

// swarmingBuildLogImpl is the implementation for getting a log name from
// a swarming build via annotee.  It returns the full text of the specific log,
// and whether or not it has been closed.
func swarmingBuildLogImpl(c context.Context, svc SwarmingService, taskID, logname string) (string, bool, error) {
	server := svc.GetHost()
	cached, err := mc.GetKey(c, path.Join("swarmingLog", server, taskID, logname))
	switch {
	case err == mc.ErrCacheMiss:

	case err != nil:
		logging.WithError(err).Errorf(c, "failed to fetch log with key %s from memcache", cached.Key())

	default:
		logging.Debugf(c, "Cache hit for step log %s/%s/%s", server, taskID, logname)
		return string(cached.Value()), false, nil
	}

	fr, err := swarmingFetch(c, svc, taskID, swarmingFetchParams{fetchLog: true})
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
