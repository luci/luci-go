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

// Date returns a flag.Getter which parses the flag value as a UTC date.
// The string must be a time with layout "2006-01-24".
func Date(t *time.Time) flag.Getter {
	return TimeLayout(t, "2006-01-02")
}

// TimeLayout returns a flag.Getter which parses the flag value as a UTC time,
// based on the given layout.
func TimeLayout(t *time.Time, layout string) flag.Getter {
	return &timeLayoutFlag{ptr: t, layout: layout}
}

type timeLayoutFlag struct {
	ptr    *time.Time
	layout string
}

func (f *timeLayoutFlag) String() string {
	// flag.Value implementations must work for zero values.
	if f.ptr == nil || f.ptr.IsZero() {
		return ""
	}
	return f.ptr.Format(f.layout)
}

func (f *timeLayoutFlag) Get() any {
	return *f.ptr
}

func (f *timeLayoutFlag) Set(v string) error {
	t, err := time.Parse(f.layout, v)
	if err != nil {
		return err
	}
	*f.ptr = t
	return nil
}
