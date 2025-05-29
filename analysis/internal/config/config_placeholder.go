// Copyright 2022 The LUCI Authors.
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

package config

import (
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/errors"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

var sampleConfigStr = `
	monorail_hostname: "monorail-test.appspot.com"
	chunk_gcs_bucket: "my-chunk-bucket"
	reclustering_workers: 50
	reclustering_interval_minutes: 5
`

// CreatePlaceholderConfig returns a new valid Config for testing.
func CreatePlaceholderConfig() (*configpb.Config, error) {
	var cfg configpb.Config
	err := prototext.Unmarshal([]byte(sampleConfigStr), &cfg)
	if err != nil {
		return nil, errors.Fmt("Marshaling a test config: %w", err)
	}
	return &cfg, nil
}
