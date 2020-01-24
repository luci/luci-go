import { customElement, css } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { html } from "lit-html";
import * as mdcCardStyle from '@material/card/dist/mdc.card.css';
import moment from 'moment';
import { Invocation } from "../../models/build";
import { classMap } from "lit-html/directives/class-map";
import { observable } from "mobx";

@customElement('milo-test-invocation-additional-info')
export class TestInvocationAdditionalInfo extends MobxLitElement {
    @observable.ref
    public invocation?: Invocation;

    protected render() {
        return html`
        <div id="invocation-card" class="mdc-card mdc-card--outlined">
            <div id="time">
                <div>Create Time: ${moment(this.invocation!.createTime).calendar()}</div>
                <div>Finalize Time: ${moment(this.invocation!.finalizeTime).calendar()}</div>
                <div>Deadline: ${moment(this.invocation!.deadline).calendar()}</div>
            </div>
            <div
                id="included-invocations"
                class=${classMap({hidden: this.invocation!.includedInvocations.length === 0})}
            >Included Invocations:
                <ul>
                    ${this.invocation!.includedInvocations.map(invocationName => html`
                    <li><a href=${`/invocation/${encodeURIComponent(invocationName)}`}>${invocationName}</a></li>
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
        `;
    }

    static styles = css`
        .hidden {
            display: none;
        }

        #invocation-card > * {
            margin-top: 5px;
        }
        #invocation-card > *:first-child {
            margin-top: 0px;
        }

        #time {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            grid-gap: 5px;
        }

        .mdc-card {
            padding: 5px;
        }
        `;

    constructor() {
        super();
        (this.shadowRoot as any).adoptedStyleSheets = [
            mdcCardStyle,
            ...(this.shadowRoot as any).adoptedStyleSheets,
        ];
    }
}
