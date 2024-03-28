// Copyright 2024 The LUCI Authors.
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

package dsutils

import (
	"context"
	"fmt"
	"reflect"

	cloudds "cloud.google.com/go/datastore"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/sdf"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/log"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func init() {
	beam.RegisterType(reflect.TypeOf((*getEstimatedCountFn)(nil)).Elem())
	register.DoFn3x1(&getEstimatedCountFn{})
	register.Emitter1[NamespaceCount]()

	beam.RegisterType(reflect.TypeOf((*getAllKeysWithHexPrefixFn)(nil)).Elem())
	register.DoFn4x1(&getAllKeysWithHexPrefixFn{})
	register.Emitter1[KeyBatch]()
}

type ReadOptions struct {
	// HexPrefixLength is the minimum guaranteed length of hex prefix in
	// `ParentKind`'s key. The hex prefix will be used to divide the read queries
	// to make it run faster should the beam runner decides more parallelism is
	// needed.
	HexPrefixLength int
	// OutputBatchSize controls the number of datastore keys in a given output
	// batch.
	OutputBatchSize int
	// MinEstimatedCount is the count used as a floor when estimating the number
	// of entities of the specified Kind in a namespace when the stats is not
	// available or is too small. This can happen when the kind was just added to
	// the namespace and stats hasn't been updated yet.
	MinEstimatedCount int64
}

// GetAllKeysWithHexPrefix queries all the keys from datastore for the given
// kind in the given namespaces (in a PCollection<string>) and returns
// PCollection<KeyBatch>.
func GetAllKeysWithHexPrefix(
	s beam.Scope,
	cloudProject string,
	namespaces beam.PCollection,
	kind string,
	opts ReadOptions,
) beam.PCollection {
	if opts.OutputBatchSize < 1 {
		opts.OutputBatchSize = 1
	}

	s = s.Scope(fmt.Sprintf("datastore.GetAllKeysWithHexPrefix.%s.%s", cloudProject, kind))
	namespaces = beam.Reshuffle(s, namespaces)
	namespacesWithCount := beam.ParDo(s, &getEstimatedCountFn{
		CloudProject:      cloudProject,
		Kind:              kind,
		MinEstimatedCount: opts.MinEstimatedCount,
	}, namespaces)

	return beam.ParDo(s, &getAllKeysWithHexPrefixFn{
		CloudProject:    cloudProject,
		Kind:            kind,
		HexPrefixLength: opts.HexPrefixLength,
		OutputBatchSize: opts.OutputBatchSize,
	}, namespacesWithCount)
}

type getEstimatedCountFn struct {
	CloudProject      string
	Kind              string
	MinEstimatedCount int64

	withDatastoreEnv func(context.Context) context.Context
}

// Setup implements beam DoFn protocol.
func (fn *getEstimatedCountFn) Setup(ctx context.Context) error {
	if fn.withDatastoreEnv == nil {
		client, err := cloudds.NewClient(ctx, fn.CloudProject)
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.withDatastoreEnv = func(ctx context.Context) context.Context {
			return (&cloud.ConfigLite{
				ProjectID: fn.CloudProject,
				DS:        client,
			}).Use(ctx)
		}
	}

	return nil
}

type NamespaceCount struct {
	Namespace      string
	EstimatedCount int64
}

type kindStat struct {
	Key   *datastore.Key        `gae:"$key"`
	Count int64                 `gae:"count,noindex"`
	Extra datastore.PropertyMap `gae:",extra"`
}

// ProcessElement implements beam DoFn protocol.
func (fn *getEstimatedCountFn) ProcessElement(
	ctx context.Context,
	namespace string,
	emit func(NamespaceCount),
) error {
	ctx = fn.withDatastoreEnv(ctx)
	ctx, err := info.Namespace(ctx, namespace)
	if err != nil {
		return errors.Annotate(err, "failed to apply namespace: %s", namespace).Err()
	}

	stat := kindStat{
		Key:   datastore.MakeKey(ctx, "__Stat_Ns_Kind__", fn.Kind),
		Count: 0,
	}
	err = datastore.Get(ctx, &stat)
	if err != nil {
		if !errors.Is(err, datastore.ErrNoSuchEntity) {
			return errors.Annotate(err, "failed to query stats for kind `%s` in namespace `%s`", fn.Kind, namespace).Err()
		}
	} else {
		log.Infof(ctx, "Datastore: there are `%d` `%s` in namespace `%s`.", stat.Count, fn.Kind, namespace)
	}

	if stat.Count < fn.MinEstimatedCount {
		log.Warnf(ctx, "Datastore: there are only `%d` `%s` recorded in namespace `%s`. The minimum size `%d` will be used.",
			stat.Count, fn.Kind, namespace, fn.MinEstimatedCount)
		stat.Count = fn.MinEstimatedCount
	} else {
		log.Infof(ctx, "Datastore: there are `%d` `%s` in namespace `%s`.", stat.Count, fn.Kind, namespace)
	}

	emit(NamespaceCount{Namespace: namespace, EstimatedCount: stat.Count})
	return nil
}

type getAllKeysWithHexPrefixFn struct {
	CloudProject string
	Kind         string

	HexPrefixLength int
	OutputBatchSize int

	withDatastoreEnv func(context.Context) context.Context
	emittedKeys      beam.Counter
	emittedBatches   beam.Counter
}

// Setup implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) Setup(ctx context.Context) error {
	if fn.withDatastoreEnv == nil {
		client, err := cloudds.NewClient(ctx, fn.CloudProject)
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.withDatastoreEnv = func(ctx context.Context) context.Context {
			return (&cloud.ConfigLite{
				ProjectID: fn.CloudProject,
				DS:        client,
			}).Use(ctx)
		}
	}

	namespace := fmt.Sprintf("datastore.get-all-keys-with-hex-prefix.%s.%s", fn.CloudProject, fn.Kind)
	fn.emittedKeys = beam.NewCounter(namespace, "emitted-keys")
	fn.emittedBatches = beam.NewCounter(namespace, "emitted-batches")

	return nil
}

