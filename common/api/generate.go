// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate go install github.com/luci/luci-go/tools/cmd/apigen
//go:generate apigen -api-subproject "luci_config" -service "https://luci-config.appspot.com" -api "config:v1"
//go:generate apigen -api-subproject "swarming" -service "https://chromium-swarm.appspot.com" -api "swarming:v1"
//go:generate apigen -api-subproject "isolate" -service "https://isolateserver.appspot.com" -api "isolateservice:v2"

package api
