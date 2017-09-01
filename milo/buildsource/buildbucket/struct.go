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
	// Key is formulated via <host>:<build ID>.
	Key string `gae:"$id"`

	// BuildbucketData is the json marshalled form of
	// a bucketApi.ApiCommonBuildMessage message.
	BuildbucketData []byte `gae:",noindex"`

	// Project is the luci project name of the build.
	Project string

	// Created is the time when this build entry was first created.
	Created time.Time

	// Modified is the time when this build entry was last modified.
	Modified time.Time

	// respBuild is the resp.MiloBuild format of the build.  This is
	// not stored in datastore.
	respBuild *resp.MiloBuild
}

// buildEntryKey returns the key for a build entry given a hostname and build ID.
func buildEntryKey(host string, buildID int64) string {
	return fmt.Sprintf("%s:%d", host, buildID)
}

func (b *buildEntry) getBuild() (*bucketApi.ApiCommonBuildMessage, error) {
	msg := bucketApi.ApiCommonBuildMessage{}
	err := json.Unmarshal(b.BuildbucketData, &msg)
	return &msg, err
}
