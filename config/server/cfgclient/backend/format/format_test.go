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

package format

import (
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// testingBackend is a Backend implementation that ignores Authority.
type testingBackend struct {
	backend.B
	err   error
	items []*backend.Item
}

func (tb *testingBackend) Get(c context.Context, configSet config.Set, path string, p backend.Params) (*backend.Item, error) {
	if err := tb.err; err != nil {
		return nil, tb.err
	}
	if len(tb.items) == 0 {
		return nil, config.ErrNoConfig
	}
	return tb.cloneItems()[0], nil
}

func (tb *testingBackend) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*backend.Item, error) {

	if err := tb.err; err != nil {
		return nil, tb.err
	}
	return tb.cloneItems(), nil
}

func (tb *testingBackend) cloneItems() []*backend.Item {
	clones := make([]*backend.Item, len(tb.items))
	for i, it := range tb.items {
		clone := *it
		clones[i] = &clone
	}
	return clones
}

// retainingBackend is a simple Backend implementation that retains the last set
// of data that it received.
type retainingBackend struct {
	backend.B

	lastItems []*backend.Item
	lastErr   error
}

func (b *retainingBackend) Get(c context.Context, configSet config.Set, path string, p backend.Params) (*backend.Item, error) {
	var item *backend.Item
	item, b.lastErr = b.B.Get(c, configSet, path, p)
	b.lastItems = []*backend.Item{item}
	return item, b.lastErr
}

func (b *retainingBackend) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*backend.Item, error) {

	b.lastItems, b.lastErr = b.B.GetAll(c, t, path, p)
	return b.lastItems, b.lastErr
}

type customFormatter string

func (cf customFormatter) FormatItem(c string, fd string) (string, error) {
	return string(cf), nil
}

type retainingResolver struct {
	format string
	item   *backend.Item
}

func (rr *retainingResolver) Format() backend.FormatSpec { return backend.FormatSpec{rr.format, ""} }
func (rr *retainingResolver) Resolve(it *backend.Item) error {
	rr.item = it
	return nil
}

var _ interface {
	cfgclient.Resolver
	cfgclient.FormattingResolver
} = (*retainingResolver)(nil)

type panicFormatter struct{}

func (pf panicFormatter) FormatItem(string, string) (string, error) { panic("panic") }

func TestFormatBackend(t *testing.T) {
	Convey(`A testing environment`, t, func() {
		tb := testingBackend{
			items: []*backend.Item{
				{Meta: config.Meta{"projects/foo", "path", "####", "v1", "config_url"}, Content: "foo"},
				{Meta: config.Meta{"projects/bar", "path", "####", "v1", "config_url"}, Content: "bar"},
			},
		}

		// Pass all things from the backend through the formatter.
		fb := Backend{
			B: &tb,
		}

		// Retain all raw Items that were returned for examination.
		rb := retainingBackend{
			B: &fb,
		}

		c := context.Background()
		c = backend.WithBackend(c, &rb)

		ClearRegistry()

		Convey(`Will panic if an attempt to register empty key is made.`, func() {
			So(func() { Register("", nil) }, ShouldPanic)
		})

		Convey(`Will ignore items that have already been formatted.`, func() {
			rr := retainingResolver{"test", nil}
			Register("test", panicFormatter{})

			// Confirm that this setup correctly attempts to format the item.
			So(func() { cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil) }, ShouldPanic)

			// Now, pretend the item is already formatter. We should not get a panic.
			tb.items[0].FormatSpec.Formatter = "something"
			So(cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil), ShouldBeNil)
		})

		Convey(`Testing custom formatter`, func() {
			cf := customFormatter("content")
			Register("test", cf)

			// Confirm that this setup correctly attempts to format the item.
			rr := retainingResolver{"test", nil}
			So(cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil), ShouldBeNil)
			So(rr.item, ShouldNotBeNil)
			So(rr.item.FormatSpec.Formatter, ShouldEqual, "test")
			So(rr.item.Content, ShouldEqual, "content")
		})
	})
}
