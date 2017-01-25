// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/luci/gae/service/info"
	swarm "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	googlepb "github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/retry"
	sv1 "github.com/luci/luci-go/dm/api/distributor/swarming/v1"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/distributor"
	"github.com/luci/luci-go/server/auth"
)

type swarmingDist struct {
	context.Context

	cfg *distributor.Config

	sCfg *sv1.Config
}

var _ distributor.D = (*swarmingDist)(nil)

var swarmBotLookup = map[string]dm.AbnormalFinish_Status{
	"BOT_DIED":  dm.AbnormalFinish_CRASHED,
	"CANCELED":  dm.AbnormalFinish_CANCELLED,
	"EXPIRED":   dm.AbnormalFinish_EXPIRED,
	"TIMED_OUT": dm.AbnormalFinish_TIMED_OUT,
}

func cipdPackageFromSwarm(pkg *swarm.SwarmingRpcsCipdPackage) *sv1.CipdPackage {
	return &sv1.CipdPackage{Name: pkg.PackageName, Version: pkg.Version}
}

func cipdSpecFromSwarm(pkgs *swarm.SwarmingRpcsCipdPins) *sv1.CipdSpec {
	ret := &sv1.CipdSpec{
		Client: cipdPackageFromSwarm(pkgs.ClientPackage),
		ByPath: map[string]*sv1.CipdSpec_CipdPackages{},
	}
	for _, pkg := range pkgs.Packages {
		pkgs, ok := ret.ByPath[pkg.Path]
		if !ok {
			pkgs = &sv1.CipdSpec_CipdPackages{}
			ret.ByPath[pkg.Path] = pkgs
		}
		pkgs.Pkg = append(pkgs.Pkg, cipdPackageFromSwarm(pkg))
	}
	return ret
}

func toSwarmMap(m map[string]string) []*swarm.SwarmingRpcsStringPair {
	ret := make([]*swarm.SwarmingRpcsStringPair, 0, len(m))
	for key, value := range m {
		ret = append(ret, &swarm.SwarmingRpcsStringPair{
			Key: key, Value: value})
	}
	return ret
}

func toIntSeconds(p *duration.Duration) int64 {
	return int64(googlepb.DurationFromProto(p).Seconds())
}

func httpClients(c context.Context) (anonC, authC *http.Client) {
	rt, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		// if we can't set up a transport, we're seriously hosed
		panic(err)
	}
	anonTransport, err := auth.GetRPCTransport(c, auth.NoAuth)
	if err != nil {
		panic(err)
	}
	anonC = &http.Client{Transport: anonTransport}
	authC = &http.Client{Transport: rt}
	return
}

func newSwarmClient(c context.Context, cfg *sv1.Config) *swarm.Service {
	_, authC := httpClients(c)
	svc, err := swarm.New(authC)
	if err != nil {
		// can only happen with nil client
		panic(err)
	}
	svc.BasePath = cfg.Swarming.Url + "/_ah/api/swarming/v1/"
	return svc
}

func parseParams(desc *dm.Quest_Desc) (ret *sv1.Parameters, err error) {
	ret = &sv1.Parameters{}
	if err = jsonpb.UnmarshalString(desc.DistributorParameters, ret); err != nil {
		err = errors.Annotate(err).
			Reason("unmarshalling DistributorParameters").
			InternalReason("These paramaeters were already validated?").
			Err()
		return
	}
	if err = ret.Normalize(); err != nil {
		err = errors.Annotate(err).
			Reason("normalizing DistributorParameters").
			InternalReason("These paramaeters were already normalized successfully once?").
			Err()
		return
	}
	return
}

