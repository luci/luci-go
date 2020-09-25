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

package eval

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

var clNotFound = &errors.BoolTag{Key: errors.NewTagKey("CL not found")}

// GerritChange is a CL on Gerrit.
type GerritChange struct {
	Host   string `json:"host"`
	Number int    `json:"change"`
}

// String returns the CL URL.
func (cl *GerritChange) String() string {
	return fmt.Sprintf("https://%s/c/%d", cl.Host, cl.Number)
}

// GerritPatchset is a revision of a Gerrit CL.
type GerritPatchset struct {
	Change   GerritChange `json:"cl"`
	Patchset int          `json:"patchset"`
}

// String returns the patchset URL.
func (p *GerritPatchset) String() string {
	return fmt.Sprintf("https://%s/c/%d/%d", p.Change.Host, p.Change.Number, p.Patchset)
}

type gerritClient struct {
	// getChangeRPC makes a request to Gerrit and fetches a change info.
	// Mockable.
	getChangeRPC func(ctx context.Context, host string, req *gerritpb.GetChangeRequest) (*gerritpb.ChangeInfo, error)
	limiter      *rate.Limiter
}

// ChangedFiles returns the list of files changed in the given patchset.
func (c *gerritClient) ChangedFiles(ctx context.Context, ps *GerritPatchset) ([]string, error) {
	// TODO(crbug.com/1112125): implement.
	panic("not implemented")
}
