// Copyright 2015 The LUCI Authors.
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

package cmpbin

import (
	"errors"
	"flag"
	"log"
	"time"
)

var seed = flag.Int64("cmpbin.seed", time.Now().UnixNano(), "random seed for testing")

var randomTestSize = 1000

func init() {
	flag.Parse()
	log.Println("cmpbin.seed =", *seed)
}

type fakeWriter struct{ count int }

func (f *fakeWriter) WriteByte(byte) error {
	if f.count == 0 {
		return errors.New("nope")
	}
	f.count--
	return nil
}
