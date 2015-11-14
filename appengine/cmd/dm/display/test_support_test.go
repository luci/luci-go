// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"time"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

var randSeed = time.Now().UnixNano()

func init() {
	fmt.Println("random seed:", randSeed)
}

func newRand() *rand.Rand {
	return rand.New(rand.NewSource(randSeed))
}

func randByteSequence(r *rand.Rand, n int, alphabet string) []byte {
	ret := make([]byte, n)
	for i := range ret {
		ret[i] = alphabet[r.Intn(len(alphabet))]
	}
	return ret
}

const encodeURL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// avoid circular import with model
var questIDLength = base64.URLEncoding.EncodedLen(sha256.Size) - 1

func randQuestID(r *rand.Rand) string {
	return string(randByteSequence(r, questIDLength, encodeURL))
}

func randAttemptID(r *rand.Rand) *types.AttemptID {
	return &types.AttemptID{
		QuestID:    randQuestID(r),
		AttemptNum: r.Uint32(),
	}
}

func randU32s(r *rand.Rand, n int) types.U32s {
	ret := make(types.U32s, n)
	for i := range ret {
		ret[i] = r.Uint32()
	}
	return ret
}
