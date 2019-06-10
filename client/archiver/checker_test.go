// Copyright 2016 The LUCI Authors.
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

package archiver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	service "go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

type fakeIsolateService struct {
	isolateService
	mu sync.Mutex

	// pushStates keeps track of the PushState objects it hands out, keyed
	// by digest (assumed to be unique).
	pushStates map[string]*isolatedclient.PushState

	// itemBatches keeps the incoming batches of items/digests.
	itemBatches [][]*service.HandlersEndpointsV1Digest

	// batchc is a channel on which incoming batches are sent (before processing)
	// if non-nil.
	batchc chan<- []*service.HandlersEndpointsV1Digest

	// errc is a channel from which errors are pulled, if non-nil.
	errc <-chan error
}

func (f *fakeIsolateService) Contains(ctx context.Context, digests []*service.HandlersEndpointsV1Digest) ([]*isolatedclient.PushState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var states []*isolatedclient.PushState
	f.itemBatches = append(f.itemBatches, digests)
	if f.batchc != nil {
		f.batchc <- digests
	}
	if f.pushStates == nil {
		f.pushStates = make(map[string]*isolatedclient.PushState)
	}
	for _, d := range digests {
		ps := &isolatedclient.PushState{}
		f.pushStates[d.Digest] = ps
		states = append(states, ps)
	}
	if f.errc != nil {
		return states, <-f.errc
	}
	return states, nil
}

func TestChecker(t *testing.T) {
	fake := &fakeIsolateService{}
	checker := newChecker(context.Background(), fake, 8)

	type itemPair struct {
		item *Item
		ps   *isolatedclient.PushState
	}

	gotc := make(chan itemPair, 150)
	for i := 0; i < 150; i++ {
		item := &Item{
			Path:   fmt.Sprintf("/item/%d", i),
			Digest: isolated.HexDigest(fmt.Sprintf("digest%d", i)),
		}
		checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {
			gotc <- itemPair{item, ps}
		})
	}

	if err := checker.Close(); err != nil {
		t.Fatalf("checker.Close: got error %v; want %v", err, nil)
	}
	close(gotc)

	// Check that we have 3 batches of 50 items each.
	if got, want := len(fake.itemBatches), 3; got != want {
		t.Errorf("checker received %d batches, want %d", got, want)
	}
	for i, batch := range fake.itemBatches {
		if got, want := len(batch), 50; got != want {
			t.Errorf("checker batch[%d] has len %d, want %d", i, got, want)
		}
	}

	// Check that the items/push states pairs match what the service gave.
	for got := range gotc {
		gotPS, wantPS := got.ps, fake.pushStates[string(got.item.Digest)]
		if gotPS != wantPS {
			t.Errorf("push state for item %v wrong", got.item)
			break
		}
	}

	if got, want := checker.Hit.Count(), 0; got != want {
		t.Errorf("checker hit count: got %v ; want: %v", got, want)
	}
	if got, want := checker.Miss.Count(), 150; got != want {
		t.Errorf("checker hit count: got %v ; want: %v", got, want)
	}
}

func TestCheckerDelay(t *testing.T) {
	batchc := make(chan []*service.HandlersEndpointsV1Digest, 2)
	fake := &fakeIsolateService{batchc: batchc}
	checker := newChecker(context.Background(), fake, 8)

	nop := func(item *Item, ps *isolatedclient.PushState) {}
	checker.AddItem(&Item{Digest: "aaa"}, false, nop)
	checker.AddItem(&Item{Digest: "bbb"}, false, nop)
	<-batchc // Block until a batch is sent.
	checker.AddItem(&Item{Digest: "ccc"}, false, nop)

	if err := checker.Close(); err != nil {
		t.Fatalf("checker.Close: got error %v; want %v", err, nil)
	}

	// Check that we have 2 batches (of 2 and 1 items respectively).
	if got, want := len(fake.itemBatches), 2; got != want {
		t.Errorf("checker received %d batches, want %d", got, want)
	}
	for i, batch := range fake.itemBatches {
		if got, want := len(batch), 2-i; got != want {
			t.Errorf("checker batch[%d] has len %d, want %d", i, got, want)
		}
	}
}

func TestCheckerAlreadyExists(t *testing.T) {
	fake := &fakeIsolateService{}
	checker := newChecker(context.Background(), fake, 8)

	item := &Item{
		Path:   fmt.Sprintf("/item/%d", 0),
		Digest: isolated.HexDigest(fmt.Sprintf("digest%d", 0)),
	}

	// Mark the item as presume exists
	checker.PresumeExists(item)

	// Now add the same item to the checker
	checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {})

	if err := checker.Close(); err != nil {
		t.Fatalf("checker.Close: got error %v; want %v", err, nil)
	}

	// Check that we have 0 batches, there should be no uploads.
	if got, want := len(fake.itemBatches), 0; got != want {
		t.Errorf("checker received %d batches, want %d", got, want)
	}

	if got, want := checker.Hit.Count(), 1; got != want {
		t.Errorf("checker hit count: got %v ; want: %v", got, want)
	}

	if got, want := checker.Miss.Count(), 0; got != want {
		t.Errorf("checker miss count: got %v ; want: %v", got, want)
	}
}

func disabledTestCheckerErrors(t *testing.T) {
	// Make an error channel which sends errBang on the second receive.
	errc := make(chan error, 2)
	errBang := errors.New("bang")
	errc <- nil
	errc <- errBang
	close(errc)

	fake := &fakeIsolateService{errc: errc}
	checker := newChecker(context.Background(), fake, 8)

	nop := func(item *Item, ps *isolatedclient.PushState) {}
	for i := 0; i < 150; i++ {
		item := &Item{
			Path:   fmt.Sprintf("/item/%d", i),
			Digest: isolated.HexDigest(fmt.Sprintf("digest%d", i)),
		}
		checker.AddItem(item, false, nop)
	}

	if err := checker.Close(); err != errBang {
		t.Fatalf("checker.Close: got error %v; want %v", err, errBang)
	}
}
