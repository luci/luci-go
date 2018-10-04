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

package tq

import (
	"context"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/common/data/stringset"
)

// Internals is used by tqtesting package and must not be used directly.
//
// For that reason it returns opaque interface type, to curb the curiosity.
//
// We do this to avoid linking testing implementation into production binaries.
// We make testing live in a different package and use this secret back door API
// to talk to Dispatcher.
func (d *Dispatcher) Internals() interface{} {
	return internalsImpl{d}
}

// internalsImpl secretly conforms to tqtesting.dispatcherInternals interface.
type internalsImpl struct {
	*Dispatcher
}

func (d internalsImpl) GetAllQueues() []string {
	qs := stringset.New(0)
	d.mu.RLock()
	for _, h := range d.handlers {
		qs.Add(h.queue)
	}
	d.mu.RUnlock()
	return qs.ToSlice()
}

func (d internalsImpl) GetPayload(blob []byte) (proto.Message, error) {
	payload, err := deserializePayload(blob)
	if err != nil {
		return nil, err
	}
	if _, err := d.handler(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (d internalsImpl) GetHandler(payload proto.Message) (cb Handler, q string, err error) {
	h, err := d.handler(payload)
	if err != nil {
		return nil, "", err
	}
	return h.cb, h.queue, nil
}

func (d internalsImpl) WithRequestHeaders(c context.Context, hdr *taskqueue.RequestHeaders) context.Context {
	return withRequestHeaders(c, hdr)
}
