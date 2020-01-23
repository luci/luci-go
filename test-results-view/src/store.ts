import { observable, action } from "mobx";
const config = require("../config.json");

export class Store {
    @observable.ref
    googleAuth?: gapi.auth2.GoogleAuth;

    // better than listening to the user object, because googleAuth mutates the user object instead of creating a new one
    @observable
    isSignedIn: boolean = false;
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
