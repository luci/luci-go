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
	"strings"

	"go.chromium.org/luci/cipkg/core"
	luciproto "go.chromium.org/luci/common/proto"

	"google.golang.org/protobuf/proto"
)

type FetchURL struct {
	URL           string
	Mode          fs.FileMode
	HashAlgorithm core.HashAlgorithm
	HashValue     string
}

// FetchURLs downloads files from servers based on the path-url pairs of URLs.
type FetchURLs struct {
	Name     string
	Metadata *core.Action_Metadata
	URLs     map[string]FetchURL
}

func (f *FetchURLs) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	var deps []*core.Action

	// Generate separate action for every url.
	files := make(map[string]*core.ActionFilesCopy_Source)
	for k, v := range f.URLs {
		spec := &core.ActionURLFetch{
			Url:           v.URL,
			HashAlgorithm: v.HashAlgorithm,
			HashValue:     v.HashValue,
		}

		// Truncate the id to save some characters for windows because this id will
		// be used as part of the path. 32^6 = 2^30 should be good enough.
		id, err := stableID(spec, 6)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("%s_%s", f.Name, id)

		deps = append(deps, &core.Action{
			Name:     name,
			Metadata: &core.Action_Metadata{ContextInfo: f.Metadata.GetContextInfo()},
			Spec:     &core.Action_Url{Url: spec},
		})

		m := v.Mode
		if m == 0 {
			m = 0o666
		}
		files[k] = &core.ActionFilesCopy_Source{
			Content: &core.ActionFilesCopy_Source_Output_{
				Output: &core.ActionFilesCopy_Source_Output{Name: name, Path: "file"},
			},
			Mode: uint32(m),
		}
	}

	// TODO: Sort deps
	return &core.Action{
		Name:     f.Name,
		Metadata: f.Metadata,
		Deps:     deps,
		Spec: &core.Action_Copy{
			Copy: &core.ActionFilesCopy{
				Files: files,
			},
		},
	}, nil
}

func stableID(m proto.Message, n uint) (string, error) {
	h := sha256.New()
	if err := luciproto.StableHash(h, m); err != nil {
		return "", err
	}
	enc := base32.HexEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(h.Sum(nil)[:n])), nil
}
