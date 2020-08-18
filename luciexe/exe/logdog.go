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

package exe

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	ldtypes "go.chromium.org/luci/logdog/common/types"
)

// ldPrep generates a Log entry on the Build or Step, checking for duplicate
// logs.
//
// Used by Build.Log* and Step.Log*
func ldPrep(ctx context.Context, nameToks []string, logManipulator func(func(logs *[]*bbpb.Log) error) error) (ldtypes.StreamName, error) {
	name := nameToks[len(nameToks)-1]

	fullName, err := ldtypes.MakeStreamName("_", nameToks...)
	if err != nil {
		return "", errors.Annotate(err, "making streamname: %q", nameToks).Err()
	}

	err = logManipulator(func(logs *[]*bbpb.Log) error {
		for _, log := range *logs {
			if log.Name == name {
				return errors.Reason("duplicate logname %q", name).Err()
			}
		}
		*logs = append(*logs, &bbpb.Log{
			Name: name,
			Url:  string(fullName), // see luciexe docs re: Url
		})
		return nil
	})
	return fullName, err
}