// CreateInitialRestriction implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) CreateInitialRestriction(ctx context.Context, nc NamespaceCount) hexPrefixRestriction {
	return hexPrefixRestriction{
		HexPrefixLength: fn.HexPrefixLength,

		StartIsExclusive: false,
		Start:            "",

		EndIsUnbounded: true,
		EndIsExclusive: false,
		End:            "",
	}
}

// CreateTracker implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) CreateTracker(ctx context.Context, restriction hexPrefixRestriction) *sdf.LockRTracker {
	return sdf.NewLockRTracker(newHexPrefixRestrictionTracker(restriction))
}

// SplitRestriction implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) SplitRestriction(ctx context.Context, nc NamespaceCount, restriction hexPrefixRestriction) (splits []hexPrefixRestriction) {
	// Return the restriction as is. Let the runner initiate the splits.
	return []hexPrefixRestriction{restriction}
}

// RestrictionSize implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) RestrictionSize(ctx context.Context, nc NamespaceCount, restriction hexPrefixRestriction) float64 {
	return restriction.Ratio() * float64(nc.EstimatedCount)
}

type KeyBatch struct {
	Namespace string
	Keys      []*datastore.Key
}

// ProcessElement implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) ProcessElement(
	ctx context.Context,
	rt *sdf.LockRTracker,
	nc NamespaceCount,
	emit func(KeyBatch),
) error {
	ctx = fn.withDatastoreEnv(ctx)
	ctx, err := info.Namespace(ctx, nc.Namespace)
	if err != nil {
		return errors.Annotate(err, "failed to apply namespace: %s", nc.Namespace).Err()
	}

	restriction := rt.GetRestriction().(hexPrefixRestriction)
	log.Infof(ctx, "Datastore: processing Namespace `%s` Range %s", nc.Namespace, restriction.RangeString())

	q := datastore.NewQuery(fn.Kind).KeysOnly(true)
	// If start == "", its practically unbounded. We don't need to apply the
	// filter. And we cannot apply an empty key anyway otherwise datastore will
	// report an error.
	if restriction.Start != "" {
		startKey := datastore.MakeKey(ctx, fn.Kind, restriction.Start)
		if restriction.StartIsExclusive {
			q = q.Gt("__key__", startKey)
		} else {
			q = q.Gte("__key__", startKey)
		}
	}
	if !restriction.EndIsUnbounded {
		// Key token cannot be empty otherwise datastore will report an error. When
		// end is bounded to "", nothing can be smaller than it. Short-circuit it.
		if restriction.End == "" {
			return nil
		}
		endKey := datastore.MakeKey(ctx, fn.Kind, restriction.End)
		if restriction.EndIsExclusive {
			q = q.Lt("__key__", endKey)
		} else {
			q = q.Lte("__key__", endKey)
		}
	}

	claimedKeys := make([]*datastore.Key, 0, fn.OutputBatchSize)
	emitClaimedKeys := func() {
		if len(claimedKeys) == 0 {
			return
		}

		// We cannot batch keys in the same namespace in a later stage without
		// using a GBK (GroupByKey) or something similar. We want to avoid GBK
		// because
		// 1. GBK prevents stage fusion, which leads to unnecessary IO between
		//    stages.
		// 2. GBK can lead to OOM when certain keys are very large.
		// 3. In batch mode, GBK stops the next stage from executing until all
		//    elements are collected.
		//
		// Therefore, we need to emit batches instead of individual keys here.
		emit(KeyBatch{Namespace: nc.Namespace, Keys: claimedKeys})
		fn.emittedKeys.Inc(ctx, int64(len(claimedKeys)))
		fn.emittedBatches.Inc(ctx, 1)
		claimedKeys = make([]*datastore.Key, 0, fn.OutputBatchSize)
	}
	// We already claimed these keys from the restriction tracker. Always emit the
	// final batch of claimed keys, even when there was an error.
	defer emitClaimedKeys()

	err = datastore.Run(ctx, q, func(key *datastore.Key) error {
		if !rt.TryClaim(HexPosClaim{Value: key.StringID()}) {
			return datastore.Stop
		}

		claimedKeys = append(claimedKeys, key)
		if len(claimedKeys) < fn.OutputBatchSize {
			return nil
		}
		emitClaimedKeys()
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to run bounded query Namespace `%s` Range: `%s`", nc.Namespace, restriction.RangeString()).Err()
	}
	rt.TryClaim(HexPosClaim{End: true})

	// The restriction might have been split. Log the actual restriction we
	// completed.
	finalRestriction := rt.GetRestriction().(hexPrefixRestriction)
	log.Infof(ctx, "Datastore: finished processing Namespace `%s` Range %s (was %s)", nc.Namespace, finalRestriction.RangeString(), restriction.RangeString())

	return nil
}
