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

	"go.chromium.org/luci/cipd/client/cipd/plugin/admission"
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
	err := admission.RunPlugin(ctx, os.Stdin, pluginVersion, admissionHandler)
	if err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func admissionHandler(ctx context.Context, adm *protocol.Admission) error {
	if strings.HasPrefix(adm.Package, "experimental/") {
		return status.Errorf(codes.FailedPrecondition, "experimental packages are not allowed")
	}
	return nil
}
