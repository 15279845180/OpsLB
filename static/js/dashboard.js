// 仪表盘页面逻辑
let ws = null;
let wsReconnectTimer = null;

window.addEventListener('pageLoaded', async () => {
    // 初始加载一次(显示loading)
    await loadDashboardData();
    
    // 连接WebSocket实时推送
    connectWebSocket();
});

// 连接WebSocket
function connectWebSocket() {
    // 关闭旧连接（页面切换后重新加载JS时，旧ws已不可达，需通过window引用关闭）
    if (window.__dashWs && window.__dashWs.readyState === WebSocket.OPEN) {
        window.__dashWs.close();
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const token = localStorage.getItem('token');

    // 通过URL参数传token
    ws = new WebSocket(`${protocol}//${window.location.host}/api/dashboard/ws?token=${token}`);
    window.__dashWs = ws;  // 暴露给 main.js 以便页面切换时关闭
    ws.onopen = () => {
        console.log('✅ WebSocket连接成功');
        // 清除重连定时器
        if (wsReconnectTimer) {
            clearTimeout(wsReconnectTimer);
            wsReconnectTimer = null;
        }
    };
    
    // 接收消息
    ws.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleWebSocketMessage(data);
        } catch (error) {
            console.error('WebSocket消息解析失败:', error);
        }
    };
    
    // 连接错误
    ws.onerror = (error) => {
        console.error('❌ WebSocket错误:', error);
        window.__dashWs = null;
    };
    
    // 连接关闭
    ws.onclose = () => {
        console.log('⚠️ WebSocket连接关闭');
        window.__dashWs = null;
        // 人为关闭（页面切换），不重连
        if (window.__dashWsIntentionalClose) {
            window.__dashWsIntentionalClose = false;
            return;
        }
        // 随机2-4秒后重连(避免惊群效应)
        if (!wsReconnectTimer) {
            const delay = 2000 + Math.random() * 2000;
            wsReconnectTimer = setTimeout(() => {
                console.log('🔄 尝试重连WebSocket...');
                connectWebSocket();
            }, delay);
        }
    };
}

// 处理WebSocket消息
function handleWebSocketMessage(data) {
    switch(data.type) {
        case 'initial':
            // 初始数据
            updateDashboardUI(data);
            console.log('📊 收到初始数据');
            break;
            
        case 'realtime':
            // 实时更新并发数
            updateRealtimeStats(data);
            break;
            
        case 'stats':
            // 更新今日统计
            updateTodayStats(data);
            console.log('📈 收到统计数据');
            break;
    }
}

// 更新实时数据(并发数)
function updateRealtimeStats(data) {
    const statInboundCalls = document.getElementById('statInboundCalls');
    if (statInboundCalls) {
        statInboundCalls.textContent = data.inbound_calls || 0;
    }
    
    const statOutboundCalls = document.getElementById('statOutboundCalls');
    if (statOutboundCalls) {
        statOutboundCalls.textContent = data.outbound_calls || 0;
    }
    
    // 注: 网关表格在初始加载时已渲染,实时更新时不需要重复渲染
    // 如需实时更新网关表格,需后端推送网关列表数据
}

// 更新今日统计
function updateTodayStats(data) {
    const statTotalCalls = document.getElementById('statTotalCalls');
    if (statTotalCalls) {
        statTotalCalls.textContent = data.total_calls || 0;
    }
    
    const statAnswerRate = document.getElementById('statAnswerRate');
    if (statAnswerRate) {
        statAnswerRate.textContent = data.answer_rate ? data.answer_rate.toFixed(1) + '%' : '0%';
    }
}

// 更新整个仪表盘UI
function updateDashboardUI(data) {
    updateRealtimeStats(data);
    updateTodayStats(data);
}

// 加载仪表盘数据(初始加载)
async function loadDashboardData() {
    try {
        const res = await api('/dashboard/stats');
        if (!res) return;
        
        const data = await res.json();
        updateDashboardUI(data);
        
        // 更新网关表格
        updateGatewayTables(data);
        
        // 更新失败TOP5
        updateFailedCallsTable(data);
    } catch (error) {
        console.error('加载仪表盘数据失败:', error);
    }
}

