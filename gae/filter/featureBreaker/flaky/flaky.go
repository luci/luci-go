// Copyright 2017 The LUCI Authors.
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

// Package flaky can be used to emulate flaky GAE errors that happen randomly
// with some probability.
//
// NOTE: Emulates only datastore errors currently.
//
// To install it:
//   ctx, fb := featureBreaker.FilterRDS(ctx, nil)
//   fb.BreakFeaturesWithCallback(
//     flaky.Errors(...), featureBreaker.DatastoreFeatures...)
package flaky

import (
	"errors"
	"math/rand"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/service/datastore"
)

// ErrFlakyRPCDeadline is returned by FlakyErrors to indicate a deadline.
var ErrFlakyRPCDeadline = errors.New("simulated RPC deadline error")

// Params define options for Errors
type Params struct {
	// Rand is a source of pseudo-randomness to use.
	//
	// It will be accessed under a lock.
	//
	// By default it is new rand.NewSource(0).
	Rand interface {
		Int63() int64 // uniformly-distributed pseudo-random value in the range [0, 1<<63)
	}

	// DeadlineProbability is a probability of ErrFlakyRPCDeadline happening on
	// any of the calls that involve RPCs.
	//
	// Deadline can happen independently of other errors. For example,
	// a transaction can fail due to a deadline or due to a commit confict. These
	// events are assumed to be independent and their probabilities add up
	// accordingly.
	//
	// Default is 0 (no deadline errors at all).
	DeadlineProbability float64

	// ConcurrentTransactionProbability is a probability of datastore commit
	// returning ErrConcurrentTransaction.
	//
	// Default is 0 (no commit errors at all).
	ConcurrentTransactionProbability float64
}

// Errors can be used to emulate flaky GAE errors that happen randomly with some
// probability.
//
// NOTE: Emulates only datastore errors currently.
func Errors(params Params) featureBreaker.BreakFeatureCallback {
	if params.Rand == nil {
		params.Rand = rand.NewSource(0)
	}
	dice := diceRoller{p: &params} // stateful!
	return func(_ context.Context, feature string) error {
		// All datastore calls that involve RPCs.
		switch feature {
		case
			"AllocateIDs",
			"Run",
			"Count",
			"BeginTransaction",
			"CommitTransaction",
			"DeleteMulti",
			"GetMulti",
			"PutMulti":
			if dice.roll(params.DeadlineProbability) {
				return ErrFlakyRPCDeadline
			}
		}

		if feature == "CommitTransaction" && dice.roll(params.ConcurrentTransactionProbability) {
			return datastore.ErrConcurrentTransaction
		}

		return nil
	}
}

type diceRoller struct {
	l sync.Mutex
	p *Params
}

func (r *diceRoller) roll(prob float64) bool {
	r.l.Lock()
	rnd := r.p.Rand.Int63()
	r.l.Unlock()
	return float64(rnd)/(1<<63) < prob
}
