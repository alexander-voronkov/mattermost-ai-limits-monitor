import React from 'react';
import RHSPanel from './components/rhs_panel';

const PLUGIN_ID = 'com.fambear.ai-limits-monitor';

class PluginClass {
    initialize(registry: any, store: any) {
        // Register the RHS panel component
        const {toggleRHSPlugin} = registry.registerRightHandSidebarComponent(
            RHSPanel,
            'AI Limits Monitor',
        );

        // Register AppBar icon (Mattermost 10+)
        if (registry.registerAppBarComponent) {
            const iconURL = `/plugins/${PLUGIN_ID}/static/app-bar-icon.png`;
            registry.registerAppBarComponent(
                iconURL,
                () => store.dispatch(toggleRHSPlugin),
                'AI Limits Monitor',
            );
        }

        // Fallback: register channel header button for older versions
        registry.registerChannelHeaderButtonAction(
            () => React.createElement('span', {style: {fontSize: '16px'}}, '📊'),
            () => store.dispatch(toggleRHSPlugin),
            null,
            'AI Limits Monitor',
        );
    }

    uninitialize() {
        // cleanup if needed
    }
}

// @ts-ignore
global.window.registerPlugin(PLUGIN_ID, new PluginClass());
