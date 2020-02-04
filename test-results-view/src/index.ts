import { html, customElement, css } from 'lit-element';
import { MobxLitElement } from '@adobe/lit-mobx';
import { Router } from '@vaadin/router';

import './components/test-invocation-view';
import './test-utils';
import { store } from './store';

@customElement('app-test-results-view')
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
        <tr-test-invocation-view .invocationName=${this.invocationName}></tr-test-invocation-view>
        `;
    }

    static styles = css`
        :host {
            display: grid;
            grid-template-rows: auto 1fr;
            color: #212121;
            font-family: "Roboto";
            font-size: 16px;
            letter-spacing: 0.5px;
            height: 100%;
        }
        `;
}

const router = new Router(document.getElementById('app-root'));
router.setRoutes([
    {path: '/invocation/:invocation_name', component: 'app-test-results-view'}
]);
