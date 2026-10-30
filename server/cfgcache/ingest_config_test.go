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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
)

func TestIngestConfigRegistrationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		register func(reg *Registry)
		wantErr  string
	}{
		{
			name: "empty ID",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `ID must not be empty`,
		},
		{
			name: "negative TransformVersion",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID:               "svc",
					Files:            []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					TransformVersion: -1,
				}).Register(reg)
			},
			wantErr: `TransformVersion must not be negative`,
		},
		{
			name: "duplicate ID",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID:    "dup",
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "a.cfg"}},
				}).Register(reg)
				(&ProjectPipeline[*durationpb.Duration]{
					ID:    "dup",
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "b.cfg"}},
				}).Register(reg)
			},
			wantErr: `duplicate ID "dup"`,
		},
		{
			name: "invalid RegexFile pattern",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID: "svc",
					Files: []File{
						&RegexFile[*durationpb.Duration]{PathRegex: `[unclosed`},
					},
					TransformVersion: 1,
					Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
						return &durationpb.Duration{}, nil
					},
				}).Register(reg)
			},
			wantErr: `bad PathRegex "[unclosed"`,
		},
		{
			name: "unresolvable variable in ServiceName",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID:          "svc",
					ServiceName: "${bad_svc}",
					Files:       []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `bad serviceNamePattern: no placeholder named "bad_svc" is registered`,
		},
		{
			name: "unresolvable variable in Files path",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID:    "svc",
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "${bad_path}.cfg"}},
				}).Register(reg)
			},
			wantErr: `Files[0]: bad path template: no placeholder named "bad_path" is registered`,
		},
		{
			name: "invalid ServiceName",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration]{
					ID:          "svc",
					ServiceName: "Bad/Service",
					Files:       []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `bad ServiceName: invalid service name "Bad/Service"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, ts := TestingContext(t.Context())
			tc.register(ts.Registry)
			assert.ErrIsLike(t, ts.Initialize(ctx), tc.wantErr)
		})
	}
}

func TestTestingStateSetConfig(t *testing.T) {
	t.Parallel()

	cs := config.Set("services/my-app")

	cases := []struct {
		name    string
		path    string
		val     any
		wantSub string
	}{
		{
			name:    "string",
			path:    "str.txt",
			val:     "hello",
			wantSub: "hello",
		},
		{
			name:    "[]byte",
			path:    "raw.bin",
			val:     []byte("world"),
			wantSub: "world",
		},
		{
			name:    "proto.Message",
			path:    "msg.cfg",
			val:     &durationpb.Duration{Seconds: 42},
			wantSub: "seconds",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ts := TestingContext(t.Context())
			ts.SetConfig(cs, tc.path, tc.val)
			assert.That(t, ts.CfgFiles[cs][tc.path], should.ContainSubstring(tc.wantSub))
		})
	}

	t.Run("nil deletes existing config", func(t *testing.T) {
		t.Parallel()
		_, ts := TestingContext(t.Context())
		ts.SetConfig(cs, "str.txt", "hello")
		ts.SetConfig(cs, "str.txt", nil)
		_, ok := ts.CfgFiles[cs]["str.txt"]
		assert.That(t, ok, should.BeFalse)
	})
}

func TestRegistryLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("double Initialize returns error", func(t *testing.T) {
		t.Parallel()
		ctx, ts := TestingContext(t.Context())
		assert.NoErr(t, ts.Initialize(ctx))
		assert.ErrIsLike(t, ts.Initialize(ctx), "registry already initialized")
	})

	t.Run("stateless descriptors can be registered across multiple registries", func(t *testing.T) {
		t.Parallel()
		f := &SingleFile[*durationpb.Duration]{Path: "${appid}.cfg"}
		svcCfg := &ServicePipeline[*durationpb.Duration]{ID: "svc", Files: []File{f}}
		projCfg := &ProjectPipeline[*durationpb.Duration]{ID: "proj", Files: []File{f}}

		ctx1, ts1 := TestingContext(t.Context())
		svcCfg.Register(ts1.Registry)
		projCfg.Register(ts1.Registry)
		assert.NoErr(t, ts1.Initialize(ctx1))

		ctx2, ts2 := TestingContext(t.Context())
		svcCfg.Register(ts2.Registry)
		projCfg.Register(ts2.Registry)
		assert.NoErr(t, ts2.Initialize(ctx2))
	})

	t.Run("String formatting", func(t *testing.T) {
		t.Parallel()
		f := &SingleFile[*durationpb.Duration]{Path: "neat.cfg"}
		svcDesc := &ServicePipeline[*durationpb.Duration]{ID: "svc-neat", Files: []File{f}}
		projDesc := &ProjectPipeline[*durationpb.Duration]{ID: "proj-multi", Files: []File{f}}
		assert.That(t, svcDesc.String(), should.Equal(`ServicePipeline{ID: "svc-neat", ServiceName: "${appid}"}`))
		assert.That(t, projDesc.String(), should.Equal(`ProjectPipeline{ID: "proj-multi"}`))
	})
}