func (d *swarmingDist) Run(desc *dm.Quest_Desc, auth *dm.Execution_Auth, prev *dm.JsonResult) (tok distributor.Token, _ time.Duration, err error) {
	id := auth.Id

	params, err := parseParams(desc)
	if err != nil {
		return
	}

	prevParsed := (*sv1.Result)(nil)
	if prev != nil {
		prevParsed = &sv1.Result{}
		if err = jsonpb.UnmarshalString(prev.Object, prevParsed); err != nil {
			err = errors.Annotate(err).Reason("parsing previous result").Err()
			return
		}
	}

	isoCtx, _ := context.WithTimeout(d, 30*time.Second)
	iso, err := prepIsolate(isoCtx, d.sCfg.Isolate.Url, desc, prev, params)
	if err != nil {
		err = errors.Annotate(err).Reason("prepping Isolated").Err()
		return
	}

	secretBytesRaw := &bytes.Buffer{}
	marshaller := &jsonpb.Marshaler{OrigName: true}
	if err = marshaller.Marshal(secretBytesRaw, auth); err != nil {
		return
	}
	secretBytes := base64.StdEncoding.EncodeToString(secretBytesRaw.Bytes())

	cipdInput := (*swarm.SwarmingRpcsCipdInput)(nil)
	if prevParsed != nil {
		cipdInput = prevParsed.CipdPins.ToCipdInput()
	} else {
		cipdInput = params.Job.Inputs.Cipd.ToCipdInput()
	}

	topic, token, err := d.cfg.PrepareTopic(d, auth.Id)
	if err != nil {
		err = errors.Annotate(err).Reason("preparing topic").Err()
		return
	}

	prefix := params.Meta.NamePrefix
	if len(prefix) > 0 {
		prefix += " / "
	}

	dims := make(map[string]string, len(params.Scheduling.Dimensions))
	for k, v := range params.Scheduling.Dimensions {
		dims[k] = v
	}
	if prevParsed != nil {
		for k, v := range prevParsed.SnapshotDimensions {
			dims[k] = v
		}
	}

	tags := []string{
		"requestor:DM",
		"requestor:" + info.TrimmedAppID(d),
		"requestor:swarming_v1",
		fmt.Sprintf("quest:%s", id.Quest),
		fmt.Sprintf("attempt:%s|%d", id.Quest, id.Attempt),
		fmt.Sprintf("execution:%s|%d|%d", id.Quest, id.Attempt, id.Id),
	}

	rslt := (*swarm.SwarmingRpcsTaskRequestMetadata)(nil)
	err = retry.Retry(d, retry.Default, func() (err error) {
		rpcCtx, _ := context.WithTimeout(d, 10*time.Second)
		rslt, err = newSwarmClient(rpcCtx, d.sCfg).Tasks.New(&swarm.SwarmingRpcsNewTaskRequest{
			ExpirationSecs: toIntSeconds(desc.Meta.Timeouts.Start),
			Name:           fmt.Sprintf("%s%s|%d|%d", prefix, id.Quest, id.Attempt, id.Id),

			Priority: int64(params.Scheduling.Priority),

			Properties: &swarm.SwarmingRpcsTaskProperties{
				CipdInput:            cipdInput,
				Dimensions:           toSwarmMap(dims),
				Env:                  toSwarmMap(params.Job.Env),
				ExecutionTimeoutSecs: toIntSeconds(desc.Meta.Timeouts.Run),
				GracePeriodSecs:      toIntSeconds(desc.Meta.Timeouts.Stop),
				IoTimeoutSecs:        toIntSeconds(params.Scheduling.IoTimeout),
				InputsRef:            iso,
				SecretBytes:          secretBytes,
			},

			PubsubTopic:     topic.String(),
			PubsubAuthToken: token,

			Tags: tags,
		}).Context(rpcCtx).Do()
		return
	}, retry.LogCallback(d, "swarm.Tasks.New"))
	if err != nil {
		err = errors.Annotate(err).Reason("calling swarm.Tasks.New").Err()
		return
	}

	tok = distributor.Token(rslt.TaskId)
	return
}

func (d *swarmingDist) Cancel(q *dm.Quest_Desc, tok distributor.Token) error {
	return retry.Retry(d, retry.Default, func() (err error) {
		ctx, _ := context.WithTimeout(d, 10*time.Second)
		_, err = newSwarmClient(ctx, d.sCfg).Task.Cancel(string(tok)).Context(ctx).Do()
		return
	}, retry.LogCallback(d, "swarm.Task.Cancel"))
}

func snapshotDimensions(p *sv1.Parameters, dims []*swarm.SwarmingRpcsStringListPair) map[string]string {
	if len(p.Scheduling.SnapshotDimensions) == 0 {
		return nil
	}
	allDims := map[string]string{}
	for _, dim := range dims {
		allDims[dim.Key] = dim.Value[len(dim.Value)-1]
	}

	ret := make(map[string]string, len(p.Scheduling.SnapshotDimensions))
	for _, k := range p.Scheduling.SnapshotDimensions {
		if v, ok := allDims[k]; ok {
			ret[k] = v
		}
	}
	return ret
}

