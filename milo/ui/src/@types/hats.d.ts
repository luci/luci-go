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

// Declare HaTS namespace with relevant type signatures pulled from
// https://source.corp.google.com/piper///depot/google3/science_search/html/hats_externs.js.
// This assumes https://www.gstatic.com/feedback/js/help/prod/service/lazy.min.js is loaded.
declare namespace help {
  namespace service {
    export interface Config {
      apiKey?: string;
      locale?: string;
      productData?: { [key: string]: string } | null;
    }

    export interface SurveyMetadata {
      triggerId?: string;
      surveyId?: string;
      sessionId?: string;
    }

    export interface SurveyData {
      surveyData: string;
      triggerRequestTime: number;
      apiKey: string;
      nonProd?: boolean;
      language?: string;
      libraryVersion: number;
      surveyMetadata: SurveyMetadata;
    }

    export interface SurveyError {
      reason: string;
    }

    export interface RequestSurveyCallbackParam {
      surveyData: SurveyData | null;
      triggerId: string;
      surveyError: SurveyError | null;
    }

    export interface RequestSurveyParam {
      triggerId: string;
      callback: (param: RequestSurveyCallbackParam) => void;
      authuser?: number;
      enableTestingMode?: boolean;
      nonProd?: boolean;
      preferredSurveyLanguageList?: string[];
    }

    export interface ProductSurveyData {
      productVersion?: string;
      experimentIds?: string[];
      customData?: { [key: string]: string };
    }

    export interface PresentSurveyParam {
      surveyData: SurveyData;
      colorScheme: number;
      seamlessMode?: boolean;
      authuser?: number;
      customZIndex?: number;
      customLogoUrl?: string;
      productData?: ProductSurveyData;
      promptStyle?: number;
      completionStyle?: number;
      defaultStyle?: number;
      enableReloadScriptWhenLanguageChanges?: boolean;
    }

    export class HaTSLazy {
      static create(productId: number, config: Config): HaTSLazy;
      requestSurvey(param: RequestSurveyParam): void;
      presentSurvey(param: PresentSurveyParam): void;
    }
  }
}

// Per product config
interface HaTSConfig {
  readonly apiKey: string;
  readonly triggerId: string;
  readonly productId: number;
}
