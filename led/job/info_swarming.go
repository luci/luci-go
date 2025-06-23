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
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

type swInfo struct {
	*Swarming
}

var _ Info = swInfo{}

func (s swInfo) SwarmingHostname() string {
	return s.GetHostname()
}

func (s swInfo) TaskName() string {
	return s.GetTask().GetName()
}

func (s swInfo) CurrentIsolated() (*swarmingpb.CASReference, error) {
	casOptions := map[string]*swarmingpb.CASReference{}
	if p := s.GetCasUserPayload(); p.GetDigest().GetHash() != "" {
		casOptions[p.Digest.Hash] = p
	}

	if sw := s.Swarming; sw != nil {
		for _, slc := range sw.GetTask().GetTaskSlices() {
			input := slc.GetProperties().GetCasInputRoot()
			if input != nil {
				casOptions[input.Digest.GetHash()] = input
			}
		}
	}
	if len(casOptions) > 1 {
		return nil, errors.Fmt("Definition contains multiple RBE-CAS inputs: %v", casOptions)
	}
	for _, v := range casOptions {
		return proto.Clone(v).(*swarmingpb.CASReference), nil
	}
	return nil, nil
}

func (s swInfo) Dimensions() (ExpiringDimensions, error) {
	ldims := logicalDimensions{}
	var totalExpiration time.Duration
	for _, slc := range s.GetTask().GetTaskSlices() {
		exp := time.Duration(slc.ExpirationSecs) * time.Second
		totalExpiration += exp

		for _, dim := range slc.GetProperties().GetDimensions() {
			ldims.updateDuration(dim.Key, dim.Value, totalExpiration)
		}
	}
	return ldims.toExpiringDimensions(), nil
}

func (s swInfo) CIPDPkgs() (ret CIPDPkgs, err error) {
	slices := s.GetTask().GetTaskSlices()
	if len(slices) >= 1 {
		if pkgs := slices[0].GetProperties().GetCipdInput().GetPackages(); len(pkgs) > 0 {
			ret = CIPDPkgs{}
			ret.fromList(pkgs)
		}
	}
	if len(slices) > 1 {
		for idx, slc := range slices[1:] {
			pkgDict := CIPDPkgs{}
			pkgDict.fromList(slc.GetProperties().GetCipdInput().GetPackages())
			if !ret.equal(pkgDict) {
				return nil, errors.Fmt("slice %d has cipd pkgs which differ from slice 0: %v vs %v",
					idx+1, pkgDict, ret)
			}
		}
	}
	return
}

func (s swInfo) Env() (ret map[string]string, err error) {
	slices := s.GetTask().GetTaskSlices()
	extractEnv := func(slc *swarmingpb.TaskSlice) (slcEnv map[string]string) {
		if env := slices[0].GetProperties().GetEnv(); len(env) > 0 {
			slcEnv = make(map[string]string, len(env))
			for _, pair := range env {
				slcEnv[pair.Key] = pair.Value
			}
		}
		return
	}

	if len(slices) >= 1 {
		ret = extractEnv(slices[0])

		for idx, slc := range slices[1:] {
			if slcEnv := extractEnv(slc); !reflect.DeepEqual(ret, slcEnv) {
				return nil, errors.Fmt("slice %d has env which differs from slice 0: %v vs %v",
					idx+1, slcEnv, ret)
			}
		}
	}
	return
}

func (s swInfo) Priority() int32 {
	return s.GetTask().GetPriority()
}

func (s swInfo) PrefixPathEnv() (ret []string, err error) {
	slices := s.GetTask().GetTaskSlices()
	if len(slices) >= 1 {
		for _, keyVals := range slices[0].GetProperties().GetEnvPrefixes() {
			if keyVals.Key == "PATH" {
				ret = make([]string, len(keyVals.Value))
				copy(ret, keyVals.Value)
				break
			}
		}
	}
	if len(slices) > 1 {
		for idx, slc := range slices[1:] {
			foundIt := false
			for _, keyVal := range slc.GetProperties().GetEnvPrefixes() {
				if keyVal.Key == "PATH" {
					foundIt = true
					if !reflect.DeepEqual(ret, keyVal.Value) {
						return nil, errors.Fmt("slice %d has $PATH env prefixes which differ from slice 0: %v vs %v",
							idx+1, keyVal.Value, ret)
					}
					break
				}
			}
			if !foundIt && len(ret) > 0 {
				return nil, errors.Fmt("slice %d has $PATH env prefixes which differ from slice 0: %v vs %v",
					idx+1, []string{}, ret)
			}
		}
	}
	return
}

func (s swInfo) Tags() (ret []string) {
	if tags := s.GetTask().GetTags(); len(tags) > 0 {
		ret = make([]string, len(tags))
		copy(ret, tags)
	}
	return
}
