// Copyright 2018 The LUCI Authors.
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

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/milo/internal/utils"
)

// errGRPCNotFound is what gRPC API would have returned for NotFound error.
var errGRPCNotFound = status.Errorf(codes.NotFound, "not found")

// CLEmail implements Client interface.
func (p *implementation) CLEmail(c context.Context, host string, changeNumber int64) (email string, err error) {
	defer func() { err = utils.TagGRPC(c, err) }()
	changeInfo, err := p.clEmailAndProjectNoACLs(c, host, changeNumber)
	if err != nil {
		return
	}
	allowed, err := p.acls.IsAllowed(c, host, changeInfo.Project)
	switch {
	case err != nil:
		return
	case allowed:
		email = changeInfo.GetOwner().GetEmail()
	default:
		err = errGRPCNotFound
	}
	return
}

var gerritChangeInfoCache = layered.RegisterCache(layered.Parameters[*gerritpb.ChangeInfo]{
	ProcessCacheCapacity: 4096,
	GlobalNamespace:      "gerrit-change-info",
	Marshal: func(item *gerritpb.ChangeInfo) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (*gerritpb.ChangeInfo, error) {
		changeInfo := &gerritpb.ChangeInfo{}
		err := json.Unmarshal(blob, changeInfo)
		return changeInfo, err
	},
})

// clEmailAndProjectNoACLs fetches and caches change owner email and project.
//
// Gerrit change owner and project are deemed immutable.
// Caveat: technically only owner's account id is immutable. Owner's email
// associated with this account id may change, but this is rare.
func (p *implementation) clEmailAndProjectNoACLs(c context.Context, host string, changeNumber int64) (*gerritpb.ChangeInfo, error) {
	key := fmt.Sprintf("%s/%d", host, changeNumber)
	return gerritChangeInfoCache.GetOrCreate(c, key, func() (v *gerritpb.ChangeInfo, exp time.Duration, err error) {
		client, err := p.gerritClient(c, host)
		if err != nil {
			return nil, 0, err
		}

		info, err := client.GetChange(c, &gerritpb.GetChangeRequest{
			Number: changeNumber,
			Options: []gerritpb.QueryOption{
				gerritpb.QueryOption_DETAILED_ACCOUNTS,
				gerritpb.QueryOption_SKIP_MERGEABLE,
			},
		})
		// We can't cache outcome of not found CL because
		//  * Milo may not at first have access to a CL, say while CL was hidden or
		//    because of bad ACLs.
		//  * Gerrit is known to return 404 flakes.
		if err != nil {
			return nil, 0, err
		}

		// Cache and return only email and project.
		ret := &gerritpb.ChangeInfo{
			Project: info.Project,
			Owner:   &gerritpb.AccountInfo{Email: info.GetOwner().GetEmail()},
		}

		return ret, 0, nil
	})
}
