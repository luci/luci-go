// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"fmt"
	"strings"

	"github.com/luci/luci-go/common/api/swarming/swarming/v1"

	"golang.org/x/net/context"
)

// BuildLog contains the log text retrieved from swarming.
// TODO(hinoka): Maybe put this somewhere more generic, like under resp/.
type BuildLog struct {
	log string
}

func swarmingBuildLogImpl(c context.Context, server string, taskID string, logname string) (*BuildLog, error) {
	sc, err := func(debug bool) (*swarming.Service, error) {
		if debug {
			return nil, nil
		}
		return getSwarmingClient(c, server)
	}(strings.HasPrefix(taskID, "debug:"))

	body, err := getSwarmingLog(sc, taskID)
	if err != nil {
		return nil, err
	}

	// Decode the data using annotee.
	s, err := streamsFromAnnotatedLog(c, body)
	if err != nil {
		return nil, err
	}

	k := fmt.Sprintf("steps%s", logname)
	if s, ok := s.Streams[k]; ok {
		return &BuildLog{log: s.Text}, nil
	}
	var keys []string
	for sk := range s.Streams {
		keys = append(keys, sk)
	}
	return nil, fmt.Errorf("%s not found in client\nAvailable keys:%s", k, keys)
}
