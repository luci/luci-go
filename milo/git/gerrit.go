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

// FetchGerritChangeEmail fetches the CL owner email.
//
// Returns empty string if either:
//   CL doesn't exist, or
//   current user has no access to CL's project.
func FetchGerritChangeEmail(c context.Context, host string, changeNumber int64) (string, error) {
	// First, obtain email and project of the change.
	getEmailAndProject := func() (string, string, error) {
		// Cache (project, email} proto to determine whether current user has access
		// to this CL.
		cache := memcache.NewItem(c, fmt.Sprintf("gerrit-change-owner/%s/%d", host, changeNumber))
		if err := memcache.Get(c, cache); err == nil {
			val := gerritpb.ChangeInfo{}
			if err = proto.Unmarshal(cache.Value(), &val); err == nil {
				return val.GetOwner().GetEmail(), val.Project, nil
			}
		}

		client, err := gerritClient(c, host, projectUnknownAssumeAllowed)
		if err != nil {
			return "", "", err
		}
		changeInfo, err := client.GetChange(c, &gerritpb.GetChangeRequest{
			Number:  changeNumber,
			Options: []gerritpb.QueryOption{gerritpb.QueryOption_DETAILED_ACCOUNTS},
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return "", "", err
		}

		email := changeInfo.GetOwner().GetEmail()
		// Cache only email and project.
		val := gerritpb.ChangeInfo{Project: changeInfo.Project, Owner: &gerritpb.AccountInfo{Email: email}}
		if blob, err := proto.Marshal(&val); err == nil {
			cache.SetValue(blob)
			memcache.Set(c, cache)
		}
		return email, changeInfo.Project, nil
	}

	email, project, err := getEmailAndProject()
	if err != nil {
		return "", err
	}
	switch allowed, err := isAllowed(c, host, project); {
	case err != nil:
		return "", err
	case allowed:
		return email, err
	default:
		return "", nil
	}
}
