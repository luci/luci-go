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
		chLog := make(chan *logdog.GetResponse, maxBuffer)
		chErr := make(chan error)
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
			s := s
			c = WithServer(c, &s)
			go fetch(c, chLog, chErr, project, path)
			// We should get 2 messages.  First one contains the full log, second
			// is a nil err.
			select {
			case resp := <-chLog:
				So(resp.State.TerminalIndex, ShouldEqual, 7)
				So(len(resp.Logs), ShouldEqual, 2)
			case err := <-chErr:
				t.Fatal(err)
			}

			select {
			case <-chLog:
				t.Fatal("Should not get log")
			case err := <-chErr:
				So(err, ShouldBeNil)
			}
		})

		Convey(`Error Case`, func() {
			fakeError := errors.New("fake error")
			s := fakeServer{err: fakeError}
			c = WithServer(c, &s)
			go fetch(c, chLog, chErr, project, path)
			// We should get 2 messages.  First one contains the full log, second
			// is a nil err.
			select {
			case <-chLog:
				t.Fatal("Should not load log")
			case err := <-chErr:
				So(err, ShouldEqual, fakeError)
			}
		})

		Convey(`Double fetch`, func() {
			s := s
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
			c = WithServer(c, &s)
			go fetch(c, chLog, chErr, project, path)

			// We should get 3 messages.  First two containing logs, third
			// is a nil err.
			select {
			case resp := <-chLog:
				So(resp.State.TerminalIndex, ShouldEqual, 20)
				So(len(resp.Logs), ShouldEqual, 2)
				So(resp.Logs[0].StreamIndex, ShouldEqual, 1)
				So(resp.Logs[1].StreamIndex, ShouldEqual, 7)
			case err := <-chErr:
				t.Fatal(err)
			}

			select {
			case resp := <-chLog:
				So(resp.State.TerminalIndex, ShouldEqual, 20)
				So(len(resp.Logs), ShouldEqual, 1)
				So(resp.Logs[0].StreamIndex, ShouldEqual, 20)
			case err := <-chErr:
				t.Fatal(err)
			}

			select {
			case <-chLog:
				t.Fatal("Should not get log")
			case err := <-chErr:
				So(err, ShouldBeNil)
			}
		})
	})
}
