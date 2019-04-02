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

package normalize

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	pb "go.chromium.org/luci/milo/api/config"
)

// Milo normalizes luci-milo.cfg config.
func Milo(c context.Context, cfg *pb.Project) error {
	// Sort consoles by ID.
	sort.Slice(cfg.Consoles, func(i, j int) bool {
		return cfg.Consoles[i].Id < cfg.Consoles[j].Id
	})

	// Inline headers.
	headers := make(map[string]*pb.Header)
	for _, h := range cfg.Headers {
		headers[h.Id] = h
	}
	for _, c := range cfg.Consoles {
		switch {
		case c.HeaderId == "":
			continue
		case c.Header != nil:
			return fmt.Errorf("bad console %q - has both 'header' and 'header_id' fields", c.Id)
		default:
			c.Header = headers[c.HeaderId]
		}
	}
	cfg.Headers = nil

	// Normalize and sort refs.
	for _, c := range cfg.Consoles {
		for i, r := range c.Refs {
			if !strings.HasPrefix(r, "regexp:") {
				c.Refs[i] = "regexp:" + regexp.QuoteMeta(r)
			}
		}
		sort.Strings(c.Refs)
	}

	// Sort alternative builder names, but not builders themselves! Their order is
	// significant for Milo UI.
	for _, c := range cfg.Consoles {
		for _, b := range c.Builders {
			sort.Strings(b.Name)
		}
	}

	return nil
}
