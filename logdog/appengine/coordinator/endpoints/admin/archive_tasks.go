package admin

import (
	"context"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/admin/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/mutations"
	"go.chromium.org/luci/tumble"

	"google.golang.org/grpc/codes"
)

type request struct {
	*logdog.ArchiveRequest
	start time.Time
	end   time.Time
}

func launch(c context.Context, lst *coordinator.LogStreamState) error {
	cat := mutations.CreateArchiveTask{
		ID:         lst.ID(),
		Expiration: clock.Now(c),
	}

	aeParent, aeName := datastore.KeyForObj(c, lst), cat.TaskName(c)
	if err := tumble.PutNamedMutations(c, aeParent, map[string]tumble.Mutation{aeName: &cat}); err != nil {
		logging.WithError(err).Errorf(c, "Failed to replace archive expiration mutation.")
		return grpcutil.Internal
	}
	return nil
}

func launchTasksInNamespace(c context.Context, r request, namespace string, count chan<- int64) error {
	logging.Debugf(c, "Running on namespace %s", namespace)

	q := datastore.NewQuery("LogStreamState")
	q = q.Gt("Created", r.start)
	q = q.Lt("Created", r.end)
	if r.ArchiveRequest.Limit != 0 {
		q = q.Limit(r.ArchiveRequest.Limit)
	}
	if r.ArchiveRequest.Offset != 0 {
		q = q.Offset(r.ArchiveRequest.Offset)
	}
	q1 := q.Eq("_ArchiveState", coordinator.NotArchived)
	q2 := q.Eq("_ArchiveState", coordinator.ArchiveTasked)

	err := datastore.Run(c, q1, func(lst *coordinator.LogStreamState) error {
		err := launch(c, lst)
		if err == nil {
			count <- 1
		}
		return err
	})
	if err != nil {
		return err
	}
	return datastore.Run(c, q2, func(lst *coordinator.LogStreamState) error {
		err := launch(c, lst)
		if err == nil {
			count <- 1
		}
		return err
	})
}

// LaunchArchiveTasks forcefully launches archive tasks for unarchived streams
// some number days back.
func (s *server) LaunchArchiveTasks(c context.Context, req *logdog.ArchiveRequest) (*logdog.ArchiveResponse, error) {
	logging.Debugf(c, "Got Request %q", req)
	r := request{ArchiveRequest: req}
	r.start = clock.Now(c).Add(time.Duration(req.StartDaysAgo) * 24 * -time.Hour)
	if req.EndDaysAgo < 1 {
		req.EndDaysAgo = 1 // For safety, since this maybe force terminate streams.
	}
	r.end = clock.Now(c).Add(-time.Hour * 24 * time.Duration(req.EndDaysAgo))
	nq := datastore.NewQuery("__namespace__").KeysOnly(true)
	var keys []*datastore.Key
	if err := datastore.GetAll(c, nq, &keys); err != nil {
		logging.WithError(err).Errorf(c, "Fetching namespaces")
		return nil, grpcutil.Errf(codes.Unknown, err.Error())
	}

	var countch chan int64
	var count int64
	go func() {
		for i := range countch {
			count += i
		}
	}()

	err := parallel.FanOutIn(func(ch chan<- func() error) {
		for _, k := range keys {
			logging.Debugf(c, "Key: %q", k)
			ch <- func() error {
				return launchTasksInNamespace(c, r, k.StringID(), countch)
			}
		}
	})
	logging.Infof(c, "Launched %d tasks", count)
	return &logdog.ArchiveResponse{NumTasked: count}, err
}
