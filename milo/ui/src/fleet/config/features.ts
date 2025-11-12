// Copyright 2025 The LUCI Authors.
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

import { isProdEnvironment } from '../utils/utils';

import { features as featuresDev, FeaturesSchema } from './features_dev';
import { features as featuresProd } from './features_prod';

const FEATURES_LOCAL_STORAGE_KEY = 'FEATURE_FLAG_OVERRIDES';

const hydrateLocalStorage = () => {
  const object = (Object.keys(featuresProd) as (keyof FeaturesSchema)[]).reduce(
    (acc, key) => {
      acc[key] = getFromLocalStorage(key);
      return acc;
    },
    {} as FeaturesSchema,
  );
  localStorage.setItem(FEATURES_LOCAL_STORAGE_KEY, JSON.stringify(object));
};

const getFromLocalStorage = (feature: keyof FeaturesSchema) => {
  const localStorageFeatures = localStorage.getItem(FEATURES_LOCAL_STORAGE_KEY);

  if (!localStorageFeatures) return null;
  const value = JSON.parse(localStorageFeatures)[feature];
  if (typeof value === 'boolean') {
    return value;
  }
  return null;
};

const getFromFiles = (feature: keyof FeaturesSchema) =>
  isProdEnvironment() ? featuresProd[feature] : featuresDev[feature];

hydrateLocalStorage();

export const getFeatureFlag = (feature: keyof FeaturesSchema) => {
  return getFromLocalStorage(feature) ?? getFromFiles(feature);
};

export const getEnabledFeatureFlags = () => {
  const features = isProdEnvironment() ? featuresProd : featuresDev;
  return Object.keys(features).filter((feature) =>
    getFeatureFlag(feature as keyof FeaturesSchema),
  );
};
