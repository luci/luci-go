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
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/gae/service/memcache"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CLEmail implements Client interface.
func (p *implementation) CLEmail(c context.Context, host string, changeNumber int64) (string, error) {
	changeInfo, err := p.changeEmailAndProject(c, host, changeNumber)
	if err != nil {
		return "", err
	}
	switch allowed, err := p.acls.IsAllowed(c, host, changeInfo.Project); {
	case err != nil:
		return "", err
	case allowed:
		return changeInfo.GetOwner().GetEmail(), nil
	default:
		return "", status.Errorf(codes.NotFound, "https://%s/%d not found or no access", host, changeNumber)
	}
}

// changeEmailAndProject fetches and caches change owner email and project.
//
// Gerrit change owner and project are deemed immutable.
// Caveat: technically only owner's account id is immutable. Owner's email
// associated with this account id may change, but this is rare.
func (p *implementation) changeEmailAndProject(c context.Context, host string, changeNumber int64) (*gerritpb.ChangeInfo, error) {
	cache := memcache.NewItem(c, fmt.Sprintf("gerrit-change-owner/%s/%d", host, changeNumber))
	if err := memcache.Get(c, cache); err == nil {
		val := &gerritpb.ChangeInfo{}
		if err = proto.Unmarshal(cache.Value(), val); err == nil && val.Project != "" {
			return val, nil
		}
	}

	client, err := p.gerritClient(c, host)
	if err != nil {
		return nil, err
	}
	changeInfo, err := client.GetChange(c, &gerritpb.GetChangeRequest{
		Number:  changeNumber,
		Options: []gerritpb.QueryOption{gerritpb.QueryOption_DETAILED_ACCOUNTS},
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}

	// Cache and return only email and project.
	ret := &gerritpb.ChangeInfo{
		Project: changeInfo.Project,
		Owner:   &gerritpb.AccountInfo{Email: changeInfo.GetOwner().GetEmail()},
	}
	if blob, err := proto.Marshal(ret); err == nil {
		cache.SetValue(blob)
		memcache.Set(c, cache)
	}
	return ret, nil
}
