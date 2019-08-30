package null

import (
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
)

// Output implements the butler output.Output interface, but sends the data
// nowhere.
//
// Output does collect stats, however.
type Output struct {
	stats output.StatsBase
}

var _ output.Output = (*Output)(nil)

// SendBundle implements output.Output
func (o *Output) SendBundle(b *logpb.ButlerLogBundle) error {
	o.stats.F.SentMessages += int64(len(b.Entries))
	o.stats.F.SentBytes += int64(proto.Size(b))
	return nil
}

// Stats implements output.Output
func (o *Output) Stats() output.Stats {
	return &o.stats
}

// URLConstructionEnv implements output.Output
func (o *Output) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{}
}

// MaxSize returns a large number instead of 0 because butler has bugs.
func (o *Output) MaxSize() int { return 1024 * 1024 * 1024 }

// Close implements output.Output
func (o *Output) Close() {}