func (d *swarmingDist) GetStatus(q *dm.Quest_Desc, tok distributor.Token) (*dm.Result, error) {
	rslt := (*swarm.SwarmingRpcsTaskResult)(nil)

	err := retry.Retry(d, retry.Default, func() (err error) {
		ctx, _ := context.WithTimeout(d, 10*time.Second)
		rslt, err = newSwarmClient(ctx, d.sCfg).Task.Result(string(tok)).Context(ctx).Do()
		return
	}, retry.LogCallback(d, fmt.Sprintf("swarm.Task.Result(%s)", tok)))
	if err != nil {
		if gerr := err.(*googleapi.Error); gerr != nil {
			if gerr.Code == http.StatusNotFound {
				return &dm.Result{
					AbnormalFinish: &dm.AbnormalFinish{
						Status: dm.AbnormalFinish_MISSING,
						Reason: "swarming: notFound",
					},
				}, nil
			}
		}

		return nil, err
	}
	ret := &dm.Result{}

	switch rslt.State {
	case "PENDING", "RUNNING":
		return nil, nil

	case "COMPLETED":
		if rslt.InternalFailure {
			ret.AbnormalFinish = &dm.AbnormalFinish{
				Status: dm.AbnormalFinish_CRASHED,
				Reason: fmt.Sprintf("swarming: COMPLETED/InternalFailure(%d)", rslt.ExitCode),
			}
			break
		}

		retData := &sv1.Result{
			ExitCode: rslt.ExitCode,
		}
		if rslt.CipdPins != nil {
			retData.CipdPins = cipdSpecFromSwarm(rslt.CipdPins)
		}
		params, err := parseParams(q)
		if err != nil {
			return nil, err
		}
		retData.SnapshotDimensions = snapshotDimensions(params, rslt.BotDimensions)

		if ref := rslt.OutputsRef; ref != nil {
			retData.IsolatedOutdir = &sv1.IsolatedRef{
				Id: ref.Isolated, Server: ref.Isolatedserver}
		}
		data, err := (&jsonpb.Marshaler{OrigName: true}).MarshalToString(retData)
		if err != nil {
			panic(err)
		}

		ret.Data = &dm.JsonResult{
			Object: data,
			Size:   uint32(len(data)),
			Expiration: googlepb.NewTimestamp(
				clock.Now(d).Add(googlepb.DurationFromProto(d.sCfg.Isolate.Expiration))),
		}

	default:
		if bad, ok := swarmBotLookup[rslt.State]; ok {
			ret.AbnormalFinish = &dm.AbnormalFinish{
				Status: bad,
				Reason: fmt.Sprintf("swarming: %s", rslt.State),
			}
		} else {
			ret.AbnormalFinish = &dm.AbnormalFinish{
				Status: dm.AbnormalFinish_RESULT_MALFORMED,
				Reason: fmt.Sprintf("swarming: unknown state %s", rslt.State),
			}
		}
	}

	return ret, nil
}

func (d *swarmingDist) InfoURL(tok distributor.Token) string {
	return fmt.Sprintf("%s/user/task/%s", d.sCfg.Swarming.Url, tok)
}

func (d *swarmingDist) HandleNotification(q *dm.Quest_Desc, notification *distributor.Notification) (*dm.Result, error) {
	type Data struct {
		TaskID distributor.Token `json:"task_id"`
	}
	dat := &Data{}
	if err := json.Unmarshal(notification.Data, dat); err != nil {
		logging.Fields{"payload": notification.Data}.Errorf(
			d, "Could not unmarshal swarming payload! relying on timeout.")
		return nil, nil
	}
	return d.GetStatus(q, dat.TaskID)
}

func (*swarmingDist) HandleTaskQueueTask(r *http.Request) ([]*distributor.Notification, error) {
	return nil, nil
}

func (*swarmingDist) Validate(payload string) error {
	msg := &sv1.Parameters{}
	if err := jsonpb.UnmarshalString(payload, msg); err != nil {
		return errors.Annotate(err).Reason("unmarshal").D("payload", payload).Err()
	}
	return errors.Annotate(msg.Normalize()).Reason("normalize").D("payload", payload).Err()
}

func factory(c context.Context, cfg *distributor.Config) (distributor.D, error) {
	return &swarmingDist{c, cfg, cfg.Content.(*sv1.Config)}, nil
}

// AddFactory adds this distributor implementation into the distributor
// Registry.
func AddFactory(m distributor.FactoryMap) {
	m[(*sv1.Config)(nil)] = factory
}
