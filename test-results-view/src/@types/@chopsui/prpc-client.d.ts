declare module '@chopsui/prpc-client' {
    export interface PrpcClientOpts {
        host?: string;
        accessToken?: string;
        insecure?: boolean;
        fetchImpl?: typeof fetch;
    }

    export class PrpcClient {
        constructor(options?: PrpcClientOpts);
        call(service: string, method: string, message: Object, additionalHeaders?: {[key: string]: string}): Promise<Object>;
    }
}
