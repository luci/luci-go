// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// uploadOptions mixin.

// uploadOptions defines command line options for commands that upload packages.
type uploadOptions struct {
	verificationTimeout time.Duration
	attestation         string
	writeCacheDir       string
	skipRemoteUpload    bool
}

func (opts *uploadOptions) registerFlags(f *flag.FlagSet) {
	f.DurationVar(
		&opts.verificationTimeout, "verification-timeout",
		cipd.CASFinalizationTimeout, "Maximum time to wait for backend-side package hash verification.")
	f.StringVar(&opts.attestation, "attestation", "", "Path to the attestation bundle file for the instance.")
	f.StringVar(&opts.writeCacheDir, "write-cache-dir", "",
		fmt.Sprintf("Directory `path` to store the package in addition to uploading to the CIPD backend (can also be set by %s env var).", cipd.EnvWriteCacheDir))
	f.BoolVar(&opts.skipRemoteUpload, "skip-remote-upload", false,
		fmt.Sprintf("Skip uploading packages and metadata to the remote CIPD backend (can also be set by $%s=1).", cipd.EnvSkipRemoteUpload))
}

func (opts *uploadOptions) targetCacheDir(ctx context.Context) string {
	if opts.writeCacheDir != "" {
		return opts.writeCacheDir
	}
	return environ.FromCtx(ctx).Get(cipd.EnvWriteCacheDir)
}

func (opts *uploadOptions) shouldSkipRemoteUpload(ctx context.Context) bool {
	if opts.skipRemoteUpload {
		return true
	}
	v := environ.FromCtx(ctx).Get(cipd.EnvSkipRemoteUpload)
	return v == "1" || v == "true"
}
