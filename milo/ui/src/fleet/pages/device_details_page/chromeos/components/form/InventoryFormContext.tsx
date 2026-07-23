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

/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useContext,
  ReactNode,
  useMemo,
  useCallback,
} from 'react';

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import {
  getEditableFields,
  isLabstationConfig,
} from '../../utils/inventory_editing_utils';

interface InventoryFormContextType {
  originalLse: MachineLSE | null;
  draftLse: MachineLSE | null;
  updateDraftFields: (
    updates: Array<{ path: string | string[]; value: unknown }>,
  ) => void;
  activeEditingCardId: string | null;
  setActiveEditingCardId: (id: string | null) => void;
  editable: boolean;
  isPathEditable: (path: string) => boolean;
}

const InventoryFormContext = createContext<InventoryFormContextType | null>(
  null,
);

export const useInventoryForm = () => {
  const context = useContext(InventoryFormContext);
  if (!context) {
    throw new Error(
      'useInventoryForm must be used within an InventoryFormProvider',
    );
  }
  return context;
};

interface InventoryFormProviderProps {
  originalLse: MachineLSE | null;
  draftLse: MachineLSE | null;
  updateDraftFields: (
    updates: Array<{ path: string | string[]; value: unknown }>,
  ) => void;
  activeEditingCardId: string | null;
  setActiveEditingCardId: (id: string | null) => void;
  editable?: boolean;
  children: ReactNode;
}

export const InventoryFormProvider = ({
  originalLse,
  draftLse,
  updateDraftFields,
  activeEditingCardId,
  setActiveEditingCardId,
  editable = false,
  children,
}: InventoryFormProviderProps) => {
  const isLabstation = isLabstationConfig(originalLse);
  const editableFields = useMemo(
    () => getEditableFields(isLabstation),
    [isLabstation],
  );

  const isPathEditable = useCallback(
    (path: string) => {
      return editableFields.some((field) => field.path === path);
    },
    [editableFields],
  );

  const value = useMemo(
    () => ({
      originalLse,
      draftLse,
      updateDraftFields,
      activeEditingCardId,
      setActiveEditingCardId,
      editable,
      isPathEditable,
    }),
    [
      originalLse,
      draftLse,
      updateDraftFields,
      activeEditingCardId,
      setActiveEditingCardId,
      editable,
      isPathEditable,
    ],
  );

  return (
    <InventoryFormContext.Provider value={value}>
      {children}
    </InventoryFormContext.Provider>
  );
};
