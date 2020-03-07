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

package job

import (
	"go.chromium.org/luci/common/errors"
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
	isolatedOptions := map[string]*swarmingpb.CASTree{}
	if up := s.userPayload; up != nil {
		isolatedOptions[up.Digest] = up
	}

	if sw := s.Swarming; sw != nil {
		for _, slc := range sw.GetTask().GetTaskSlices() {
			input := slc.GetProperties().GetCasInputs()
			if input != nil {
				isolatedOptions[input.Digest] = input
			}
		}
	}
	if len(isolatedOptions) > 1 {
		return nil, errors.Reason(
			"Definition contains multiple isolateds: %v", isolatedOptions).Err()
	}
	for _, v := range isolatedOptions {
		return v, nil
	}
	return nil, nil
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
