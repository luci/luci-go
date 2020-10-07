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

// Package eval implements evaluation of a pre-submit RTS algorithm.
//
// RTS
//
// RTS stands for Regression Test Selection. It is a technique to intellegently
// select tests to run, such that bad code is detected, but without spending too
// much resources on testing.
// An RTS algorithm for pre-submit accepts a list of changed files as input
// and returns a list of tests to run as output.
//
// Why evaluation
//
// An RTS algorithm can significantly increase efficiency of CQ, but also it can
// start letting bad code into the repository and create a havoc for sheriffs.
// Thus before an RTS algorithm is deployed, its safety and efficiency must be
// evaluated.
//
// Also there are many possible RTS algorithms and we need objective metrics
// to choose among them.
//
// Finally an algorithm developer needs objective metrics in order to make
// iterative improvements to the algorithm.
//
// Quick start
//
// The primary entry point to this package is function Main(), which accepts
// an RTS algorithm as input and prints its safety and efficiency metrics to
// stdout.
// The following is a coinflip RTS algorithm which drops a test with 0.5
// probability:
//
//   func main() {
//   	ctx := context.Background()
//   	rand.Seed(time.Now().Unix())
//   	eval.Main(ctx, func(ctx context.Context, in eval.Input) (eval.Output, error) {
//   		return eval.Output{
//   			ShouldRun: rand.Intn(2) == 0,
//   		}, nil
//   	})
//   }
//
// This compiles to a program that evaluates the coinflip algorithm.
// Program execution requires a history file as input:
//
//   ./rts-random -history cq.hist
//
// Example of output:
//
//   Safety:
//     Score: 74%
//     # of eligible rejections: 5190
//     # of them preserved by this RTS: 3851
//   Efficiency:
//     Saved: 50%
//     Compute time in the sample: 131h44m38.864038382s
//     Forecasted compute time: 65h58m34.295175147s
//   Total records: 605372
//
// This tells us that safety of coinflip is only 74%, meaning it would let
// 26% of bad CLs through. Also it tells that it would unsurprisingly save 50%
// of compute time.
//
// The coinflip algorithm is notable because it represents the bare minimum.
// Any RTS algorithm we consider must have much higher safety than 74%.
// The algorithm is implemented in
// https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/rts/cmd/rts-random/
//
// History files
//
// RTS evaluation emulates CQ with the given RTS algorithm, based on historical
// records read from a history file. Different LUCI projects acquire the
// history files differently.
// For Chromium, use https://pkg.go.dev/go.chromium.org/luci/rts/cmd/rts-chromium.
//
// For history file format, see
// https://pkg.go.dev/go.chromium.org/luci/rts/presubmit/eval/history
//
// Safety evaluation
//
// Safety is evaluated as a ratio preserved_rejections/total_rejections,
// where
//   - total_rejections is the number of patchsets rejected by CQ due to test
//     failures.
//   - preserved_rejections is how many of them would still be rejected
//     if the given RTS algorithm was deployed.
//
// A rejection is considered preserved iff the RTS algorithm selects at least
// one test that caused the rejection.
//
// The ideal safety score is 1.0, meaning all historical rejections would be
// preserved.
//
// Efficiency evaluation
//
// Efficiency is evaluated as the amount of saved compute time:
// forecast_duration/total_duration, where
//
//   - total_duration is the sum of test durations found in the history file.
//   - forecast_duration is the duration sum for those tests that the RTS
//     algorithm decides to select.
package eval
