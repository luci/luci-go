// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	cfg "github.com/luci/luci-go/common/config/impl/memory"
)

const project1CronCfg = `
job {
  id: "swarming-job"
  schedule: "* * * 1 * * *"
  task: {
    swarming_task: {
      server: "https://chromium-swarm-dev.appspot.com"

      command: "echo"
      command: "Hello, world"

      dimensions: "os:Ubuntu"
    }
  }
}

job {
  id: "some-really-really-long-job-name-to-test-the-table"
  schedule: "0 0,10,20,30,40,50 * * * * *"
  task: {
    noop: {}
  }
}
`

// devServerConfig returns mocked luci-config configs to use locally on dev
// server (to avoid talking to real luci-config service).
func devServerConfigs() map[string]cfg.ConfigSet {
	return map[string]cfg.ConfigSet{
		"projects/project1": {
			"cron.cfg": project1CronCfg,
		},
	}
}
