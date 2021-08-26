// Copyright 2021 The LUCI Authors.
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

// +build !copybara

package lib

import (
	"context"

	"go.chromium.org/luci/common/data/embeddedkvs"
)

func init() {
	// This is not to introduce dependency to embeddedkvs in google3.
	// See https://crbug.com/1202614
	newSmallFileCache = func(ctx context.Context, path string) (smallFileCache, error) {
		return embeddedkvs.New(ctx, path)
	}
}
