// Copyright 2021 The LUCI Authors.
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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/cv/internal/buildbucket"
)

var downloadBuildFlag = flag.Int64("download-build", 0, "Fetches & stores test data for a given build ID.\n\nFor internal builds, run inside luci-auth context, but beware of confidential data.")

func downloadFixture(ctx context.Context, buildID int64) error {
	opts := auth.Options{}
	opts.PopulateDefaults()
	rt, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, opts).Transport()
	if err != nil {
		return err
	}
	client := bbpb.NewBuildsPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: rt},
		Host: "cr-buildbucket.appspot.com",
	})
	build, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID, Mask: buildbucket.TryjobBuildMask})
	if err != nil {
		return err
	}

	buildJSON, err := protojson.MarshalOptions{Indent: "    "}.Marshal(build)
	if err != nil {
		return err
	}

	fName := fmt.Sprintf("%d.json", buildID)
	if err := os.WriteFile(fName, buildJSON, 0644); err != nil {
		return err
	}

	fmt.Printf("Downloaded build %d to %q\n", buildID, fName)
	return nil
}

func main() {
	ctx := context.Background()
	flag.Parse()
	if *downloadBuildFlag > 0 {
		if err := downloadFixture(ctx, *downloadBuildFlag); err != nil {
			logging.Errorf(ctx, "downloading %d %s", downloadBuildFlag, err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "-download-build is required")
	}
	os.Exit(0)
}
