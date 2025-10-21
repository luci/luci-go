// Copyright 2025 The LUCI Authors.
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

package proxyserver

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/dustin/go-humanize"

	"go.chromium.org/luci/common/logging"

	logpb "go.chromium.org/luci/cipd/api/cipd/v1/logpb"
)

// ProxyStats are used in -json-output of `proxy` subcommand.
type ProxyStats struct {
	m sync.Mutex

	// RPCs is how many times each RPC was called.
	RPCs map[string]int64 `json:"rpcs"`
	// CAS is how many times a corresponding HTTP CAS operation was performed.
	CAS map[string]int64 `json:"cas"`

	// Downloaded is the total number of bytes downloaded from the remote CAS.
	Downloaded uint64 `json:"downloaded"`
	// Uploaded is the total number of bytes uploaded to the remote CAS.
	Uploaded uint64 `json:"uploaded"`
}

// RPCCall records an RPC call outcome.
func (p *ProxyStats) RPCCall(entry *logpb.AccessLogEntry) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.RPCs == nil {
		p.RPCs = map[string]int64{}
	}
	p.RPCs[entry.Method]++
}

// CASOp records CAS operation outcome.
func (p *ProxyStats) CASOp(op *CASOp) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.CAS == nil {
		p.CAS = map[string]int64{}
	}
	switch {
	case op.Downloaded != 0:
		p.CAS["GET"]++
		p.Downloaded += op.Downloaded
	case op.Uploaded != 0:
		p.CAS["PUT"]++
		p.Uploaded += op.Uploaded
	}
}

// Report dumps the stats into the logger.
func (p *ProxyStats) Report(ctx context.Context) {
	p.m.Lock()
	defer p.m.Unlock()

	line := func(key, val string) {
		logging.Infof(ctx, "%s%s%s", key, strings.Repeat(" ", max(1, 40-len(key))), val)
	}

	if len(p.RPCs) != 0 {
		logging.Infof(ctx, "RPC stats")
		logging.Infof(ctx, "-----------------------")
		for _, key := range slices.Sorted(maps.Keys(p.RPCs)) {
			line(key, fmt.Sprintf("%d calls", p.RPCs[key]))
		}
		logging.Infof(ctx, "-----------------------")
	}
	if len(p.CAS) != 0 {
		logging.Infof(ctx, "CAS stats")
		logging.Infof(ctx, "-----------------------")
		for _, key := range slices.Sorted(maps.Keys(p.CAS)) {
			line(key, fmt.Sprintf("%d requests", p.CAS[key]))
		}
		if p.Downloaded != 0 {
			line("Totally downloaded", humanize.Bytes(p.Downloaded))
		}
		if p.Uploaded != 0 {
			line("Totally uploaded", humanize.Bytes(p.Uploaded))
		}
		logging.Infof(ctx, "-----------------------")
	}
}
