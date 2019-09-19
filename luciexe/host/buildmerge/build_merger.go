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

package buildmerge

import (
	"github.com/golang/protobuf/ptypes/timestamp"
)

type calcURLFn func(namespace, streamName string) (url, viewURL string)

// TODO(iannucci): this is a temporary type to facilitate the addition of
// buildState in a standalone CL.
type agent struct {
	clockNow      func() *timestamp.Timestamp
	calculateURLs calcURLFn
	informed      chan struct{}
}

func (a *agent) informNewData() {
	a.informed <- struct{}{}
}
