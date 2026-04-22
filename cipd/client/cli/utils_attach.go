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

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
)

func attachAndMove(ctx context.Context, client cipd.Client, pin common.Pin, md []cipd.Metadata, tags tagList, refs refList) error {
	if err := client.AttachMetadataWhenReady(ctx, pin, md); err != nil {
		return err
	}
	if err := client.AttachTagsWhenReady(ctx, pin, tags); err != nil {
		return err
	}
	for _, ref := range refs {
		if err := client.SetRefWhenReady(ctx, ref, pin); err != nil {
			return err
		}
	}
	return nil
}
