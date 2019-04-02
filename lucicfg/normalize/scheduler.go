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

	"go.chromium.org/luci/common/data/stringset"

	pb "go.chromium.org/luci/scheduler/appengine/messages"
)

// Scheduler normalizes luci-scheduler.cfg config.
func Scheduler(c context.Context, cfg *pb.ProjectConfig) error {
	// Sort jobs by ID.
	sort.Slice(cfg.Job, func(i, j int) bool {
		return cfg.Job[i].Id < cfg.Job[j].Id
	})
	sort.Slice(cfg.Trigger, func(i, j int) bool {
		return cfg.Trigger[i].Id < cfg.Trigger[j].Id
	})

	// ACL set name => list of ACLs.
	aclSets := make(map[string][]*pb.Acl)
	for _, s := range cfg.AclSets {
		aclSets[s.Name] = s.Acls
	}

	// Expand ACL sets.
	for _, x := range cfg.Job {
		x.Acls = normalizeACLs(x.Acls, x.AclSets, aclSets)
		x.AclSets = nil
	}
	for _, x := range cfg.Trigger {
		x.Acls = normalizeACLs(x.Acls, x.AclSets, aclSets)
		x.AclSets = nil
	}
	cfg.AclSets = nil

	// Normalize triggers.
	for _, x := range cfg.Trigger {
		sort.Strings(x.Triggers)
		if gt := x.Gitiles; gt != nil {
			for i, r := range gt.Refs {
				if !strings.HasPrefix(r, "regexp:") {
					gt.Refs[i] = "regexp:" + regexp.QuoteMeta(r)
				}
			}
			sort.Strings(gt.Refs)
		}
	}

	return nil
}

func normalizeACLs(acls []*pb.Acl, sets []string, all map[string][]*pb.Acl) (out []*pb.Acl) {
	seen := stringset.New(0)

	emit := func(a []*pb.Acl) {
		for _, x := range a {
			if dedup := normalizeACL(x); seen.Add(dedup) {
				out = append(out, x)
			}
		}
	}

	emit(acls)
	for _, s := range sets {
		emit(all[s])
	}

	sort.Slice(out, func(i, j int) bool {
		switch l, r := out[i], out[j]; {
		case l.Role < r.Role:
			return true
		case l.Role > r.Role:
			return false
		default:
			return l.GrantedTo < r.GrantedTo
		}
	})
	return
}

func normalizeACL(a *pb.Acl) (dedupKey string) {
	if !strings.ContainsRune(a.GrantedTo, ':') {
		a.GrantedTo = "user:" + a.GrantedTo
	}
	return fmt.Sprintf("%d:%s", a.Role, a.GrantedTo)
}
