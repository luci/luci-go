// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
)

func makeUnifiedPath(project config.ProjectName, path types.StreamPath) string {
	val := string(project)
	if path != "" {
		val += types.StreamNameSepStr + string(path)
	}
	return val
}
