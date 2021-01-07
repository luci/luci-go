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

// Gitiles commits are stored in the database, for purposes of using their
// commit position to index historical test results by, as a pair of fields:
// `Ordinal` and `OrdinalDomain`. The functions in this file convert between
// these and the CommitPosition proto message.

package spanutil

import (
	"fmt"
	"regexp"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var gitilesDomainRE = regexp.MustCompile(`^gitiles://([^/]+)/(.+)/\+/(.+)$`)

// GitilesCommitFromOrdinalFields returns a commit position proto based on the
// given ordinal fields.
func GitilesCommitFromOrdinalFields(domain string, ordinal spanner.NullInt64) (cm *pb.CommitPosition, err error) {
	matches := gitilesDomainRE.FindStringSubmatch(domain)
	switch {
	case domain == "" && ordinal.IsNull():
		return nil, nil
	case domain == "" && !ordinal.IsNull():
		return nil, errors.Reason("ordinal %d does not make sense without a domain", ordinal.Int64).Err()
	case matches == nil:
		return nil, errors.Reason("unsupported ordinal domain %q", domain).Err()
	case !ordinal.Valid:
		return nil, errors.Reason("ordinal field was not set, but ordinalDomain was").Err()
	}
	return &pb.CommitPosition{
		Host:     matches[1],
		Project:  matches[2],
		Ref:      matches[3],
		Position: ordinal.Int64,
	}, nil

}

// GitilesCommitOrdinalDomain composes a string that identifies the gitiles ref that a
// commit's position exists in.
func GitilesCommitOrdinalDomain(cm *pb.CommitPosition) string {
	return fmt.Sprintf("gitiles://%s/%s/+/%s", cm.Host, cm.Project, cm.Ref)
}
