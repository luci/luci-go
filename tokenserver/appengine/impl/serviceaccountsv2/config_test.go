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

package serviceaccountsv2

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
)

const fakeMappingConfig = `
mapping {
	project: "proj1"
	project: "proj2"
	service_account: "sa1@example.com"
	service_account: "sa2@example.com"
}

mapping {
	project: "proj1"
	service_account: "sa3@example.com"
}

mapping {
	project: "proj3"
	service_account: "sa1@example.com"
}
`

func TestMapping(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := context.Background()

		mapping, err := loadMapping(ctx, fakeMappingConfig)
		So(err, ShouldBeNil)
		So(mapping, ShouldNotBeNil)

		// TODO(vadimsh): Implement.
	})
}

func loadMapping(ctx context.Context, text string) (*Mapping, error) {
	cfg := &admin.ServiceAccountsProjectMapping{}
	err := proto.UnmarshalText(text, cfg)
	if err != nil {
		return nil, err
	}
	mapping, err := prepareMapping(ctx, policy.ConfigBundle{configFileName: cfg}, "fake-revision")
	if err != nil {
		return nil, err
	}
	return mapping.(*Mapping), nil
}
