import React, {useEffect, useState, useCallback} from 'react';

const PLUGIN_ID = 'com.fambear.ai-limits-monitor';

interface ServiceData {
    id: string;
    name: string;
    enabled: boolean;
    status: string;
    data?: any;
    error?: string;
    cachedAt?: number;
}

interface StatusResponse {
    services: ServiceData[];
}

const fetchStatus = async (): Promise<StatusResponse> => {
    const resp = await fetch(`/plugins/${PLUGIN_ID}/api/v1/status`, {
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    });
    if (!resp.ok) {
        throw new Error(`HTTP ${resp.status}`);
    }
    return resp.json();
};

const refreshAll = async (): Promise<StatusResponse> => {
    const resp = await fetch(`/plugins/${PLUGIN_ID}/api/v1/refresh`, {
        method: 'POST',
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    });
    if (!resp.ok) {
        throw new Error(`HTTP ${resp.status}`);
    }
    return resp.json();
};

const formatNumber = (n: number): string => {
    if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1) + 'B';
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
    return n.toFixed(0);
};

const formatTimeUntil = (timestampMs: number): string => {
    const diff = timestampMs - Date.now();
    if (diff <= 0) return 'now';
    const hours = Math.floor(diff / 3600000);
    const mins = Math.floor((diff % 3600000) / 60000);
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
};

const getStatusColor = (status: string): string => {
    switch (status) {
        case 'ok': return '#3db887';
        case 'warning': return '#f5a623';
        case 'error': return '#d24b4e';
        default: return '#8b8fa7';
    }
};

const UsageBar: React.FC<{used: number; total: number; label?: string}> = ({used, total, label}) => {
    const percent = total > 0 ? Math.min((used / total) * 100, 100) : 0;
    const color = percent > 90 ? '#d24b4e' : percent > 70 ? '#f5a623' : '#3db887';

    return (
        <div style={{marginBottom: '8px'}}>
            {label && <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '2px'}}>{label}</div>}
            <div style={{display: 'flex', alignItems: 'center', gap: '8px'}}>
                <div style={{
                    flex: 1,
                    height: '8px',
                    backgroundColor: '#e0e0e0',
                    borderRadius: '4px',
                    overflow: 'hidden',
                }}>
                    <div style={{
                        width: `${percent}%`,
                        height: '100%',
                        backgroundColor: color,
                        borderRadius: '4px',
                        transition: 'width 0.3s ease',
                    }}/>
                </div>
                <span style={{fontSize: '12px', color: '#555', minWidth: '60px', textAlign: 'right'}}>
                    {formatNumber(used)} / {formatNumber(total)}
                </span>
            </div>
        </div>
    );
};

const AugmentCard: React.FC<{data: any}> = ({data}) => {
    if (!data) return null;
    return (
        <div>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '4px'}}>{data.planName || 'Augment Code'}</div>
            <UsageBar used={data.usageUsed || 0} total={data.usageTotal || 0} label="Credits" />
            {data.cycleEnd && (
                <div style={{fontSize: '11px', color: '#8b8fa7'}}>
                    Cycle ends: {new Date(data.cycleEnd).toLocaleDateString()}
                </div>
            )}
        </div>
    );
};

const ZaiCard: React.FC<{data: any}> = ({data}) => {
    if (!data) return null;
    return (
        <div>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '4px'}}>
                {data.planName || 'Z.AI'} {data.planStatus ? `(${data.planStatus})` : ''}
            </div>
            <UsageBar used={data.tokensUsed || 0} total={data.tokensTotal || 0} label="Tokens (5h window)" />
            <UsageBar used={data.mcpUsed || 0} total={data.mcpTotal || 0} label="MCP Tools" />
            {data.nextReset > 0 && (
                <div style={{fontSize: '11px', color: '#8b8fa7'}}>
                    Resets in: {formatTimeUntil(data.nextReset)}
                </div>
            )}
        </div>
    );
};

const OpenAICard: React.FC<{data: any}> = ({data}) => {
    if (!data) return null;
    return (
        <div>
            <div style={{fontSize: '14px', fontWeight: 600}}>
                ${(data.totalCost || 0).toFixed(2)}
            </div>
            <div style={{fontSize: '11px', color: '#8b8fa7'}}>{data.period || 'This period'}</div>
        </div>
    );
};

const UtilizationBar: React.FC<{utilization: number; label: string; resetEpoch?: string; status?: string}> = ({utilization, label, resetEpoch, status}) => {
    const percent = Math.min(utilization * 100, 100);
    const color = status === 'rejected' ? '#d24b4e' : percent > 80 ? '#d24b4e' : percent > 60 ? '#f5a623' : '#3db887';

    let resetStr = '';
    if (resetEpoch) {
        const resetMs = parseInt(resetEpoch, 10) * 1000;
        if (resetMs > Date.now()) {
            resetStr = ` · resets in ${formatTimeUntil(resetMs)}`;
        }
    }

    return (
        <div style={{marginBottom: '8px'}}>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '2px'}}>
                {label}{resetStr}
            </div>
            <div style={{display: 'flex', alignItems: 'center', gap: '8px'}}>
                <div style={{
                    flex: 1,
                    height: '8px',
                    backgroundColor: '#e0e0e0',
                    borderRadius: '4px',
                    overflow: 'hidden',
                }}>
                    <div style={{
                        width: `${percent}%`,
                        height: '100%',
                        backgroundColor: color,
                        borderRadius: '4px',
                        transition: 'width 0.3s ease',
                    }}/>
                </div>
                <span style={{fontSize: '12px', color: '#555', minWidth: '50px', textAlign: 'right'}}>
                    {percent.toFixed(0)}%
                </span>
            </div>
        </div>
    );
};

