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
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    return resp.json();
};

const refreshAll = async (): Promise<StatusResponse> => {
    const resp = await fetch(`/plugins/${PLUGIN_ID}/api/v1/refresh`, {
        method: 'POST',
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    return resp.json();
};

const formatNumber = (n: number): string => {
    if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1) + 'B';
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
    return n.toFixed(0);
};

const formatTimeUntil = (input: number | string): string => {
    let ts: number;
    if (typeof input === 'string') {
        ts = new Date(input).getTime();
    } else {
        ts = input;
    }
    const diff = ts - Date.now();
    if (diff <= 0) return 'now';
    const hours = Math.floor(diff / 3600000);
    const mins = Math.floor((diff % 3600000) / 60000);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
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
                <div style={{flex: 1, height: '8px', backgroundColor: '#e0e0e0', borderRadius: '4px', overflow: 'hidden'}}>
                    <div style={{width: `${percent}%`, height: '100%', backgroundColor: color, borderRadius: '4px', transition: 'width 0.3s ease'}}/>
                </div>
                <span style={{fontSize: '12px', color: '#555', minWidth: '60px', textAlign: 'right'}}>
                    {formatNumber(used)} / {formatNumber(total)}
                </span>
            </div>
        </div>
    );
};

const UtilizationBar: React.FC<{utilization: number; label: string; resetAt?: string}> = ({utilization, label, resetAt}) => {
    // utilization comes as percentage (e.g. 48.0 = 48%)
    const percent = Math.min(utilization, 100);
    const color = percent >= 100 ? '#d24b4e' : percent > 80 ? '#f5a623' : '#3db887';

    let resetStr = '';
    if (resetAt) {
        resetStr = ` · resets in ${formatTimeUntil(resetAt)}`;
    }

    return (
        <div style={{marginBottom: '8px'}}>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '2px'}}>{label}{resetStr}</div>
            <div style={{display: 'flex', alignItems: 'center', gap: '8px'}}>
                <div style={{flex: 1, height: '8px', backgroundColor: '#e0e0e0', borderRadius: '4px', overflow: 'hidden'}}>
                    <div style={{width: `${percent}%`, height: '100%', backgroundColor: color, borderRadius: '4px', transition: 'width 0.3s ease'}}/>
                </div>
                <span style={{fontSize: '12px', color: '#555', minWidth: '50px', textAlign: 'right'}}>
                    {percent.toFixed(0)}%
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
            <UsageBar used={data.usageUsed || 0} total={data.usageTotal || 0} label={`Credits: ${formatNumber(data.usageRemaining || 0)} remaining`} />
            {data.cycleEnd && <div style={{fontSize: '11px', color: '#8b8fa7'}}>Cycle ends: {new Date(data.cycleEnd).toLocaleDateString()}</div>}
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
            {data.nextReset > 0 && <div style={{fontSize: '11px', color: '#8b8fa7'}}>Resets in: {formatTimeUntil(data.nextReset)}</div>}
        </div>
    );
};

const OpenAICard: React.FC<{data: any}> = ({data}) => {
    if (!data) return null;
    const cost = data.totalCost || 0;
    const budget = data.budget || 0;
    const credit = data.creditBalance;
    const days = data.daysUntilReset || 0;
    return (
        <div>
            {credit > 0 && (
                <div style={{fontSize: '12px', marginBottom: '6px'}}>
                    <span style={{color: '#8b8fa7'}}>Credit balance: </span>
                    <span style={{fontWeight: 600}}>${credit.toFixed(2)}</span>
                </div>
            )}
            {budget > 0 ? (
                <div>
                    <div style={{fontSize: '12px', marginBottom: '4px'}}>
                        <span style={{color: '#8b8fa7'}}>{data.period || 'Monthly'} budget: </span>
                        <span style={{fontWeight: 600}}>${cost.toFixed(2)} / ${budget.toFixed(0)}</span>
                    </div>
                    <div style={{height: '6px', borderRadius: '3px', backgroundColor: '#e8e8e8', overflow: 'hidden'}}>
                        <div style={{
                            height: '100%', borderRadius: '3px',
                            width: `${Math.min(cost / budget * 100, 100)}%`,
                            backgroundColor: cost/budget >= 1 ? '#d24b4e' : cost/budget > 0.8 ? '#f5a623' : '#3db887',
                        }}/>
                    </div>
                </div>
            ) : (
                <div style={{fontSize: '14px', fontWeight: 600}}>${cost.toFixed(2)}</div>
            )}
            <div style={{fontSize: '11px', color: '#8b8fa7', marginTop: '4px'}}>
                Resets in {days} day{days !== 1 ? 's' : ''}
            </div>
        </div>
    );
};

