// Copyright 2018 The LUCI Authors.
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

package reader

import (
	"context"
	"io"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

type noopCloserSrc struct {
	io.ReadSeeker
}

func (noopCloserSrc) Close(ctx context.Context, corrupt bool) error { return nil }

// CalculatePin returns a pin that represents the package.
//
// It reads the package name from the manifest inside, and calculates package's
// hash to get instance ID.
func CalculatePin(ctx context.Context, body io.ReadSeeker, algo api.HashAlgo) (common.Pin, error) {
	inst, err := OpenInstance(ctx, noopCloserSrc{body}, OpenInstanceOpts{
		VerificationMode: CalculateHash,
		HashAlgo:         algo,
	})
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)
	return inst.Pin(), nil
}
