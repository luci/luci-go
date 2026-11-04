// Copyright 2026 The LUCI Authors.
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

import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  CatalogDiffItem,
  CatalogEditableFieldKey,
  CatalogEditableFields,
  computeCatalogSpecDiffs,
  createCatalogEditableDraft,
  validateAllCatalogSpecFields,
} from './catalog_edit_validation';
import { UnifiedProductCatalogEntry } from './types';

const EMPTY_SPEC_ERRORS: Partial<Record<CatalogEditableFieldKey, string>> = {};
const EMPTY_SPEC_DIFFS: readonly CatalogDiffItem[] = [];

const EMPTY_SPEC_DRAFT: CatalogEditableFields = {
  productName: '',
  descriptiveName: '',
  gpn: '',
  resourceType: '',
  fleetPlmStatus: '',
  r11n: '',
  numberOfDevicesPerRack: '',
  unitCost: '',
};

export function applyCatalogSpecDraft(
  entry: UnifiedProductCatalogEntry,
  draft: CatalogEditableFields,
): UnifiedProductCatalogEntry {
  const rawRackCount = draft.numberOfDevicesPerRack.trim();
  const parsedRackCount = Number(rawRackCount);
  return {
    ...entry,
    productName: draft.productName.trim(),
    descriptiveName: draft.descriptiveName.trim(),
    gpn: draft.gpn.trim(),
    resourceType: draft.resourceType.trim(),
    fleetPlmStatus: draft.fleetPlmStatus.trim(),
    r11n: draft.r11n
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
    numberOfDevicesPerRack:
      rawRackCount !== '' && !Number.isNaN(parsedRackCount)
        ? parsedRackCount
        : undefined,
    unitCost: draft.unitCost.trim(),
  };
}

export interface UseCatalogSpecEditStateResult {
  readonly isEditingSpecs: boolean;
  readonly baselineSpecDraft: CatalogEditableFields;
  readonly activeSpecDraft: CatalogEditableFields;
  readonly specErrors: Partial<Record<CatalogEditableFieldKey, string>>;
  readonly specErrorMessages: readonly string[];
  readonly specDiffs: readonly CatalogDiffItem[];
  readonly displayEntry: UnifiedProductCatalogEntry | undefined;
  readonly handleToggleEditingSpecs: () => void;
  readonly handleStartEditingSpecs: () => void;
  readonly handleStopEditingSpecs: () => void;
  readonly handleSpecFieldChange: (
    field: CatalogEditableFieldKey,
    value: string,
  ) => void;
  readonly handleResetSpecs: () => void;
}

export const useCatalogSpecEditState = (
  id: string,
  entry?: UnifiedProductCatalogEntry,
): UseCatalogSpecEditStateResult => {
  const [isEditingSpecs, setIsEditingSpecs] = useState(false);
  const [specDraftOverride, setSpecDraftOverride] = useState<
    CatalogEditableFields | undefined
  >(undefined);

  const handleResetSpecs = useCallback(() => {
    setSpecDraftOverride(undefined);
  }, []);

  useEffect(() => {
    setSpecDraftOverride(undefined);
    setIsEditingSpecs(false);
  }, [id]);

  const baselineSpecDraft = useMemo(
    () => (entry ? createCatalogEditableDraft(entry) : EMPTY_SPEC_DRAFT),
    [entry],
  );
  const activeSpecDraft = specDraftOverride ?? baselineSpecDraft;

  const specErrors = useMemo(
    () =>
      specDraftOverride
        ? validateAllCatalogSpecFields(activeSpecDraft, baselineSpecDraft)
        : EMPTY_SPEC_ERRORS,
    [activeSpecDraft, baselineSpecDraft, specDraftOverride],
  );

  const specErrorMessages = useMemo(
    () =>
      Object.values(specErrors).filter((msg): msg is string => Boolean(msg)),
    [specErrors],
  );

  const specDiffs = useMemo(
    () =>
      entry && specDraftOverride
        ? computeCatalogSpecDiffs(entry, activeSpecDraft)
        : EMPTY_SPEC_DIFFS,
    [activeSpecDraft, entry, specDraftOverride],
  );

  const displayEntry = useMemo<UnifiedProductCatalogEntry | undefined>(() => {
    if (!entry) return undefined;
    if (!specDraftOverride) return entry;
    return applyCatalogSpecDraft(entry, activeSpecDraft);
  }, [activeSpecDraft, entry, specDraftOverride]);

  const handleToggleEditingSpecs = useCallback(() => {
    setIsEditingSpecs((prev) => !prev);
  }, []);

  const handleStartEditingSpecs = useCallback(() => {
    setIsEditingSpecs(true);
  }, []);

  const handleStopEditingSpecs = useCallback(() => {
    setIsEditingSpecs(false);
  }, []);

  const handleSpecFieldChange = useCallback(
    (field: CatalogEditableFieldKey, value: string) => {
      setSpecDraftOverride((prev) => ({
        ...(prev ?? baselineSpecDraft),
        [field]: value,
      }));
    },
    [baselineSpecDraft],
  );

  return useMemo(
    () => ({
      isEditingSpecs,
      baselineSpecDraft,
      activeSpecDraft,
      specErrors,
      specErrorMessages,
      specDiffs,
      displayEntry,
      handleToggleEditingSpecs,
      handleStartEditingSpecs,
      handleStopEditingSpecs,
      handleSpecFieldChange,
      handleResetSpecs,
    }),
    [
      isEditingSpecs,
      baselineSpecDraft,
      activeSpecDraft,
      specErrors,
      specErrorMessages,
      specDiffs,
      displayEntry,
      handleToggleEditingSpecs,
      handleStartEditingSpecs,
      handleStopEditingSpecs,
      handleSpecFieldChange,
      handleResetSpecs,
    ],
  );
};
