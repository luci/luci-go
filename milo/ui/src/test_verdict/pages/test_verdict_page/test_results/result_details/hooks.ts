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

import { useContext } from 'react';

import { ResultDataCtx } from './context';

export function useResultArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useResultArtifacts can only be used in a ResultDataProvider',
    );
  }

  return ctx.resultArtifacts;
}

export function useInvArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error('useInvArtifacts can only be used in a ResultDataProvider');
  }

  return ctx.invArtifacts;
}

export function useCombinedArtifacts() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useCombinedArtifacts can only be used in a ResultDataProvider',
    );
  }

  return ctx.resultArtifacts.concat(ctx.invArtifacts);
}

export function useArtifactsLoading() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useArtifactsLoading can only be used in a ResultDataProvider',
    );
  }

  return ctx.artifactsLoading;
}

export function useResult() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error('useResult can only be used in a ResultDataProvider');
  }

  return ctx.result;
}

export function useTopPanelExpanded() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useTopPanelExpanded can only be used in a ResultDataProvider',
    );
  }

  return ctx.topPanelExpanded;
}

export function useSetTopPanelExpanded() {
  const ctx = useContext(ResultDataCtx);

  if (!ctx) {
    throw new Error(
      'useSetTopPanelExpanded can only be used in a ResultDataProvider',
    );
  }

  return ctx.setTopPanelExpanded;
}
