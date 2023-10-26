// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"fmt"

	"go.chromium.org/luci/analysis/internal/bugs"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func createAssociatedBugPB(b bugs.BugID, cfg *configpb.ProjectConfig) *pb.AssociatedBug {
	// Fallback bug name and URL.
	linkText := fmt.Sprintf("%s/%s", b.System, b.ID)
	url := ""

	switch b.System {
	case bugs.MonorailSystem:
		monorailCfg := cfg.BugManagement.GetMonorail()
		if monorailCfg == nil {
			// Try fetching from legacy config location.
			monorailCfg = cfg.Monorail
		}
		if monorailCfg == nil {
			break
		}
		project, id, err := b.MonorailProjectAndID()
		if err != nil {
			// Fallback to basic name and blank URL.
			break
		}
		if project == monorailCfg.Project {
			if monorailCfg.DisplayPrefix != "" {
				linkText = fmt.Sprintf("%s/%s", monorailCfg.DisplayPrefix, id)
			} else {
				linkText = id
			}
		}
		if monorailCfg.MonorailHostname != "" {
			url = fmt.Sprintf("https://%s/p/%s/issues/detail?id=%s", monorailCfg.MonorailHostname, project, id)
		}
	case bugs.BuganizerSystem:
		linkText = fmt.Sprintf("b/%s", b.ID)
		url = fmt.Sprintf("https://issuetracker.google.com/issues/%s", b.ID)
	default:
		// Fallback.
	}
	return &pb.AssociatedBug{
		System:   b.System,
		Id:       b.ID,
		LinkText: linkText,
		Url:      url,
	}
}