// 更新网关表格
function updateGatewayTables(data) {
    // 入局网关表格
    const inboundTable = document.getElementById('inboundGatewayTable');
    if (inboundTable && data.inbound_gateways) {
        if (Array.isArray(data.inbound_gateways) && data.inbound_gateways.length > 0) {
            inboundTable.innerHTML = data.inbound_gateways.map(gw => {
                const usage = gw.max_concurrent > 0 ? (gw.current_calls / gw.max_concurrent * 100).toFixed(1) : 0;
                const ips = gw.ips.split('\n').filter(ip => ip.trim()).slice(0, 3).join(', ') + (gw.ips.split('\n').length > 3 ? '...' : '');
                return `
                    <tr>
                        <td>${gw.name}</td>
                        <td title="${gw.ips}">${ips}</td>
                        <td>${gw.current_calls || 0}</td>
                        <td>${gw.max_concurrent}</td>
                        <td>
                            <div style="background: #f0f0f0; border-radius: 4px; overflow: hidden; height: 20px; position: relative;">
                                <div style="background: ${usage > 80 ? '#ff4d4f' : '#52c41a'}; height: 100%; width: ${Math.min(usage, 100)}%; transition: width 0.3s;"></div>
                                <span style="position: absolute; right: 8px; top: 50%; transform: translateY(-50%); font-size: 12px;">${usage}%</span>
                            </div>
                        </td>
                    </tr>
                `;
            }).join('');
        } else {
            inboundTable.innerHTML = '<tr><td colspan="5" style="text-align: center; color: #999;">暂无数据</td></tr>';
        }
    }
    
    // 出局网关表格
    const outboundTable = document.getElementById('outboundGatewayTable');
    if (outboundTable && data.outbound_gateways) {
        if (Array.isArray(data.outbound_gateways) && data.outbound_gateways.length > 0) {
            outboundTable.innerHTML = data.outbound_gateways.map(gw => {
                const usage = gw.max_concurrent > 0 ? (gw.current_calls / gw.max_concurrent * 100).toFixed(1) : 0;
                return `
                    <tr>
                        <td>${gw.name}</td>
                        <td>${gw.ip}:${gw.port}</td>
                        <td>${gw.current_calls || 0}</td>
                        <td>${gw.max_concurrent}</td>
                        <td>
                            <div style="background: #f0f0f0; border-radius: 4px; overflow: hidden; height: 20px; position: relative;">
                                <div style="background: ${usage > 80 ? '#ff4d4f' : '#52c41a'}; height: 100%; width: ${Math.min(usage, 100)}%; transition: width 0.3s;"></div>
                                <span style="position: absolute; right: 8px; top: 50%; transform: translateY(-50%); font-size: 12px;">${usage}%</span>
                            </div>
                        </td>
                    </tr>
                `;
            }).join('');
        } else {
            outboundTable.innerHTML = '<tr><td colspan="5" style="text-align: center; color: #999;">暂无数据</td></tr>';
        }
    }
}

// 更新失败呼叫表格
function updateFailedCallsTable(data) {
    const failedTable = document.getElementById('failedCallsTable');
    if (failedTable && data.failed_calls_top) {
        if (Array.isArray(data.failed_calls_top) && data.failed_calls_top.length > 0) {
            failedTable.innerHTML = data.failed_calls_top.map(item => `
                <tr>
                    <td>${item.outbound_gateway_ip}</td>
                    <td>${item.status_code}</td>
                    <td>${item.status_text || getStatusText(item.status_code)}</td>
                    <td style="color: #ff4d4f; font-weight: bold;">${item.count}</td>
                </tr>
            `).join('');
        } else {
            failedTable.innerHTML = '<tr><td colspan="4" style="text-align: center; color: #999;">暂无失败记录</td></tr>';
        }
    }
}

function getStatusText(code) {
    if (!code) return '未知';
    
    // ✅ 内部错误（负数）
    if (code < 0) {
        if (code === -503) return 'CPS超限';
        if (code === -500) return '路由选择失败';
        return `内部错误(${code})`;
    }
    
    // UAS返回的SIP响应码（正数）
    const sipStatusMap = {
        0: '呼叫中',
        200: '成功',
        408: '无应答',
        486: '忙',
        487: '取消',
        500: '服务器错误',
        503: '服务不可用'
    };
    return sipStatusMap[code] || '未知';
}
