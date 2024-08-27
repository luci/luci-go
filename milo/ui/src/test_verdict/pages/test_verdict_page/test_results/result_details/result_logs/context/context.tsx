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

import { ReactNode, createContext } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { useInvArtifacts, useResultArtifacts } from '../../context';

const SELECTED_ARTIFACT_PARAM = 'selectedArtifact';
const SELECTED_ARTIFACT_SOURCE_PARAM = 'selectedArtifactSource';

export type SelectedArtifactSource = 'result' | 'invocation';

export interface ResultLogsContext {
  selectedArtifact: Artifact | null;
  updateSelectedArtifact: (
    artifact: Artifact | null,
    source: SelectedArtifactSource | null,
  ) => void;
}

export const ResultLogsCtx = createContext<ResultLogsContext | null>(null);

interface ResultLogsProps {
  children: ReactNode;
}

export function ResultLogsProvider({ children }: ResultLogsProps) {
  const resultArtifacts = useResultArtifacts();
  const invArtifacts = useInvArtifacts();
  const [urlSearchParams, setURLSearchParams] = useSyncedSearchParams();
  const selectedArtifactParam = urlSearchParams.get(SELECTED_ARTIFACT_PARAM);
  const selectedArtifactSourceParam = urlSearchParams.get(
    SELECTED_ARTIFACT_SOURCE_PARAM,
  );

  const artifacts =
    selectedArtifactSourceParam === 'invocation'
      ? invArtifacts
      : resultArtifacts;
  const selectedArtifact =
    artifacts.find(
      (artifact) => artifact.artifactId === selectedArtifactParam,
    ) || null;

  function updateSelectedArtifact(
    artifact: Artifact | null,
    source: SelectedArtifactSource | null,
  ) {
    if (artifact !== null) {
      setURLSearchParams(
        updateSelectedArtifactParam(artifact.artifactId, source),
      );
    } else {
      setURLSearchParams(updateSelectedArtifactParam('', source));
    }
  }

  return (
    <ResultLogsCtx.Provider
      value={{
        selectedArtifact,
        updateSelectedArtifact,
      }}
    >
      {children}
    </ResultLogsCtx.Provider>
  );
}

function updateSelectedArtifactParam(
  artifactId: string,
  source: SelectedArtifactSource | null,
) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (artifactId === '') {
      searchParams.delete(SELECTED_ARTIFACT_PARAM);
      searchParams.delete(SELECTED_ARTIFACT_SOURCE_PARAM);
    } else {
      searchParams.set(SELECTED_ARTIFACT_PARAM, artifactId);
      searchParams.set(SELECTED_ARTIFACT_SOURCE_PARAM, source || '');
    }
    return searchParams;
  };
}
