// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package format

import (
	"testing"

	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// testingBackend is a Backend implementation that ignores Authority.
type testingBackend struct {
	backend.B
	err   error
	items []*backend.Item
}

func (tb *testingBackend) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
	if err := tb.err; err != nil {
		return nil, tb.err
	}
	if len(tb.items) == 0 {
		return nil, cfgclient.ErrNoConfig
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

func (b *retainingBackend) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
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
	t.Parallel()

	Convey(`A testing environment`, t, func() {
		tb := testingBackend{
			items: []*backend.Item{
				{Meta: backend.Meta{"projects/foo", "path", "####", "v1"}, Content: "foo"},
				{Meta: backend.Meta{"projects/bar", "path", "####", "v1"}, Content: "bar"},
			},
		}

		// Pass all things from the backend through the formatter.
		var fr cfgclient.FormatterRegistry
		fb := Backend{
			B:           &tb,
			GetRegistry: func(context.Context) *cfgclient.FormatterRegistry { return &fr },
		}

		// Retain all raw Items that were returned for examination.
		rb := retainingBackend{
			B: &fb,
		}

		c := context.Background()
		c = backend.WithBackend(c, &rb)

		Convey(`Will panic if an attempt to register empty key is made.`, func() {
			So(func() { fr.Register("", nil) }, ShouldPanic)
		})

		Convey(`Will ignore items that have already been formatted.`, func() {
			rr := retainingResolver{"test", nil}
			fr.Register("test", panicFormatter{})

			// Confirm that this setup correctly attempts to format the item.
			So(func() { cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil) }, ShouldPanic)

			// Now, pretend the item is already formatter. We should not get a panic.
			tb.items[0].FormatSpec.Formatter = "something"
			So(cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil), ShouldBeNil)
		})

		Convey(`Testing custom formatter`, func() {
			cf := customFormatter("content")
			fr.Register("test", cf)

			// Confirm that this setup correctly attempts to format the item.
			rr := retainingResolver{"test", nil}
			So(cfgclient.Get(c, cfgclient.AsService, "", "", &rr, nil), ShouldBeNil)
			So(rr.item, ShouldNotBeNil)
			So(rr.item.FormatSpec.Formatter, ShouldEqual, "test")
			So(rr.item.Content, ShouldEqual, "content")
		})
	})
}
