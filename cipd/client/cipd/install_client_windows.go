// Copyright 2015 The LUCI Authors.
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

package cipd

import (
	"context"
	"hash"
	"os"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

func (c *clientImpl) installClient(ctx context.Context, fs fs.FileSystem, h hash.Hash, fetchURL, destination, hexDigest string) error {
	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		// TODO(iannucci): worry about owner/group/permissions?
		if err := c.storage.download(ctx, fetchURL, of, h); err != nil {
			return err
		}
		if got := common.HexDigest(h); got != hexDigest {
			return cipderr.HashMismatch.Apply(errors.Fmt("client binary file hash mismatch: expecting %q, got %q", hexDigest, got))
		}
		return nil
	})
}
