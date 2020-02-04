export enum TestStatus {
    Unspecified = "STATUS_UNSPECIFIED",
    Pass = "PASS",
    Fail = "FAIL",
    Crash = "CRASH",
    Abort = "ABORT",
    Skip = "SKIP",
}

export enum InvocationState {
    Unspecified = 'STATE_UNSPECIFIED',
    Active = 'ACTIVE',
    Finalizing = 'FINALIZING',
    Finalized = 'FINALIZED',
}

export interface Invocation {
    readonly interrupted: boolean;
    readonly name: string;
    readonly state: InvocationState;
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