// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"encoding/json"
	"fmt"
	"time"

	bucketApi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/milo/api/resp"
)

// buildEntry is a full buildbucket build along with its full resp rendering
// at the time of modification.  This is a parent of a BuildSummary.
type buildEntry struct {
	// key is formulated via <project ID>:<build ID>.  From PubSub, project ID
	// is determined via the topic name.
	key string `gae:"$id"`

	// buildData is the json marshalled form of
	// a bucketApi.ApiCommonBuildMessage message.
	buildbucketData []byte `gae:",noindex"`

	// respBuild is the resp.MiloBuild representation of the build.
	respBuild *resp.MiloBuild `gae:",noindex"`

	// project is the luci project name of the build.
	project string

	// created is the time when this build entry was first created.
	created time.Time

	// last is the time when this build entry was last modified.
	modified time.Time
}

// buildEntryKey returns the key for a build entry given a hostname and build ID.
func buildEntryKey(host string, buildID int64) string {
	return fmt.Sprintf("%s:%d", host, buildID)
}

func (b *buildEntry) getBuild() (*bucketApi.ApiCommonBuildMessage, error) {
	msg := bucketApi.ApiCommonBuildMessage{}
	err := json.Unmarshal(b.buildbucketData, &msg)
	return &msg, err
}
