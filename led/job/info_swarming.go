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
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type swInfo struct {
	*Swarming
	casUserPayload *swarmingpb.CASReference
}

var _ Info = swInfo{}

func (s swInfo) SwarmingHostname() string {
	return s.GetHostname()
}

func (s swInfo) TaskName() string {
	return s.GetTask().GetName()
}

func (s swInfo) CurrentIsolated() (*isolated, error) {
	isolated := &isolated{}
	var err error
	if isolated.CASReference, err = s.currentCasInfo(); err != nil {
		return nil, err
	}
	return isolated, nil
}

func (s swInfo) currentCasInfo() (*swarmingpb.CASReference, error) {
	casOptions := map[string]*swarmingpb.CASReference{}
	if p := s.casUserPayload; p.GetDigest().GetHash() != "" {
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
		return nil, errors.Reason(
			"Definition contains multiple RBE-CAS inputs: %v", casOptions).Err()
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
		if err := slc.Expiration.CheckValid(); err != nil {
			return nil, errors.Annotate(err, "malformed expiration").Err()
		}
		exp := slc.Expiration.AsDuration()
		totalExpiration += exp

		for _, dim := range slc.GetProperties().GetDimensions() {
			for _, val := range dim.Values {
				ldims.updateDuration(dim.Key, val, totalExpiration)
			}
		}
	}
	return ldims.toExpiringDimensions(), nil
}

func (s swInfo) CIPDPkgs() (ret CIPDPkgs, err error) {
	slices := s.GetTask().GetTaskSlices()
	if len(slices) >= 1 {
		if pkgs := slices[0].GetProperties().GetCipdInputs(); len(pkgs) > 0 {
			ret = CIPDPkgs{}
			ret.fromList(pkgs)
		}
	}
	if len(slices) > 1 {
		for idx, slc := range slices[1:] {
			pkgDict := CIPDPkgs{}
			pkgDict.fromList(slc.GetProperties().GetCipdInputs())
			if !ret.equal(pkgDict) {
				return nil, errors.Reason(
					"slice %d has cipd pkgs which differ from slice 0: %v vs %v",
					idx+1, pkgDict, ret).Err()
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
				return nil, errors.Reason(
					"slice %d has env which differs from slice 0: %v vs %v",
					idx+1, slcEnv, ret).Err()
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
		for _, keyVals := range slices[0].GetProperties().GetEnvPaths() {
			if keyVals.Key == "PATH" {
				ret = make([]string, len(keyVals.Values))
				copy(ret, keyVals.Values)
				break
			}
		}
	}
	if len(slices) > 1 {
		for idx, slc := range slices[1:] {
			foundIt := false
			for _, keyVal := range slc.GetProperties().GetEnvPaths() {
				if keyVal.Key == "PATH" {
					foundIt = true
					if !reflect.DeepEqual(ret, keyVal.Values) {
						return nil, errors.Reason(
							"slice %d has $PATH env prefixes which differ from slice 0: %v vs %v",
							idx+1, keyVal.Values, ret).Err()
					}
					break
				}
			}
			if !foundIt && len(ret) > 0 {
				return nil, errors.Reason(
					"slice %d has $PATH env prefixes which differ from slice 0: %v vs %v",
					idx+1, []string{}, ret).Err()
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
