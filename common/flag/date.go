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
	"time"
)

const dateLayout = "2006-01-02"

// Date returns a flag.Getter which parses the flag value as a UTC date.
// The string must be a time with layout "2006-01-02".
func Date(t *time.Time) flag.Getter {
	return (*dateFlag)(t)
}

type dateFlag time.Time

func (f *dateFlag) String() string {
	return time.Time(*f).Format(dateLayout)
}

func (f *dateFlag) Get() interface{} {
	return time.Time(*f)
}

func (f *dateFlag) Set(v string) error {
	t, err := time.Parse(dateLayout, v)
	if err != nil {
		return err
	}
	*f = dateFlag(t)
	return nil
}
