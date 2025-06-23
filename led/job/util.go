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
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func keysOf(mapish any) []string {
	mapV := reflect.ValueOf(mapish)
	if mapV.Kind() != reflect.Map {
		panic("keysOf expected a map")
	}
	keys := []string{}
	for _, key := range mapV.MapKeys() {
		if key.Kind() != reflect.String {
			panic("keysOf expected a map with string keys")
		}
		keys = append(keys, key.String())
	}
	sort.Strings(keys)
	return keys
}

func updateStringPairList(list *[]*swarmingpb.StringPair, updates map[string]string) {
	if len(updates) == 0 {
		return
	}

	current := make(map[string]string, len(*list))
	for _, pair := range *list {
		current[pair.Key] = pair.Value
	}
	for key, value := range updates {
		if value == "" {
			delete(current, key)
		} else {
			current[key] = value
		}
	}
	newList := make([]*swarmingpb.StringPair, 0, len(current))
	for key, value := range current {
		newList = append(newList, &swarmingpb.StringPair{Key: key, Value: value})
	}
	*list = newList
}

const (
	casInstanceTemplate = "projects/%s/instances/default_instance"
)

var (
	swarmingHostRx = regexp.MustCompile(`(.*)\.appspot\.com`)
)

// ToCasInstance converts a swarming host name to cas instance name.
func ToCasInstance(swarmingHost string) (string, error) {
	match := swarmingHostRx.FindStringSubmatch(swarmingHost)
	if match == nil {
		return "", errors.New(fmt.Sprintf("invalid swarming host in job definition host=%s", swarmingHost))
	}
	return fmt.Sprintf(casInstanceTemplate, match[1]), nil
}

// ToCasDigest converts a string (in the format of "hash/size", e.g. "dead...beef/1234") to cas digest.
func ToCasDigest(str string) (*swarmingpb.Digest, error) {
	digest := &swarmingpb.Digest{}
	var err error
	switch strs := strings.Split(str, "/"); {
	case len(strs) == 2:
		digest.Hash = strs[0]
		if digest.SizeBytes, err = strconv.ParseInt(strs[1], 10, 64); err != nil {
			return nil, err
		}
	default:
		err = errors.Fmt("Invalid RBE-CAS digest %s", str)
		return nil, err
	}
	return digest, nil
}
