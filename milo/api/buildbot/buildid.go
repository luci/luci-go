// Copyright 2018 The LUCI Authors.
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

package buildbot

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
)

// BuildID identifies a buildbot build.
type BuildID struct {
	Master  string
	Builder string
	Number  int
}

// Validate returns an error if id is invalid.
func (id BuildID) Validate() error {
	switch {
	case id.Master == "":
		return errors.New("master is unspecified", grpcutil.InvalidArgumentTag)
	case id.Builder == "":
		return errors.New("builder is unspecified", grpcutil.InvalidArgumentTag)
	case id.Number < 0:
		return errors.New("nunber must be >= 0", grpcutil.InvalidArgumentTag)
	default:
		return nil
	}
}

// String returns a string "{master}/{builder}/{number}".
func (id BuildID) String() string {
	return fmt.Sprintf("%s/%s/%d", id.Master, id.Builder, id.Number)
}
