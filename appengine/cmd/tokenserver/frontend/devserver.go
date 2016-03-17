// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	cfg "github.com/luci/luci-go/common/config/impl/memory"
)

// devServerConfig returns mocked configs to use locally on dev server.
func devServerConfigs(appID string) map[string]cfg.ConfigSet {
	// TODO(vadimsh): Add stuff.
	return map[string]cfg.ConfigSet{}
}
