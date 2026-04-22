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
	"flag"
	"time"

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// uploadOptions mixin.

// uploadOptions defines command line options for commands that upload packages.
type uploadOptions struct {
	verificationTimeout time.Duration
	attestation         string
}

func (opts *uploadOptions) registerFlags(f *flag.FlagSet) {
	f.DurationVar(
		&opts.verificationTimeout, "verification-timeout",
		cipd.CASFinalizationTimeout, "Maximum time to wait for backend-side package hash verification.")
	f.StringVar(&opts.attestation, "attestation", "", "Path to the attestation bundle file for the instance.")
}
