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

package metadata

import (
	"fmt"

	"golang.org/x/net/context"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// legacyStorageImpl implements Storage on top of PackageACL entities inherited
// from Python version of CIPD backend.
type legacyStorageImpl struct {
}

func (legacyStorageImpl) GetMetadata(c context.Context, prefix string) ([]*api.PrefixMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (legacyStorageImpl) UpdateMetadata(c context.Context, prefix string, cb func(m *api.PrefixMetadata) error) (*api.PrefixMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}
