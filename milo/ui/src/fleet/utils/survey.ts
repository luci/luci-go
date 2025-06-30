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

import { AuthState } from '@/common/api/auth_state';

declare global {
  interface Window {
    help: {
      service: {
        Lazy: typeof help.service.HaTSLazy;
      };
    };
  }
}

const isValidCfg = (cfg: HaTSConfig | undefined) => {
  return (
    cfg !== undefined &&
    cfg.productId !== 0 &&
    cfg.apiKey !== '' &&
    cfg.triggerId !== ''
  );
};

export const requestSurvey = (cfg: HaTSConfig, auth: AuthState) => {
  if (auth.identity === '' || !window.help?.service?.Lazy || !isValidCfg(cfg)) {
    return;
  }

  // Create the HaTS instance using the config's product ID and API key.
  const hats = window.help.service.Lazy.create(cfg.productId, {
    apiKey: cfg.apiKey,
    locale: 'en',
  });

  // Define callback invoked with survey data.
  // The survey isn't shown when the provided data is null.
  const callback = (
    requestSurveyCallbackParam: help.service.RequestSurveyCallbackParam,
  ) => {
    if (!requestSurveyCallbackParam.surveyData) {
      return;
    }
    hats.presentSurvey({
      surveyData: requestSurveyCallbackParam.surveyData,
      colorScheme: 1, // 1 for light, 2 for dark
      authuser: Number(auth.identity),
      customZIndex: 10000, // Set a high z-index
    });
  };

  // Request the survey.
  hats.requestSurvey({
    triggerId: cfg.triggerId,
    callback: callback,
    authuser: Number(auth.identity),
  });
};