const ClaudeCard: React.FC<{data: any}> = ({data}) => {
    if (!data) return null;

    const u5h = parseFloat(data.utilization5h) || 0;
    const u7d = parseFloat(data.utilization7d) || 0;
    const hasData = data.utilization5h || data.utilization7d;

    if (!hasData) {
        return (
            <div>
                <div style={{fontSize: '12px', color: '#8b8fa7'}}>
                    {data.authMethod || 'OAuth'} · No rate limit data
                </div>
            </div>
        );
    }

    return (
        <div>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '4px'}}>
                {data.authMethod || 'claude.ai'} 
                {data.overageStatus === 'rejected' && 
                    <span style={{color: '#d24b4e', marginLeft: '4px'}}>· overage disabled</span>
                }
            </div>
            <UtilizationBar 
                utilization={u5h} 
                label="5-hour window" 
                resetEpoch={data.reset5h}
                status={data.status5h}
            />
            <UtilizationBar 
                utilization={u7d} 
                label="7-day window" 
                resetEpoch={data.reset7d}
                status={data.status7d}
            />
            {data.checkTimestamp > 0 && (
                <div style={{fontSize: '10px', color: '#b0b0b0'}}>
                    Checked: {new Date(data.checkTimestamp * 1000).toLocaleTimeString()}
                </div>
            )}
        </div>
    );
};

const ServiceCard: React.FC<{service: ServiceData}> = ({service}) => {
    const statusColor = getStatusColor(service.status);

    const renderData = () => {
        if (service.error) {
            return <div style={{fontSize: '12px', color: '#d24b4e'}}>{service.error}</div>;
        }
        switch (service.id) {
            case 'augment': return <AugmentCard data={service.data} />;
            case 'zai': return <ZaiCard data={service.data} />;
            case 'openai': return <OpenAICard data={service.data} />;
            case 'claude': return <ClaudeCard data={service.data} />;
            default: return null;
        }
    };

    return (
        <div style={{
            padding: '12px',
            marginBottom: '8px',
            borderRadius: '8px',
            backgroundColor: 'var(--center-channel-bg, #fff)',
            border: '1px solid var(--center-channel-color-08, #e0e0e0)',
        }}>
            <div style={{display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px'}}>
                <div style={{
                    width: '8px',
                    height: '8px',
                    borderRadius: '50%',
                    backgroundColor: statusColor,
                }}/>
                <span style={{fontWeight: 600, fontSize: '14px'}}>{service.name}</span>
            </div>
            {renderData()}
            {service.cachedAt && (
                <div style={{fontSize: '10px', color: '#b0b0b0', marginTop: '4px'}}>
                    Updated: {new Date(service.cachedAt * 1000).toLocaleTimeString()}
                </div>
            )}
        </div>
    );
};

const RHSPanel: React.FC = () => {
    const [services, setServices] = useState<ServiceData[]>([]);
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const loadData = useCallback(async () => {
        try {
            const data = await fetchStatus();
            setServices(data.services);
            setError(null);
        } catch (e: any) {
            setError(e.message);
        } finally {
            setLoading(false);
        }
    }, []);

    const handleRefresh = useCallback(async () => {
        setRefreshing(true);
        try {
            const data = await refreshAll();
            setServices(data.services);
            setError(null);
        } catch (e: any) {
            setError(e.message);
        } finally {
            setRefreshing(false);
        }
    }, []);

    useEffect(() => {
        loadData();
        // Auto-refresh every 5 minutes
        const interval = setInterval(loadData, 5 * 60 * 1000);
        return () => clearInterval(interval);
    }, [loadData]);

    return (
        <div style={{padding: '16px'}}>
            <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px'}}>
                <h3 style={{margin: 0, fontSize: '16px', fontWeight: 600}}>AI Service Limits</h3>
                <button
                    onClick={handleRefresh}
                    disabled={refreshing}
                    style={{
                        padding: '4px 12px',
                        border: '1px solid var(--center-channel-color-16, #ccc)',
                        borderRadius: '4px',
                        backgroundColor: 'transparent',
                        cursor: refreshing ? 'wait' : 'pointer',
                        fontSize: '13px',
                    }}
                >
                    {refreshing ? '⏳' : '🔄'} Refresh
                </button>
            </div>

            {loading && (
                <div style={{textAlign: 'center', padding: '24px', color: '#8b8fa7'}}>Loading...</div>
            )}

            {error && (
                <div style={{
                    padding: '12px',
                    backgroundColor: '#fef0f0',
                    borderRadius: '8px',
                    color: '#d24b4e',
                    fontSize: '13px',
                    marginBottom: '8px',
                }}>
                    Error: {error}
                </div>
            )}

            {!loading && services.map((service) => (
                <ServiceCard key={service.id} service={service} />
            ))}
        </div>
    );
};

export default RHSPanel;