const ClaudeCard: React.FC<{data: any}> = ({data}) => {
    if (!data || !data.hasData) {
        return <div style={{fontSize: '12px', color: '#8b8fa7'}}>Connected · No usage data yet</div>;
    }
    return (
        <div>
            <div style={{fontSize: '12px', color: '#8b8fa7', marginBottom: '4px'}}>claude.ai</div>
            {data.utilization5h !== undefined && (
                <UtilizationBar utilization={data.utilization5h} label="5-hour window" resetAt={data.reset5h} />
            )}
            {data.utilization7d !== undefined && (
                <UtilizationBar utilization={data.utilization7d} label="7-day window" resetAt={data.reset7d} />
            )}
            {data.sonnetUtil > 0 && (
                <UtilizationBar utilization={data.sonnetUtil} label="Sonnet (weekly)" />
            )}
            {data.opusUtil > 0 && (
                <UtilizationBar utilization={data.opusUtil} label="Opus (weekly)" />
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
            padding: '12px', marginBottom: '8px', borderRadius: '8px',
            backgroundColor: 'var(--center-channel-bg, #fff)',
            border: '1px solid var(--center-channel-color-08, #e0e0e0)',
        }}>
            <div style={{display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px'}}>
                <div style={{width: '8px', height: '8px', borderRadius: '50%', backgroundColor: statusColor, flexShrink: 0}}/>
                <span style={{fontWeight: 600, fontSize: '14px', flex: 1}}>{service.name}</span>
                {service.cachedAt && service.cachedAt > 0 && (
                    <span style={{fontSize: '10px', color: '#b0b0b0', flexShrink: 0}}>
                        {new Date(service.cachedAt * 1000).toLocaleTimeString()}
                    </span>
                )}
            </div>
            {renderData()}
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
        const interval = setInterval(loadData, 5 * 60 * 1000);
        return () => clearInterval(interval);
    }, [loadData]);

    return (
        <div style={{
            display: 'flex',
            flexDirection: 'column',
            height: '100%',
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            overflow: 'hidden',
        }}>
            <div style={{
                padding: '16px',
                flex: 1,
                overflowY: 'auto',
                overflowX: 'hidden',
            }}>
            <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px'}}>
                <h3 style={{margin: 0, fontSize: '16px', fontWeight: 600}}>AI Service Limits</h3>
                <button onClick={handleRefresh} disabled={refreshing} style={{
                    padding: '4px 12px', border: '1px solid var(--center-channel-color-16, #ccc)',
                    borderRadius: '4px', backgroundColor: 'transparent',
                    cursor: refreshing ? 'wait' : 'pointer', fontSize: '13px',
                }}>
                    {refreshing ? '⏳' : '🔄'} Refresh
                </button>
            </div>
            {loading && <div style={{textAlign: 'center', padding: '24px', color: '#8b8fa7'}}>Loading...</div>}
            {error && (
                <div style={{padding: '12px', backgroundColor: '#fef0f0', borderRadius: '8px', color: '#d24b4e', fontSize: '13px', marginBottom: '8px'}}>
                    Error: {error}
                </div>
            )}
            {!loading && services.map((service) => (
                <ServiceCard key={service.id} service={service} />
            ))}
            </div>
        </div>
    );
};

export default RHSPanel;
