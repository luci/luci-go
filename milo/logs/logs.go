package logs

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/fetcher"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
)

var errNoAuth = errors.New("no access")

type streamInfo struct {
	Project cfgtypes.ProjectName
	Path    types.StreamPath

	// Client is the HTTP client to use for LogDog communication.
	Client *coordinator.Client
}

const noStreamDelay = 5 * time.Second

// coordinatorSource is a fetcher.Source implementation that uses the
// Coordiantor API.
type coordinatorSource struct {
	sync.Mutex

	stream    *coordinator.Stream
	tidx      types.MessageIndex
	tailFirst bool

	streamState *coordinator.LogStream
}

func (s *coordinatorSource) LogEntries(c context.Context, req *fetcher.LogRequest) (
	[]*logpb.LogEntry, types.MessageIndex, error) {
	s.Lock()
	// TODO(hinoka): If fetching multiple streams, this would cause requests
	// to be serialized. We may not want this.
	defer s.Unlock()

	params := append(make([]coordinator.GetParam, 0, 4),
		coordinator.LimitBytes(int(req.Bytes)),
		coordinator.LimitCount(req.Count),
		coordinator.Index(req.Index),
	)

	// If we haven't terminated, use this opportunity to fetch/update our stream
	// state.
	var streamState coordinator.LogStream
	reqState := s.streamState == nil || s.streamState.State.TerminalIndex < 0
	if reqState {
		params = append(params, coordinator.WithState(&streamState))
	}

	for {
		logs, err := s.stream.Get(c, params...)
		switch err {
		case nil:
			if reqState {
				s.streamState = &streamState
				s.tidx = streamState.State.TerminalIndex
			}
			return logs, s.tidx, nil

		case coordinator.ErrNoSuchStream:
			log.Print("Stream does not exist. Sleeping pending registration.")

			// Delay, interrupting if our Context is interrupted.
			if tr := <-clock.After(c, noStreamDelay); tr.Incomplete() {
				return nil, 0, tr.Err
			}

		default:
			return nil, 0, err
		}
	}
}

func logHandler(c context.Context, w http.ResponseWriter, host, path string) error {
	// TODO(hinoka): Move this to luci-config.
	if !(host == "luci-logdog.appspot.com" || host == "luci-logdog-dev.appspot.com") {
		return fmt.Errorf("unknown host %s", host)
	}
	spath := strings.SplitN(path, "/", 2)
	if len(spath) != 2 {
		return fmt.Errorf("%s is not a valid path", path)
	}
	project := cfgtypes.ProjectName(spath[0])
	streamPath := types.StreamPath(spath[1])

	client := coordinator.NewClient(&prpc.Client{
		C: &http.Client{
			// TODO(hinoka): Once crbug.com/712506 is resolved, figure out how to get auth.
			Transport: http.DefaultTransport,
		},
		Host: host,
	})
	stream := client.Stream(project, streamPath)

	// Pull stream information.
	f := fetcher.New(c, fetcher.Options{
		Source: &coordinatorSource{
			stream: stream,
			tidx:   -1, // Must be set to probe for state.
		},
		Index: types.MessageIndex(0),
		Count: 0,
		// Try to buffer as much as possible, with a large window, since this is
		// basically a cloud-to-cloud connection.
		BufferCount:    200,
		BufferBytes:    int64(4 * 1024 * 1024),
		PrefetchFactor: 10,
	})

	for {
		// Read out of the buffer.  This _should_ be bottlenecked on the network
		// connection between the Flex instance and the client, via Fprintf().
		entry, err := f.NextLogEntry()
		switch err {
		case io.EOF:
			return nil // We're done.
		case nil:
			// Nothing
		case coordinator.ErrNoAccess:
			return errNoAuth // This will force a redirect
		default:
			return err
		}
		content := entry.GetText()
		if content == nil {
			break
		}
		for _, line := range content.Lines {
			fmt.Fprint(w, line.Value)
			fmt.Fprint(w, line.Delimiter)
		}
	}
	return nil
}
