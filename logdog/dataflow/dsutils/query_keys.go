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
	beam.RegisterType(reflect.TypeOf((*getAllKeysWithHexPrefixFn)(nil)).Elem())
	register.DoFn4x1(&getAllKeysWithHexPrefixFn{})
	register.Emitter2[string, NamespaceKey]()
}

type ReadOptions struct {
	// ParentKind is the kind where the hex prefix is used. It
	// 1. should equal to the `kind`  being queried, or
	// 2. is the only ancestor of the `kind`.
	// In case 2, the child should always have a key of `1`.
	ParentKind string
	// HexPrefixLength is the minimum guaranteed length of hex prefix in
	// `ParentKind`'s key. The hex prefix will be used to divide the read queries
	// to make it run faster should the beam runner decides more parallelism is
	// needed.
	HexPrefixLength int
	// OutputShards controls the number of beam keys per namespace the entities
	// are divided into. Dividing the entities into more shards can increase the
	// maximum parallelism in the downstream should the pipeline need to perform
	// actions on those entities.
	OutputShards int
}

// GetAllKeysWithHexPrefix queries all the keys from datastore for the given
// kind in the given namespaces (in a PCollection<string>.) and returns
// PCollection<KV<string, NamespaceKey>> where the element key is
// `${namespace}-${shard}`.
func GetAllKeysWithHexPrefix(s beam.Scope, project string, namespaces beam.PCollection, kind string, opts ReadOptions) beam.PCollection {
	if opts.OutputShards < 1 {
		opts.OutputShards = 1
	}

	s = s.Scope(fmt.Sprintf("datastore.GetAllKeysWithHexPrefix.%s.%s", project, kind))
	namespaces = beam.Reshuffle(s, namespaces)
	return beam.ParDo(s, &getAllKeysWithHexPrefixFn{
		Project:         project,
		ParentKind:      opts.ParentKind,
		Kind:            kind,
		HexPrefixLength: opts.HexPrefixLength,
		Shards:          opts.OutputShards,
	}, namespaces)
}

type getAllKeysWithHexPrefixFn struct {
	Project string
	Kind    string

	ParentKind      string
	HexPrefixLength int
	Shards          int

	withDatastoreEnv func(context.Context) context.Context
	emitted          beam.Counter
}

// Setup implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) Setup(ctx context.Context) error {
	if fn.withDatastoreEnv == nil {
		client, err := cloudds.NewClient(ctx, fn.Project)
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.withDatastoreEnv = func(ctx context.Context) context.Context {
			return (&cloud.ConfigLite{
				ProjectID: fn.Project,
				DS:        client,
			}).Use(ctx)
		}
	}

	namespace := fmt.Sprintf("datastore.get-all-keys-with-hex-prefix.%s.%s", fn.Project, fn.Kind)
	fn.emitted = beam.NewCounter(namespace, "emitted")

	return nil
}

// CreateInitialRestriction implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) CreateInitialRestriction(ctx context.Context, namespace string) hexPrefixRestriction {
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
func (fn *getAllKeysWithHexPrefixFn) SplitRestriction(ctx context.Context, namespace string, restriction hexPrefixRestriction) (splits []hexPrefixRestriction) {
	// Return the restriction as is. Let the runner initiate the splits.
	return []hexPrefixRestriction{restriction}
}

// RestrictionSize implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) RestrictionSize(ctx context.Context, namespace string, restriction hexPrefixRestriction) float64 {
	return restriction.EstimatedSize()
}

type NamespaceKey struct {
	Namespace string
	Key       *datastore.Key
}

// ProcessElement implements beam DoFn protocol.
func (fn *getAllKeysWithHexPrefixFn) ProcessElement(
	ctx context.Context,
	rt *sdf.LockRTracker,
	namespace string,
	emit func(string, NamespaceKey),
) error {
	ctx = fn.withDatastoreEnv(ctx)
	ctx, err := info.Namespace(ctx, namespace)
	if err != nil {
		return errors.Annotate(err, "failed to apply namespace: %s", namespace).Err()
	}

	restriction := rt.GetRestriction().(hexPrefixRestriction)
	log.Infof(ctx, "Datastore: processing Namespace `%s` Range %s", namespace, restriction.RangeString())

	q := datastore.NewQuery(fn.Kind).KeysOnly(true)
	// If start == "", its practically unbounded. We don't need to apply the
	// filter. And we cannot apply an empty key anyway otherwise datastore will
	// report an error.
	if restriction.Start != "" {
		startKey := datastore.MakeKey(ctx, fn.ParentKind, restriction.Start)
		if fn.ParentKind != fn.Kind {
			startKey = datastore.MakeKey(ctx, fn.ParentKind, restriction.Start, fn.Kind, 1)
		}
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
		endKey := datastore.MakeKey(ctx, fn.ParentKind, restriction.End)
		if fn.ParentKind != fn.Kind {
			endKey = datastore.MakeKey(ctx, fn.ParentKind, restriction.End, fn.Kind, 1)
		}
		if restriction.EndIsExclusive {
			q = q.Lt("__key__", endKey)
		} else {
			q = q.Lte("__key__", endKey)
		}
	}

	i := 0
	err = datastore.Run(ctx, q, func(key *datastore.Key) error {
		pivot := key
		for pivot.Kind() != fn.ParentKind {
			pivot = pivot.Parent()
		}

		if !rt.TryClaim(HexPosClaim{Value: pivot.StringID()}) {
			return datastore.Stop
		}

		// Re-key the element to avoid hot spotting if later operations decided to
		// perform updates on the keys.
		eleKey := fmt.Sprintf("%s-%d", namespace, (i)%fn.Shards)
		emit(eleKey, NamespaceKey{Namespace: namespace, Key: key})
		fn.emitted.Inc(ctx, 1)
		i += 1
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to run bounded query Namespace `%s` Range: `%s`", namespace, restriction.RangeString()).Err()
	}
	rt.TryClaim(HexPosClaim{End: true})

	// The restriction might have been split. Log the actual restriction we
	// completed.
	finalRestriction := rt.GetRestriction().(hexPrefixRestriction)
	log.Infof(ctx, "Datastore: finished processing Namespace `%s` Range %s (was %s)", namespace, finalRestriction.RangeString(), restriction.RangeString())

	return nil
}
