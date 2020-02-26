// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ledcmd

import (
	"fmt"
	"net/http"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/grpc/prpc"
)

func newSwarmClient(authClient *http.Client, host string) *swarming.Service {
	swarm, err := swarming.New(authClient)
	if err != nil {
		panic(err)
	}
	swarm.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", host)
	return swarm
}

func newSwarmbucketClient(authClient *http.Client, host string) *swarmbucket.Service {
	// TODO(iannucci): Switch this to prpc endpoints
	sbucket, err := swarmbucket.New(authClient)
	if err != nil {
		panic(err)
	}
	sbucket.BasePath = fmt.Sprintf("https://%s/_ah/api/swarmbucket/v1/", host)
	return sbucket
}

func newBuildbucketClient(authClient *http.Client, host string) bbpb.BuildsClient {
	rpcOpts := prpc.DefaultOptions()

	if info, err := version.GetCurrentVersion(); err != nil {
		rpcOpts.UserAgent = fmt.Sprintf("led, %s @ %s", info.PackageName, info.InstanceID)
	} else {
		rpcOpts.UserAgent = "led, <unknown version>"
	}

	return bbpb.NewBuildsPRPCClient(&prpc.Client{
		C:       authClient,
		Host:    host,
		Options: rpcOpts,
	})
}
