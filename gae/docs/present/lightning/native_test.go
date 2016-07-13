// +build native_appengine

package demo

import "testing"
import "google.golang.org/appengine/aetest"
import . "github.com/smartystreets/goconvey/convey"

// START OMIT

import "google.golang.org/appengine/datastore" // HL

func TestNative(t *testing.T) {
	type Model struct{ A, B int } // HL

	Convey("Put/Get", t, func() {
		ctx, shtap, err := aetest.NewContext()
		So(err, ShouldBeNil)
		defer shtap()

		keys := []*datastore.Key{
			datastore.NewKey(ctx, "Model", "one thing", 0, nil),  // HL
			datastore.NewKey(ctx, "Model", "or another", 0, nil), // HL
		}
		_, err = datastore.PutMulti(ctx, keys, []*Model{{10, 20}, {20, 30}}) // HL
		So(err, ShouldBeNil)

		ms := make([]*Model, 2)
		So(datastore.GetMulti(ctx, keys, ms), ShouldBeNil) // HL
		So(ms, ShouldResemble, []*Model{{10, 20}, {20, 30}})
	})
}

// END OMIT
