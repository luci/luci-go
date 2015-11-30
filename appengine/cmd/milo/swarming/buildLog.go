// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"
)

func swarmingBuildLogImpl(c context.Context, server string, id string, log string, step string) (*BuildLog, error) {
	// Fetch the data from Swarming
	body, err := getSwarmingLog(server, id, c)
	if err != nil {
		return nil, err
	}

	// Decode the data using annotee.
	client, err := clientFromAnnotatedLog(body)
	if err != nil {
		return nil, err
	}

	var k string
	if log == "stdio" {
		k = strings.Join([]string{"steps", step, "stdio"}, "/")
	} else {
		k = strings.Join([]string{"steps", step, "logs", log}, "/")
	}
	if s, ok := client.stream[k]; ok {
		return &BuildLog{log: s.buf.String()}, nil
	}
	return nil, fmt.Errorf("%s not found in client", k)
}
