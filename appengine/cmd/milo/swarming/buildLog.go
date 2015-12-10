// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"
)

func swarmingBuildLogImpl(c context.Context, server string, id string, logname string) (*BuildLog, error) {
	// Fetch the data from Swarming
	body, err := getSwarmingLog(server, id, c)
	if err != nil {
		return nil, err
	}

	// Decode the data using annotee.
	client, err := clientFromAnnotatedLog(c, body)
	if err != nil {
		return nil, err
	}

	k := strings.Join([]string{"steps", logname}, "")
	if s, ok := client.stream[k]; ok {
		return &BuildLog{log: s.buf.String()}, nil
	}
	return nil, fmt.Errorf("%s not found in client", k)
}
