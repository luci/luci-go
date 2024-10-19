// Copyright 2015 The LUCI Authors.
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

package count

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/mail"
	"go.chromium.org/luci/gae/service/memcache"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/gae/service/user"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func shouldHaveSuccessesAndErrors(actual any, expected ...any) string {
	a := actual.(Entry)
	if len(expected) != 2 {
		panic("Invalid number of expected, should be 2 (successes, errors).")
	}
	s, e := expected[0].(int), expected[1].(int)

	if val := a.Successes(); val != s {
		return fmt.Sprintf("Actual successes (%d) don't match expected (%d)", val, s)
	}
	if val := a.Errors(); val != e {
		return fmt.Sprintf("Actual errors (%d) don't match expected (%d)", val, e)
	}
	return ""
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Count filter", t, func(t *ftt.Test) {
		c, fb := featureBreaker.FilterRDS(memory.Use(context.Background()), nil)
		c, ctr := FilterRDS(c)

		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		vals := []ds.PropertyMap{{
			"Val":  ds.MkProperty(100),
			"$key": ds.MkPropertyNI(ds.NewKey(c, "Kind", "", 1, nil)),
		}}

		t.Run("Calling a ds function should reflect in counter", func(t *ftt.Test) {
			assert.Loosely(t, ds.Put(c, vals), should.BeNil)
			assert.Loosely(t, ctr.PutMulti.Successes(), should.Equal(1))

			t.Run("effects are cumulative", func(t *ftt.Test) {
				assert.Loosely(t, ds.Put(c, vals), should.BeNil)
				assert.Loosely(t, ctr.PutMulti.Successes(), should.Equal(2))

				t.Run("even within transactions", func(t *ftt.Test) {
					die(ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.Put(c, append(vals, vals[0])), should.BeNil)
						return nil
					}, nil))
				})
			})
		})

		t.Run("errors count against errors", func(t *ftt.Test) {
			fb.BreakFeatures(nil, "GetMulti")

			assert.Loosely(t, ds.Get(c, vals), should.ErrLike(`"GetMulti" is broken`))
			assert.Loosely(t, ctr.GetMulti.Errors(), should.Equal(1))

			fb.UnbreakFeatures("GetMulti")

			assert.Loosely(t, ds.Put(c, vals), should.BeNil)

			die(ds.Get(c, vals))
			assert.Loosely(t, ctr.GetMulti.Errors(), should.Equal(1))
			assert.Loosely(t, ctr.GetMulti.Successes(), should.Equal(1))
			assert.Loosely(t, ctr.GetMulti.Total(), should.Equal(2))
		})

		t.Run(`datastore.Stop does not count as an error for queries`, func(t *ftt.Test) {
			fb.BreakFeatures(ds.Stop, "Run")

			assert.Loosely(t, ds.Run(c, ds.NewQuery("foof"), func(_ ds.PropertyMap) error {
				return nil
			}), should.BeNil)
			assert.Loosely(t, ctr.Run.Successes(), should.Equal(1))
			assert.Loosely(t, ctr.Run.Errors(), should.BeZero)
			assert.Loosely(t, ctr.Run.Total(), should.Equal(1))
		})
	})

	ftt.Run("works for memcache", t, func(t *ftt.Test) {
		c, ctr := FilterMC(memory.Use(context.Background()))
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		die(memcache.Set(c, memcache.NewItem(c, "hello").SetValue([]byte("sup"))))

		_, err := memcache.GetKey(c, "Wat")
		assert.Loosely(t, err, should.NotBeNil)

		_, err = memcache.GetKey(c, "hello")
		die(err)

		assert.Loosely(t, ctr.SetMulti, convey.Adapt(shouldHaveSuccessesAndErrors)(1, 0))
		assert.Loosely(t, ctr.GetMulti, convey.Adapt(shouldHaveSuccessesAndErrors)(2, 0))
		assert.Loosely(t, ctr.NewItem, convey.Adapt(shouldHaveSuccessesAndErrors)(3, 0))
	})

	ftt.Run("works for taskqueue", t, func(t *ftt.Test) {
		c, ctr := FilterTQ(memory.Use(context.Background()))
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		die(taskqueue.Add(c, "", &taskqueue.Task{Name: "wat"}))
		assert.Loosely(t, taskqueue.Add(c, "DNE_QUEUE", &taskqueue.Task{Name: "wat"}),
			should.ErrLike("UNKNOWN_QUEUE"))

		assert.Loosely(t, ctr.AddMulti, convey.Adapt(shouldHaveSuccessesAndErrors)(1, 1))
	})

	ftt.Run("works for global info", t, func(t *ftt.Test) {
		c, fb := featureBreaker.FilterGI(memory.Use(context.Background()), nil)
		c, ctr := FilterGI(c)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		_, err := info.Namespace(c, "foo")
		die(err)
		fb.BreakFeatures(nil, "Namespace")
		_, err = info.Namespace(c, "boom")
		assert.Loosely(t, err, should.ErrLike(`"Namespace" is broken`))

		assert.Loosely(t, ctr.Namespace, convey.Adapt(shouldHaveSuccessesAndErrors)(1, 1))
	})

	ftt.Run("works for user", t, func(t *ftt.Test) {
		c, fb := featureBreaker.FilterUser(memory.Use(context.Background()), nil)
		c, ctr := FilterUser(c)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		_, err := user.CurrentOAuth(c, "foo")
		die(err)
		fb.BreakFeatures(nil, "CurrentOAuth")
		_, err = user.CurrentOAuth(c, "foo")
		assert.Loosely(t, err, should.ErrLike(`"CurrentOAuth" is broken`))

		assert.Loosely(t, ctr.CurrentOAuth, convey.Adapt(shouldHaveSuccessesAndErrors)(1, 1))
	})

	ftt.Run("works for mail", t, func(t *ftt.Test) {
		c, fb := featureBreaker.FilterMail(memory.Use(context.Background()), nil)
		c, ctr := FilterMail(c)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, ctr, should.NotBeNil)

		err := mail.Send(c, &mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		die(err)

		fb.BreakFeatures(nil, "Send")
		err = mail.Send(c, &mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		assert.Loosely(t, err, should.ErrLike(`"Send" is broken`))

		assert.Loosely(t, ctr.Send, convey.Adapt(shouldHaveSuccessesAndErrors)(1, 1))
	})
}

func ExampleFilterRDS() {
	// Set up your context using a base service implementation (memory or prod)
	c := memory.Use(context.Background())

	// Apply the counter.FilterRDS
	c, counter := FilterRDS(c)

	// functions use ds from the context like normal... they don't need to know
	// that there are any filters at all.
	someCalledFunc := func(c context.Context) {
		vals := []ds.PropertyMap{{
			"FieldName": ds.MkProperty(100),
			"$key":      ds.MkProperty(ds.NewKey(c, "Kind", "", 1, nil))},
		}
		if err := ds.Put(c, vals); err != nil {
			panic(err)
		}
	}

	// Using the other function.
	someCalledFunc(c)
	someCalledFunc(c)

	// Then we can see what happened!
	fmt.Printf("%d\n", counter.PutMulti.Successes())
	// Output:
	// 2
}
