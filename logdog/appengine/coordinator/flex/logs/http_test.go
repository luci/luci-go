package logs

import (
	"errors"
	"testing"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeServer struct {
	responses []*logdog.GetResponse
	index     int
	err       error
}

func (s *fakeServer) Get(c context.Context, r *logdog.GetRequest) (*logdog.GetResponse, error) {
	if s.index >= len(s.responses) {
		return nil, s.err
	}
	result := s.responses[s.index]
	s.index++
	return result, nil
}
func (s *fakeServer) Tail(c context.Context, r *logdog.TailRequest) (*logdog.GetResponse, error) {
	panic("This should not get called")
}
func (s *fakeServer) Query(c context.Context, r *logdog.QueryRequest) (*logdog.QueryResponse, error) {
	panic("This should not get called")
}

func TestHttp(t *testing.T) {
	Convey(`Testing http endpoint`, t, func() {
		c := context.Background()
		c = gologger.StdConfig.Use(c)
		project := "fakeproject"
		path := "/fake/prefix/+/fake/name"
		ch := make(chan logResp, maxBuffer)
		s := fakeServer{
			responses: []*logdog.GetResponse{
				{
					State: &logdog.LogStreamState{
						TerminalIndex: 7,
					},
					Logs: []*logpb.LogEntry{
						{
							StreamIndex: 1,
						},
						{
							StreamIndex: 7,
						},
					},
				},
			},
			err: errors.New("Should not happen"),
		}

		Convey(`Single fetch`, func() {
			c = withServer(c, &s)
			go fetch(c, ch, project, path)
			// We should get 2 messages.
			// First one contains the full log, second is a nil err.
			resp := <-ch
			So(resp.err, ShouldBeNil)
			So(resp.getResp.State.TerminalIndex, ShouldEqual, 7)
			So(len(resp.getResp.Logs), ShouldEqual, 2)

			resp = <-ch
			So(resp.getResp, ShouldBeNil)
			So(resp.err, ShouldBeNil)
		})

		Convey(`Error Case`, func() {
			fakeError := errors.New("fake error")
			s := fakeServer{err: fakeError}
			c = withServer(c, &s)
			go fetch(c, ch, project, path)
			// We should get 1 message: an error.
			resp := <-ch
			So(resp.getResp, ShouldBeNil)
			So(resp.err, ShouldEqual, fakeError)
		})

		Convey(`Double fetch`, func() {
			s.responses[0].State.TerminalIndex = 20
			s.responses = append(s.responses, &logdog.GetResponse{
				State: &logdog.LogStreamState{
					TerminalIndex: 20,
				},
				Logs: []*logpb.LogEntry{
					{
						StreamIndex: 20,
					},
				},
			})
			c = withServer(c, &s)
			go fetch(c, ch, project, path)

			// We should get 3 messages.
			// First two containing logs, third is a nil err.
			resp := <-ch
			So(resp.err, ShouldBeNil)
			So(resp.getResp.State.TerminalIndex, ShouldEqual, 20)
			So(len(resp.getResp.Logs), ShouldEqual, 2)
			So(resp.getResp.Logs[0].StreamIndex, ShouldEqual, 1)
			So(resp.getResp.Logs[1].StreamIndex, ShouldEqual, 7)

			resp = <-ch
			So(resp.err, ShouldBeNil)
			So(resp.getResp.State.TerminalIndex, ShouldEqual, 20)
			So(len(resp.getResp.Logs), ShouldEqual, 1)
			So(resp.getResp.Logs[0].StreamIndex, ShouldEqual, 20)

			resp = <-ch
			So(resp.err, ShouldBeNil)
			So(resp.getResp, ShouldBeNil)
		})
	})
}
