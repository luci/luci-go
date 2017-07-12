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

package isolatedclient

import (
	"errors"
	"flag"
	"net/http/httptest"
	"os"

	"github.com/luci/luci-go/common/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/lhttp"
)

// Flags contains values parsed from command line arguments.
type Flags struct {
	ServerURL string
	Namespace string
}

// Init registers flags in a given flag set.
func (c *Flags) Init(f *flag.FlagSet) {
	i := os.Getenv("ISOLATE_SERVER")
	f.StringVar(&c.ServerURL, "isolate-server", i,
		"Isolate server to use; defaults to value of $ISOLATE_SERVER; use special value 'fake' to use a fake server")
	f.StringVar(&c.ServerURL, "I", i, "Alias for -isolate-server")
	f.StringVar(&c.Namespace, "namespace", DefaultNamespace, "")
}

// Parse applies changes specified by command line flags.
func (c *Flags) Parse() error {
	if c.ServerURL == "" {
		return errors.New("-isolate-server must be specified")
	}
	if c.ServerURL == "fake" {
		ts := httptest.NewServer(isolatedfake.New())
		c.ServerURL = ts.URL
	} else {
		s, err := lhttp.CheckURL(c.ServerURL)
		if err != nil {
			return err
		}
		c.ServerURL = s
	}
	if c.Namespace == "" {
		return errors.New("-namespace must be specified")
	}
	return nil
}
