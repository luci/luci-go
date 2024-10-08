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

package cli

import (
	"flag"

	"go.chromium.org/luci/common/data/strpair"
	luciflag "go.chromium.org/luci/common/flag"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type tagsFlag struct {
	tags strpair.Map
}

func (f *tagsFlag) Register(fs *flag.FlagSet, help string) {
	f.tags = strpair.Map{}
	fs.Var(luciflag.StringPairs(f.tags), "t", help)
}

func (f *tagsFlag) Tags() []*pb.StringPair {
	return protoutil.StringPairs(f.tags)
}
