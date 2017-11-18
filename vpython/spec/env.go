// Copyright 2017 The LUCI Authors.
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

package spec

import (
	"io/ioutil"
	"sort"

	"go.chromium.org/luci/vpython/api/vpython"

	"go.chromium.org/luci/common/errors"

	"github.com/golang/protobuf/proto"
)

// LoadEnvironment loads an environment file text protobuf from the supplied
// path.
func LoadEnvironment(path string, environment *vpython.Environment) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Annotate(err, "failed to load file from: %s", path).Err()
	}

	return ParseEnvironment(string(content), environment)
}

// ParseEnvironment loads a environment protobuf message from a content string.
func ParseEnvironment(content string, environment *vpython.Environment) error {
	if err := proto.UnmarshalText(content, environment); err != nil {
		return errors.Annotate(err, "failed to unmarshal vpython.Environment").Err()
	}
	return nil
}

// NormalizeEnvironment normalizes the supplied Environment such that two
// messages with identical meaning will have identical representation.
func NormalizeEnvironment(env *vpython.Environment) error {
	if env.Spec == nil {
		env.Spec = &vpython.Spec{}
	}
	if err := NormalizeSpec(env.Spec, env.Pep425Tag); err != nil {
		return err
	}

	if env.Runtime == nil {
		env.Runtime = &vpython.Runtime{}
	}

	sort.Sort(pep425TagSlice(env.Pep425Tag))
	return nil
}
