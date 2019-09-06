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

package null

import (
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
)

// Output implements the butler output.Output interface, but sends the data
// nowhere.
//
// Output does collect stats, however.
type Output struct {
	statsMu sync.RWMutex
	stats   output.StatsBase
}

var _ output.Output = (*Output)(nil)

// SendBundle implements output.Output
func (o *Output) SendBundle(b *logpb.ButlerLogBundle) error {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()
	o.stats.F.SentMessages += int64(len(b.Entries))
	o.stats.F.SentBytes += int64(proto.Size(b))
	return nil
}

// MaxSendBundles implements output.Output
func (o *Output) MaxSendBundles() int {
	return 1
}

// Stats implements output.Output
func (o *Output) Stats() output.Stats {
	o.statsMu.RLock()
	defer o.statsMu.RUnlock()
	statsCp := o.stats
	return &statsCp
}

// URLConstructionEnv implements output.Output
func (o *Output) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{
		Project: "null",
		Prefix:  "null",
	}
}

// MaxSize returns a large number instead of 0 because butler has bugs.
func (o *Output) MaxSize() int { return 1024 * 1024 * 1024 }

// Close implements output.Output
func (o *Output) Close() {}
