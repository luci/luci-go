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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

type visitPinsArgs struct {
	clientOptions

	packagePrefix string
	version       string

	updatePin func(client cipd.Client, pin common.Pin) error
}

func visitPins(ctx context.Context, args *visitPinsArgs) ([]pinInfo, error) {
	client, err := args.clientOptions.makeCIPDClient(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	// Do not touch anything if some packages do not have requested version. So
	// resolve versions first and only then move refs.
	pins, err := performBatchOperation(ctx, batchOperation{
		client:        client,
		packagePrefix: args.packagePrefix,
		callback: func(pkg string) (common.Pin, error) {
			return client.ResolveVersion(ctx, pkg, args.version)
		},
	})
	if err != nil {
		return nil, err
	}
	if hasErrors(pins) {
		printPinsAndError(map[string][]pinInfo{"": pins})
		return nil,
			cipderr.InvalidVersion.WithDetails(cipderr.Details{
				Version: args.version,
			}).Apply(errors.
				Fmt("can't find %q version in all packages, aborting", args.version))
	}

	// Prepare for the next batch call.
	packages := make([]string, len(pins))
	pinsToUse := make(map[string]common.Pin, len(pins))
	for i, p := range pins {
		packages[i] = p.Pkg
		pinsToUse[p.Pkg] = *p.Pin
	}

	// Update all refs or tags.
	return performBatchOperation(ctx, batchOperation{
		client:   client,
		packages: packages,
		callback: func(pkg string) (common.Pin, error) {
			pin := pinsToUse[pkg]
			if err := args.updatePin(client, pin); err != nil {
				return common.Pin{}, err
			}
			return pin, nil
		},
	})
}
