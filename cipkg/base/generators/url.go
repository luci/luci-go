// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"go.chromium.org/luci/cipkg/core"
	luciproto "go.chromium.org/luci/common/proto"

	"google.golang.org/protobuf/proto"
)

type FetchURL struct {
	Name     string
	Metadata *core.Action_Metadata

	URL           string
	HashAlgorithm core.HashAlgorithm
	HashValue     string
	Filename      string
	Mode          fs.FileMode
}

func (f *FetchURL) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	return &core.Action{
		Name:     f.Name,
		Metadata: f.Metadata,
		Spec: &core.Action_Url{
			Url: &core.ActionURLFetch{
				Url:           f.URL,
				HashAlgorithm: f.HashAlgorithm,
				HashValue:     f.HashValue,
				Name:          f.Filename,
				Mode:          uint32(f.Mode),
			},
		},
	}, nil
}

// FetchURLs update the urls slices passed in place and generates a sorted list
// of FetchURL and set a unique name with hash calculated from the content if
// the Name of FetchURL is empty.
func FetchURLs(prefix string, urls []*FetchURL) ([]Generator, error) {
	for _, u := range urls {
		// Truncate the id to save some characters for windows because this id will
		// be used as part of the path. 32^6 = 2^30 should be good enough.
		id, err := stableID(&core.ActionURLFetch{
			Url:           u.URL,
			HashAlgorithm: u.HashAlgorithm,
			HashValue:     u.HashValue,
			Name:          u.Filename,
			Mode:          uint32(u.Mode),
		}, 6)
		if err != nil {
			return nil, err
		}

		if u.Name == "" {
			u.Name = fmt.Sprintf("%s_%s", prefix, id)
		}
	}

	// Make sure urls is sorted so dependencies can be stable.
	slices.SortFunc(urls, func(a *FetchURL, b *FetchURL) int {
		return strings.Compare(a.Name, b.Name)
	})

	gs := make([]Generator, 0, len(urls))
	for _, u := range urls {
		gs = append(gs, u)
	}

	return gs, nil
}

func stableID(m proto.Message, n uint) (string, error) {
	h := sha256.New()
	if err := luciproto.StableHash(h, m); err != nil {
		return "", err
	}
	enc := base32.HexEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(h.Sum(nil)[:n])), nil
}
