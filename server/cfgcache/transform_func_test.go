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

package cfgcache

import (
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestTransformFunc(t *testing.T) {
	t.Parallel()

	t.Run("validation errors", func(t *testing.T) {
		t.Parallel()

		noopXfrm := TransformFunc[*timestamppb.Timestamp](func(c *validation.Context, snap *Snapshot) (*timestamppb.Timestamp, error) {
			return &timestamppb.Timestamp{}, nil
		})

		cases := []struct {
			name     string
			xfrm     TransformFunc[*timestamppb.Timestamp]
			xfrmVers int
			files    []File
			wantErr  string
		}{
			{
				name:    "empty Files",
				wantErr: `Files must not be empty`,
			},
			{
				name:    "nil File",
				files:   []File{nil},
				wantErr: `Files[0]: must not be nil`,
			},
			{
				name:    "empty SingleFile Path",
				files:   []File{&SingleFile[*timestamppb.Timestamp]{}},
				wantErr: `Files[0]: Path must not be empty`,
			},
			{
				name:     "empty RegexFile PathRegex",
				xfrm:     noopXfrm,
				xfrmVers: 1,
				files:    []File{&RegexFile[*timestamppb.Timestamp]{}},
				wantErr:  `Files[0]: PathRegex must not be empty`,
			},
			{
				name:     "File unmarshalFunc error",
				xfrm:     noopXfrm,
				xfrmVers: 1,
				files:    []File{&SingleFile[int]{Path: "num.txt"}},
				wantErr:  `Files[0]: Unmarshal must be set for non-proto type int`,
			},
			{
				name:    "different SingleFile and StoreMsg with zero TransformVersion",
				xfrm:    noopXfrm,
				files:   []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				wantErr: `TransformVersion must be non-zero unless Files has a single SingleFile[StoreMsg]`,
			},
			{
				name:     "different SingleFile and StoreMsg without Transform",
				xfrmVers: 1,
				files:    []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				wantErr:  `Transform must be set unless Files has a single SingleFile[StoreMsg]`,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				reg := &Registry{}
				(&ServicePipeline[*timestamppb.Timestamp]{
					ID:               "test",
					ServiceName:      "my-app",
					Files:            tc.files,
					TransformVersion: tc.xfrmVers,
					Transform:        tc.xfrm,
				}).Register(reg)
				assert.ErrIsLike(t, reg.Initialize(t.Context(), nil), tc.wantErr)
			})
		}
	})

	// When we only have 1 SingleFile, a nil TransformFunc turns into a
	// passthrough.
	t.Run("nil default", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}
		f := &SingleFile[*durationpb.Duration]{Path: "some.cfg"}
		(&ServicePipeline[*durationpb.Duration]{
			ID:          "test",
			ServiceName: "my-app",
			Files:       []File{f},
		}).Register(reg)
		assert.NoErr(t, reg.Initialize(t.Context(), nil))

		snap := &Snapshot{singleFiles: make(map[File]singleFileEntry)}
		f.recordInSnapshot(snap, "some.cfg", &durationpb.Duration{Seconds: 99})
		got, err := reg.ingestConfigs["test"].transformFunc(nil, snap)
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match(&durationpb.Duration{Seconds: 99}))
	})

	t.Run("non-nil", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}
		f := &SingleFile[*durationpb.Duration]{Path: "some.cfg"}
		(&ServicePipeline[*timestamppb.Timestamp]{
			ID:               "test",
			ServiceName:      "my-app",
			Files:            []File{f},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*timestamppb.Timestamp, error) {
				return &timestamppb.Timestamp{Seconds: f.Get(snap).Seconds * 2}, nil
			},
		}).Register(reg)
		assert.NoErr(t, reg.Initialize(t.Context(), nil))

		snap := &Snapshot{singleFiles: make(map[File]singleFileEntry)}
		f.recordInSnapshot(snap, "some.cfg", &durationpb.Duration{Seconds: 21})
		got, err := reg.ingestConfigs["test"].transformFunc(nil, snap)
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match(&timestamppb.Timestamp{Seconds: 42}))
	})
}
