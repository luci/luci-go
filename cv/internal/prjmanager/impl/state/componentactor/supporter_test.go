// Copyright 2021 The LUCI Authors.
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

package componentactor

import (
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

type simpleSupporter struct {
	pb  *prjpb.PState
	cgs []*config.ConfigGroup
}

func (s *simpleSupporter) PCL(clid int64) *prjpb.PCL {
	for _, pcl := range s.pb.GetPcls() {
		if pcl.GetClid() == clid {
			return pcl
		}
	}
	return nil
}

func (s *simpleSupporter) ConfigGroup(index int32) *config.ConfigGroup {
	return s.cgs[index]
}
