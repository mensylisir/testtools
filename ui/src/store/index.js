import { createStore } from 'vuex'
import digApi from '@/api/dig'

export default createStore({
    state: {
        lodding: false,
        clusterCredentials: null,
        digs: []
    },
    getters: {
        getClusterCredentials: state => state.clusterCredentials,
        getDigs: state => state.digs,
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
        }
    },
    actions: {
        saveClusterCredentials({ commit, dispatch }, credentials) {
            commit('CLUSTER_CREDENTIALS', credentials)
            return credentials
        },
        async getDigs({ commit }) {
            try {
                const response = await digApi.getDigList()
            } catch (error) {
                commit('SET_ERROR', error.message)
            } finally {
                commit('SET_LOADING', false)
            }
        }
    }
})