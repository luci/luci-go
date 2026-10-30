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
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/vars"
)

// TestingState holds testing helpers returned by [TestingContext].
type TestingState struct {
	*Registry

	// Vars is the placeholder variable set used by [TestingState.Initialize].
	//
	// Pre-populated with `${appid}` = `my-app`.
	Vars *vars.VarSet

	// The active set of configuration files reflected via the in-context
	// cfgclient.
	CfgFiles map[config.Set]cfgmem.Files
}

// Initialize freezes registration and resolves template variables using s.Vars.
func (s *TestingState) Initialize(ctx context.Context) error {
	return s.Registry.Initialize(ctx, s.Vars)
}

// SetConfig sets or deletes (if data is nil) a config file in CfgFiles.
//
// If `data` is a string or []byte, it's used verbatim.
//
// If `data` is a proto message, this will marshal it as prototext.
//
// Otherwise, this will panic.
func (s *TestingState) SetConfig(cs config.Set, path string, data any) {
	cur := s.CfgFiles[cs]
	if cur == nil {
		cur = cfgmem.Files{}
		s.CfgFiles[cs] = cur
	}
	if data == nil {
		delete(cur, path)
	} else {
		switch x := data.(type) {
		case []byte:
			cur[path] = string(x)
		case string:
			cur[path] = x
		case proto.Message:
			cur[path] = prototext.Format(x)
		default:
			panic(fmt.Sprintf("cfgcache.TestingState.SetConfig: expected nil/[]byte/string/proto.Message, got %T", data))
		}
	}
}

// TestingContext installs an in-memory cfgclient into ctx for unit tests that
// do not initialize the server module via server.Main, and returns a
// [*TestingState].
//
// Sets `${appid}` to `my-app` in the returned [TestingState.Vars].
//
// Exposed for testing.
func TestingContext(ctx context.Context) (context.Context, *TestingState) {
	vs := &vars.VarSet{}
	vs.Register("appid", func(ctx context.Context) (string, error) {
		return "my-app", nil
	})
	cfgFiles := make(map[config.Set]cfgmem.Files)
	ctx = cfgclient.Use(ctx, cfgmem.New(cfgFiles))

	return ctx, &TestingState{
		Registry: &Registry{},
		Vars:     vs,
		CfgFiles: cfgFiles,
	}
}
