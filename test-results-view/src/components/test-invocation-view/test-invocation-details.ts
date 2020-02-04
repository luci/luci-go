import { customElement, css } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { html } from "lit-html";
import moment from 'moment';
import { Invocation, InvocationState } from "../../models/resultdb";
import { classMap } from "lit-html/directives/class-map";
import { observable } from "mobx";

const INVOCATION_STATE_DISPLAY_MAP = {
    [InvocationState.Unspecified]: 'unspecified',
    [InvocationState.Active]: 'active',
    [InvocationState.Finalizing]: 'finalizing',
    [InvocationState.Finalized]: 'finalized',
};

const INVOCATION_STATE_CLASS_MAP = {
    [InvocationState.Unspecified]: 'unspecified',
    [InvocationState.Active]: 'active',
    [InvocationState.Finalizing]: 'finalizing',
    [InvocationState.Finalized]: 'finalized',
};

@customElement('tr-test-invocation-details')
export class TestInvocationDetails extends MobxLitElement {
    @observable.ref
    public invocation?: Invocation;

    @observable.ref
    public expanded: boolean = false;

    protected render() {
        return html`
        <div
            class="expandable-header"
            @click=${() => this.expanded = !this.expanded}
        >
            <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
            <span class="one-line-content">Details</span>
        </div>
        <div id="body">
            <div id="content" class=${classMap({'hidden': !this.expanded})}>
                <div>Create Time: ${moment(this.invocation!.createTime).format('YYYY-MM-DD HH:mm:ss')}</div>
                <div>Finalize Time: ${moment(this.invocation!.finalizeTime).format('YYYY-MM-DD HH:mm:ss')}</div>
                <div
                    id="included-invocations"
                    class=${classMap({hidden: this.invocation!.includedInvocations.length === 0})}
                >Included Invocations:
                    <ul>
                        ${this.invocation!.includedInvocations.map(invocationName => html`
                        <li><a href=${`/invocation/${encodeURIComponent(invocationName)}`}>${invocationName.slice('invocations/'.length)}</a></li>
                        `)}
                    </ul>
                </div>
                <div
                    id="tags"
                    class=${classMap({hidden: this.invocation!.tags.length === 0})}
                >Tags:
                    <ul>
                        ${this.invocation!.tags.map(({key: k, value: v}) => html`<li>${k}: ${v}</li>`)}
                    </ul>
                </div>
            </div>
        </div>
        `;
    }

    static styles = css`
        .expandable-header {
            display: grid;
            grid-template-columns: 24px 1fr;
            grid-template-rows: 24px;
            grid-gap: 5px;
            cursor: pointer;
        }
        .expandable-header .expand-toggle {
            grid-row: 1;
            grid-column: 1;
        }
        .expandable-header .one-line-content {
            grid-row: 1;
            grid-column: 2;
            font-size: 16px;
            line-height: 24px;
            overflow: hidden;
            white-space: nowrap;
            text-overflow: ellipsis;
        }
        #body {
            display: grid;
            grid-template-columns: 24px 1fr;
            grid-gap: 5px;
        }
        #content {
            padding: 5px 0 5px 0;
            grid-column: 2;
        }
        #content-ruler {
            border-left: 1px solid grey;
            width: 0px;
            margin-left: 11.5px;
        }

        .badge {
            border: 1px solid;
            border-radius: 5px;
            padding: 2px;
        }
        #interrupted {
            color: #ffc107;
        }

        .hidden {
            display: none;
        }

        #invocation-card > * {
            margin-top: 5px;
        }
        #invocation-card > *:first-child {
            margin-top: 0px;
        }
        `;
}
