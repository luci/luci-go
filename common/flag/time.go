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

package flag

import (
	"flag"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
)

// Time returns a flag.Getter which parses a string into a time.Time pointer.
//
// The timestamp to parse must be formatted as stiptime:
// https://chromium.googlesource.com/infra/infra/infra_libs/+/b2e2c9948c327b88b138d8bd60ec4bb3a957be78/time_functions/README.md
//
// Caveat: Leap seconds are not supported due to a limitation in go's time
// library: https://github.com/golang/go/issues/15247
func Time(t *time.Time) flag.Getter {
	return (*timeFlag)(t)
}

type timeFlag time.Time

func (f *timeFlag) String() string {
	return time.Time(*f).String()
}

func (f *timeFlag) Get() any {
	return time.Time(*f)
}

func (f *timeFlag) Set(v string) error {
	if !strings.HasSuffix(v, "Z") {
		return errors.New("set time flag: timestamp must end with 'Z'")
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return errors.Fmt("set time flag: %w", err)
	}
	*f = timeFlag(t)
	return nil
}
