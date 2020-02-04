import { observable, action, computed } from "mobx";
import { PrpcClient } from '@chopsui/prpc-client';
const config = require("../config.json");

export class Store {
    @observable.ref
    googleAuth?: gapi.auth2.GoogleAuth;

    // better than listening to the user object, because googleAuth mutates the user object instead of creating a new one
    @observable
    isSignedIn: boolean = false;

    @computed
    get resultDbPrpcClient() {
        if (!store.isSignedIn) {
            return null;
        }
        const authRes = store.googleAuth!.currentUser.get().getAuthResponse();
        return new PrpcClient({
            host: 'staging.results.api.cr.dev',
            accessToken: authRes.access_token,
        });
    }
}

export const store = new Store();
gapi.load('auth2', () => {
    const googleAuth = gapi.auth2.init({
        client_id: config['client-id'],
        scope: 'openid',
    });
    googleAuth.then((googleAuth) => store.googleAuth = googleAuth);
    googleAuth.isSignedIn.listen((isSignedIn) => store.isSignedIn = isSignedIn);
});
