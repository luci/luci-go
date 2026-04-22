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
	"os"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
	"go.chromium.org/luci/common/errors"
)

func registerInstanceFile(ctx context.Context, instanceFile string, knownPin *common.Pin, opts *registerOpts) (common.Pin, error) {
	// Load metadata, in particular process -metadata-from-file, which may fail.
	metadata, err := opts.metadataOptions.load(ctx)
	if err != nil {
		return common.Pin{}, err
	}

	src, err := pkg.NewFileSource(instanceFile)
	if err != nil {
		if os.IsNotExist(err) {
			return common.Pin{}, cipderr.BadArgument.Apply(errors.Fmt("missing input instance file: %w", err))
		}
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening input instance file: %w", err))
	}
	defer src.Close(ctx, false)

	// Calculate the pin if not yet known.
	var pin common.Pin
	if knownPin != nil {
		pin = *knownPin
	} else {
		pin, err = reader.CalculatePin(ctx, src, opts.hashAlgo())
		if err != nil {
			return common.Pin{}, err
		}
	}
	inspectPin(ctx, pin)

	client, err := opts.clientOptions.makeCIPDClient(ctx, "", nil)
	if err != nil {
		return common.Pin{}, err
	}
	defer client.Close(ctx)

	var attestation string
	if opts.uploadOptions.attestation != "" {
		blob, err := os.ReadFile(opts.uploadOptions.attestation)
		if err != nil {
			return common.Pin{}, cipderr.IO.Apply(errors.Fmt("reading attestation file: %w", err))
		}
		attestation = string(blob)
	}

	err = client.RegisterInstance(ctx, pin, src, attestation, opts.uploadOptions.verificationTimeout)
	if err != nil {
		return common.Pin{}, err
	}
	err = attachAndMove(ctx, client, pin, metadata, opts.tags, opts.refs)
	if err != nil {
		return common.Pin{}, err
	}
	return pin, nil
}
