// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type swInfo struct {
	*Swarming
	userPayload *swarmingpb.CASTree
}

var _ Info = swInfo{}

func (s swInfo) SwarmingHostname() string {
	return s.GetHostname()
}

func (s swInfo) TaskName() string {
	return s.GetTask().GetName()
}

func (s swInfo) CurrentIsolated() (*swarmingpb.CASTree, error) {
	panic("implement me")
}

func (s swInfo) Env() (ret map[string]string, err error) {
	panic("implement me")
}

func (s swInfo) Priority() int32 {
	return s.GetTask().GetPriority()
}

func (s swInfo) PrefixPathEnv() (ret []string, err error) {
	panic("implement me")
}

func (s swInfo) Tags() (ret []string) {
	panic("implement me")
}
