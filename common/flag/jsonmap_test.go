// Copyright 2019 The LUCI Authors.
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
	"io/ioutil"
	"reflect"
	"testing"
)

// Check that a JSON object string gets parsed correctly using JSONMap.
func TestParseJSONMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		arg      string
		expected map[string]string
	}{
		{`{"foo": "bar"}`, map[string]string{"foo": "bar"}},
	}
	for _, c := range cases {
		var m map[string]string
		fs := testFlagSet()
		fs.Var(JSONMap(&m), "map", "Some map")
		if err := fs.Parse([]string{"-map", c.arg}); err != nil {
			t.Errorf("Parse returned an error for %s: %s", c.arg, err)
			continue
		}
		if !reflect.DeepEqual(m, c.expected) {
			t.Errorf("Parsing %s, got %#v, expected %#v", c.arg, m, c.expected)
		}
	}
}

func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Usage = func() {}
	fs.SetOutput(ioutil.Discard)
	return fs
}
