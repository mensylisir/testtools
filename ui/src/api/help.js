import store from "@/store";

export function getHeaders() {
    const credentials = store.getters['getClusterCredentials'] || store.state.clusterCredentials;
    const headers = {};

    if (credentials && credentials.token) {
        headers['x-k8s-token'] = `${credentials.token}`;
    }
    if (credentials && credentials.token) {
        headers['x-k8s-endpoints'] = `${credentials.endpoints}`;
    }
    headers['Content-Type'] = 'application/json'
    return headers;
}

export function getEndpoints() {
    const credentials = store.getters['getClusterCredentials'] || store.state.clusterCredentials;
    let endpoints = ""
    if (credentials && credentials.endpoints) {
        endpoints = credentials.endpoints;
     }

    return endpoints;
}

export function getUser() {
    const credentials = store.getters['getClusterCredentials'] || store.state.clusterCredentials;
    let user = ""
    if (credentials && credentials.name) {
        user = credentials.name;
    }

    return user;
}