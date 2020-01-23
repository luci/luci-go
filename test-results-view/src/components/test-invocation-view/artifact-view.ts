import { customElement, html, css } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { Artifact } from "../../models/build";
import '@material/mwc-icon'
import { observable } from "mobx";

@customElement('milo-artifact-view')
export class ArtifactView extends MobxLitElement {
    @observable.ref
    public artifact?: Artifact;

    protected render() {
        return html`
        <a href=${this.artifact!.viewUrl}>${this.artifact!.name}</a>
        ${this.artifact!.fetchUrl && html`<a href=${this.artifact!.fetchUrl}><mwc-icon class="inline-icon">save_alt</mwc-icon></a>`}
        ${(() => {
            if (this.artifact!.contentType === 'plain/text') {
                return this.artifact!.contents;
            } else if (this.artifact!.contentType?.startsWith('image/')) {
                return this.artifact!.contents;
            } else {
                return;
            }
        })()}
        `;
    }

    static styles = css`
        .inline-icon {
            --mdc-icon-size: 1em;
            top: .125em;
            position: relative;
            color: black;
        }
    `;
}
