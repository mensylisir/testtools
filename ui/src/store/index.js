import { createStore } from 'vuex'
import createPersistedState from 'vuex-persistedstate'

export default createStore({
    state: {
        lodding: false,
        clusterCredentials: null,
        currentNamespace: 'default'
    },
    plugins: [createPersistedState({
        paths: ['currentNamespace']
    })],
    getters: {
        getClusterCredentials: state => state.clusterCredentials,
        getCurrentNamespace: (state) => state.currentNamespace
    },
    mutations: {
        SET_LOADING(state, loading) {
            state.loading = loading
        },
        SET_ERROR(state, error) {
            state.error = error
        },
        CLUSTER_CREDENTIALS(state, credentials) {
            state.clusterCredentials = credentials
        },
        SET_CURRENT_NAMESPACE(state, namespace) {
            state.currentNamespace = namespace
        }
    },
    actions: {
        saveClusterCredentials({ commit, dispatch }, credentials) {
            commit('CLUSTER_CREDENTIALS', credentials)
            return credentials
        },
        setCurrentNamespace({ commit, dispatch }, namespace) {
            commit('SET_CURRENT_NAMESPACE', namespace)
            return namespace
        }
    }
})