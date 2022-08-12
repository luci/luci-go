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

package limit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.chromium.org/luci/server/auth"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

// ErrUserLimitReached indicates a given operation failed because the user's
// usage already reached the limit.
var ErrUserLimitReached = errors.New("reached the limit")

// StartRun consumes the quota for the given user based on the matching policy.
//
// Returns ErrUserLimitReached, if the limit was reached and the run shouldn't
// start.
func StartRun(ctx context.Context, r *run.Run, cg *prjcfg.ConfigGroup) error {
	_, err := findUserLimit(ctx, cg, r)
	if err != nil {
		return err
	}

	// TODO(crbug.com/1335519): implement me
	return nil
}

// findUserLimit find a UserLimit applicable to a given Run.
func findUserLimit(ctx context.Context, cg *prjcfg.ConfigGroup, r *run.Run) (*cfgpb.UserLimit, error) {
	ownerEmail := r.Owner.Email()
	authDB := auth.GetState(ctx).DB()

	for _, lim := range cg.Content.GetUserLimits() {
		for _, id := range lim.GetPrincipals() {
			switch chunks := strings.Split(id, ":"); {
			case len(chunks) != 2:
				// a bug in the project config validation?
				panic(fmt.Errorf("invalid Principal %q in %q", id, lim.GetName()))
			case chunks[0] == "user":
				if chunks[1] == ownerEmail {
					return lim, nil
				}
			case chunks[0] == "group":
				switch yes, err := authDB.IsMember(ctx, r.Owner, []string{chunks[1]}); {
				case err != nil:
					return nil, err
				case yes:
					return lim, nil
				}
			default:
				panic(fmt.Errorf("unknown principal kind %q", chunks[0]))
			}
		}
	}
	// Use the default limit, if no limit was found.
	return cg.Content.GetUserLimitDefault(), nil
}
