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

package cli

import (
	"context"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/logging"
)

// superLongLatency is the initial value of the minimum latency fields in sinkStatus
// that valid latnecy values should be less than.
const superLongLatency = int64(100 * 365 * 24 * time.Hour)

// sinkStats is a struct with a number of fields to track the run-time stats of a Sink run.
type sinkStats struct {
	runDuration      int64
	warmUpDuration   int64
	shutdownDuration int64
	trDrainDuration  int64
	artDrainDuration int64

	nReq uint32
	nArt uint32
	nTR  uint32

	// the time taken to handle ReportTestResults requests
	tReqLatency   int64
	minReqLatency int64
	maxReqLatency int64

	// the time taken to enqueue artifacts into the dispatcher channel
	tArtLatency   int64
	minArtLatency int64
	maxArtLatency int64

	// the time taken to enqueue test results intothe dispatcher channel
	tTRLatency   int64
	minTRLatency int64
	maxTRLatency int64
}

func newSinkStats() *sinkStats {
	return &sinkStats{
		minReqLatency: superLongLatency,
		minArtLatency: superLongLatency,
		minTRLatency:  superLongLatency,
	}
}
func (s *sinkStats) RunFinished(ctx context.Context, d time.Duration, err error) {
	s.runDuration = int64(d)
}

func (s *sinkStats) ServerWarmedUp(ctx context.Context, d time.Duration, err error) {
	s.warmUpDuration = int64(d)
}

func (s *sinkStats) ServerShutdown(ctx context.Context, d time.Duration, err error) {
	s.shutdownDuration = int64(d)
}
func (s *sinkStats) ArtifactChannelDrained(ctx context.Context, d time.Duration) {
	s.artDrainDuration = int64(d)
}
func (s *sinkStats) TestResultChannelDrained(ctx context.Context, d time.Duration) {
	s.trDrainDuration = int64(d)
}

func updateLatencies(total, min, max *int64, reported int64) {
	atomic.AddInt64(total, reported)
	if *min > reported {
		for saved := atomic.LoadInt64(min); saved > reported && atomic.CompareAndSwapInt64(min, saved, reported); {
		}
	}
	if *max < reported {
		for saved := atomic.LoadInt64(max); saved < reported && atomic.CompareAndSwapInt64(max, saved, reported); {
		}
	}
}

func (s *sinkStats) TestResultsReceived(ctx context.Context, reqL, artL, trL time.Duration, nArt int, nTR int, err error) {
	updateLatencies(&s.tReqLatency, &s.minReqLatency, &s.maxReqLatency, int64(reqL))
	updateLatencies(&s.tArtLatency, &s.minArtLatency, &s.maxArtLatency, int64(artL))
	updateLatencies(&s.tTRLatency, &s.minTRLatency, &s.maxTRLatency, int64(trL))

	atomic.AddUint32((*uint32)(&s.nReq), 1)
	atomic.AddUint32((*uint32)(&s.nTR), uint32(nTR))
	atomic.AddUint32((*uint32)(&s.nArt), uint32(nArt))
}

const summaryTemplate = `
Summary of rdb-stream
- Total runtime: %q
- Total Overhead: %q
  |- warmUpDuration: %q
  |- shutdownDuration: %q
	|- artDrainDruation: %q
	|- trDrainDuration: %q
  |- reqPrcessingLatency: %q

- Req latency breakdown (total/min/max)
  |- time taken to process ReportTestResults requests: %q %q %q
  |- time taken to enqueue Artifacts to the channel: %q %q %q
  |- time taken to enqueue TestResults to the channel: %q %q %q

- # of reports
  |- total # of requests: %d
  |- total # of artifacts: %d
  |- total # of test results: %d
`

func (s *sinkStats) logSummary(ctx context.Context) {
	totalOverhead := s.warmUpDuration + s.shutdownDuration + s.tReqLatency
	logging.Infof(
		ctx, summaryTemplate,
		time.Duration(s.runDuration),

		// overhead breakdown
		time.Duration(totalOverhead),
		time.Duration(s.warmUpDuration),
		time.Duration(s.shutdownDuration),
		time.Duration(s.artDrainDuration),
		time.Duration(s.trDrainDuration),
		time.Duration(s.tReqLatency),

		// req-latency breakdown
		time.Duration(s.tReqLatency),
		time.Duration(s.minReqLatency),
		time.Duration(s.maxReqLatency),
		time.Duration(s.tArtLatency),
		time.Duration(s.minArtLatency),
		time.Duration(s.maxArtLatency),
		time.Duration(s.tTRLatency),
		time.Duration(s.minTRLatency),
		time.Duration(s.maxTRLatency),

		// # of reports
		s.nReq, s.nArt, s.nTR,
	)
}
