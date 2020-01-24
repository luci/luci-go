import { MobxLitElement } from '@adobe/lit-mobx';

declare module '@adobe/lit-mobx' {
    interface MobxLitElement {
        location?: {
            params: {[key: string]: string},
        }
    }
}
