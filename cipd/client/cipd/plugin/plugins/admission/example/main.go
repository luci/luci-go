// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/client/cipd/plugin/plugins/admission"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
	"go.chromium.org/luci/cipd/version"
)

var pluginVersion = "example 1.0"

func init() {
	if ver, _ := version.GetStartupVersion(); ver.InstanceID != "" {
		pluginVersion += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
	}
}

func main() {
	ctx := gologger.StdConfig.Use(context.Background())
	err := admission.Run(ctx, os.Stdin, pluginVersion, admissionHandler)
	if err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func admissionHandler(ctx context.Context, adm *protocol.Admission, info admission.InstanceInfo) error {
	// Some packages can be rejected just based on their name.
	if strings.HasPrefix(adm.Package, "experimental/") {
		return status.Errorf(codes.FailedPrecondition, "experimental packages are not allowed")
	}

	// In this primitive example a package is allowed for deployment if it has
	// a metadata entry with key "allowed-sha256" and a value that matches the
	// hash of the package (as hex encoded lowercase string). Note there can be
	// many metadata entries with key "allowed-sha256". Visit them all.
	visited := 0
	found := false
	err := info.VisitMetadata(ctx, []string{"allowed-sha256"}, 0, func(md *repopb.InstanceMetadata) bool {
		visited++
		found = string(md.Value) == adm.Instance.HexDigest
		return !found
	})

	switch {
	case err != nil:
		return err // 'ListMetadata' RPC to the CIPD backend failed
	case visited == 0:
		return status.Errorf(codes.FailedPrecondition, `doesn't have metadata entries with key "allowed-sha256"`)
	case !found:
		return status.Errorf(codes.FailedPrecondition, `evaluated %d metadata entries with key "allowed-sha256" and none is valid`, visited)
	default:
		return nil
	}
}
