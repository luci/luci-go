import { html, customElement, css, property, LitElement } from 'lit-element';
import { MobxLitElement } from '@adobe/lit-mobx';
import { Router } from '@vaadin/router';

import './components/test-invocation-view';
import './test-utils';
import { store } from './store';

@customElement('app-milo')
export class Milo extends MobxLitElement {
    private get invocationName() {
        return this.location!.params['invocation_name'];
    }

    protected render() {
        if (!store.googleAuth) {
            return html``;
        }

        return html`
        <div>
        ${store.isSignedIn ?
            html`<a @click=${() => store.googleAuth!.signOut()}>Sign Out</a>` :
            html`<a @click=${() => store.googleAuth!.signIn({prompt: 'consent'})}>Sign In</a>`}
        </div>
        <milo-test-invocation-view .invocationName=${this.invocationName}></milo-test-invocation-view>
        `;
    }

    static style = css``;
}

const router = new Router(document.getElementById('app-root'));
router.setRoutes([
    {path: '/invocation/:invocation_name', component: 'app-milo'}
]);
