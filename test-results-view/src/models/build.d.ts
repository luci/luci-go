export interface Build {
    readonly builder: string;
    readonly buildId: string;
    readonly startCommit: string;
    readonly endCommit: string;
    readonly status: TestStatus;
    readonly testResults: {[key: string]: TestResult};
}

export type TestStatus =
    "STATUS_UNSPECIFIED" |
    "PASS" |
    "FAIL" |
    "CRASH" |
    "ABORT" |
    "SKIP";

export interface Invocation {
    readonly name: string;
    readonly state: string;
    readonly createTime: string;
    readonly finalizeTime: string;
    readonly deadline: string;
    readonly includedInvocations: string[];
    readonly tags: {key: string, value: string}[];
}

export interface TestResult {
    readonly name: string;
    readonly testId: string;
    readonly resultId: string;
    readonly variant?: Variant;
    readonly expected?: boolean;
    readonly status: TestStatus;
    readonly summaryHtml: string;
    readonly startTime: string;
    readonly duration: string;
    readonly tags: Tag[];
    readonly inputArtifacts?: Artifact[];
    readonly outputArtifacts?: Artifact[];
}

export interface TestExoneration {
    readonly name: string;
    readonly testId: string;
    readonly variant: Variant;
    readonly exonerationId: string;
    readonly explanationHTML?: string;
}

export interface Artifact {
    readonly name: string;
    readonly fetchUrl?: string;
    readonly viewUrl?: string;
    readonly contentType: string;
    readonly size: number;
    readonly contents: any;
}

export interface Variant {
    readonly def: {[key: string]: string};
}

export interface Tag {
    readonly key: string;
    readonly value: string;
}