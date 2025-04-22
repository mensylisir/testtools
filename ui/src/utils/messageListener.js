import { ref } from 'vue'
import store from '@/store'
import clusterConfig  from "@/config/clusterConfig";


export const clusterInfo = ref(null);

export function initMessageListener() {
    window.addEventListener('message', handlePostMessage, false)

    if (!clusterInfo.value) {
        if (clusterConfig.endpoints && clusterConfig.token) {
            console.log('Received cluster credential from config file:', {
                endpoints: clusterConfig.endpoints,
                token: clusterConfig.token.slice(0, 10) + '...',
                name: clusterConfig.name,
            });
            clusterInfo.value = {
                endpoints: clusterConfig.endpoints,
                token: clusterConfig.token,
                name: clusterConfig.name,
            };
            store.dispatch('saveClusterCredentials', clusterInfo.value);
        } else {
            console.error('Invalid cluster configuration in config file');
        }
    }

    console.log('Message listener initialized')
}


async function handlePostMessage(event) {
    console.log('Received message:', event)
    try {
        const data = event.data;
        
        if (isValidMessage(data)) {
            console.log('Received valid cluster credential:', {
                endpoints: data.endpoints,
                token: data.token.slice(0, 10) + '...',
                name: data.name,
            })

            clusterInfo.value = {
                endpoints: data.endpoints,
                token: data.token,
                name: data.name,
            }
            
            await store.dispatch('saveClusterCredentials', clusterInfo.value)
        } else {
            console.error('invalid postMessage')
        }
    } catch (error) {
        console.error('Error processing postMessage:', error)
    }
}

function isValidMessage(data) {
    return (
        typeof data === 'object' &&
        data !== null &&
        'endpoints' in data &&
        'token' in data &&
        'name' in data &&
        typeof data.endpoints === 'string' &&
        typeof data.token === 'string' &&
        typeof data.name ==='string'
    );
}