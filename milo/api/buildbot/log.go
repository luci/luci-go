// Copyright 2016 The LUCI Authors.
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
	"encoding/json"

	"go.chromium.org/luci/common/errors"
)

type Log struct {
	Name, URL string
}

func (l *Log) MarshalJSON() ([]byte, error) {
	buildbotFormat := []string{l.Name, l.URL}
	return json.Marshal(buildbotFormat)
}

func (l *Log) UnmarshalJSON(data []byte) error {
	var buildbotFormat []string
	err := json.Unmarshal(data, &buildbotFormat)
	if err != nil {
		return err
	}
	if len(buildbotFormat) != 2 {
		return errors.Reason("unexpected length of array %q, want 2", len(buildbotFormat)).Err()
	}
	l.Name = buildbotFormat[0]
	l.URL = buildbotFormat[1]
	return nil
}
