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
	"fmt"
	"hash"
	"os"

	"golang.org/x/net/context"

	"go.chromium.org/luci/cipd/client/cipd/local"
	"go.chromium.org/luci/cipd/common"
)

func (client *clientImpl) installClient(ctx context.Context, fs local.FileSystem, h hash.Hash, fetchURL, destination, hexDigest string) error {
	return fs.EnsureFile(ctx, destination, func(of *os.File) error {
		// TODO(iannucci): worry about owner/group/permissions?
		if err := client.storage.download(ctx, fetchURL, of, h); err != nil {
			return err
		}
		if got := common.HexDigest(h); got != hexDigest {
			return fmt.Errorf("file hash mismatch: expecting %q, got %q", hexDigest, got)
		}
		return nil
	})
}
