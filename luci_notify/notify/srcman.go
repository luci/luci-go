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

package notify

import (
	"net/http"

	"go.chromium.org/luci/common/proto/srcman"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/types"
)

func getSourceManifest(c context.Context, build *Build) (*srcman.Manifest, error) {
	client := coordinator.NewClient(&prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	})
	qo := coordinator.QueryOptions{
		ContentType: srcman.SourceManifestContentType,
	}
	err = client.Query(c, project, path, qo, func(s *coordinator.LogStream) bool {

	})
	if err == nil {
		return nil, err
	}
	f := coord.Stream(addr.Project, addr.Path).Fetcher(c, &fetcher.Options{
		Index:       types.MessageIndex(cmd.index),
		Count:       cmd.count,
		BufferCount: cmd.fetchSize,
		BufferBytes: int64(cmd.fetchBytes),
	})
}
