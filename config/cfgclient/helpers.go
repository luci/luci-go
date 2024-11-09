// Copyright 2020 The LUCI Authors.
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

package cfgclient

import (
	"context"
	"sort"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	luciProto "go.chromium.org/luci/common/proto"

	"go.chromium.org/luci/config"
)

// Destination receives a raw config file body, deserializes and stores it.
//
// See Get(...).
type Destination func(string) error

// Bytes can be used to put the received config body into a byte slice.
func Bytes(out *[]byte) Destination {
	return func(body string) error {
		*out = []byte(body)
		return nil
	}
}

// String can be used to put the received config body into a string.
func String(out *string) Destination {
	return func(body string) error {
		*out = body
		return nil
	}
}

// ProtoText can be used to deserialize the received config body as a text proto
// message.
func ProtoText(msg proto.Message) Destination {
	return func(body string) error {
		return luciProto.UnmarshalTextML(body, msg)
	}
}

// ProtoJSON can be used to deserialize the received config body as a JSONPB
// message.
func ProtoJSON(msg proto.Message) Destination {
	return func(body string) error {
		return jsonpb.UnmarshalString(body, msg)
	}
}

// Get fetches and optionally deserializes a single config file.
//
// It is a convenience wrapper over Client(ctx).GetConfig(...). If you need
// something more advanced, use the client directly.
//
// If `dest` is given, it is used to deserialize and store the fetched config.
// If it is nil, the config body is ignored (but its metadata is still fetched).
//
// If `meta` is given, it receives the metadata about the config.
//
// Returns an error if the config can't be fetched or deserialized.
func Get(ctx context.Context, cs config.Set, path string, dest Destination, meta *config.Meta) error {
	cfg, err := Client(ctx).GetConfig(ctx, cs, path, dest == nil)
	if err != nil {
		return err
	}
	if dest != nil {
		if err := dest(cfg.Content); err != nil {
			return err
		}
	}
	if meta != nil {
		*meta = cfg.Meta
	}
	return nil
}

// ProjectsWithConfig returns a list of LUCI projects that have the given
// configuration file.
//
// It is a convenience wrapper over Client(ctx).GetProjectConfigs(...). If you
// need something more advanced, use the client directly.
func ProjectsWithConfig(ctx context.Context, path string) ([]string, error) {
	cfgs, err := Client(ctx).GetProjectConfigs(ctx, path, true)
	if err != nil {
		return nil, err
	}
	projects := make([]string, 0, len(cfgs))
	for _, cfg := range cfgs {
		if proj := cfg.ConfigSet.Project(); proj != "" {
			projects = append(projects, proj)
		}
	}
	sort.Strings(projects)
	return projects, nil
}
