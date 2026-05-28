// 呼叫记录页面逻辑
// 使用var避免重复声明报错(每次切换页面都会重新加载脚本)
var currentPage = 1;
const pageSize = 50;

window.addEventListener('pageLoaded', () => {
    // 设置默认时间为今天 00:00 ~ 23:59
    const now = new Date();
    const today = now.toISOString().split('T')[0];
    
    const startTimeEl = document.getElementById('startTime');
    const endTimeEl = document.getElementById('endTime');
    
    if (startTimeEl) startTimeEl.value = `${today}T00:00`;
    if (endTimeEl) endTimeEl.value = `${today}T23:59`;
    
    // 初始化图标
    if (typeof lucide !== 'undefined') {
        lucide.createIcons();
    }
    
    loadCallRecords();
});

// 加载呼叫记录
async function loadCallRecords(page = 1) {
    currentPage = page;
    
    const startTimeEl = document.getElementById('startTime');
    const endTimeEl = document.getElementById('endTime');
    const callerNumberEl = document.getElementById('callerNumber');
    const calledNumberEl = document.getElementById('calledNumber');
    const statusCodeEl = document.getElementById('statusCode');
    const totalRecordsEl = document.getElementById('totalRecords');
    const tbodyEl = document.getElementById('callRecordsTable');
    const paginationEl = document.getElementById('pagination');
    
    if (!tbodyEl) {
        // 元素不存在(可能在其他页面),静默返回
        return;
    }
    
    tbodyEl.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 40px;">加载中...</td></tr>';
    
    const params = new URLSearchParams({
        page: page,
        page_size: pageSize,
        start_time: startTimeEl ? startTimeEl.value.replace('T', ' ') : '',
        end_time: endTimeEl ? endTimeEl.value.replace('T', ' ') : '',
        caller_number: callerNumberEl ? callerNumberEl.value : '',
        called_number: calledNumberEl ? calledNumberEl.value : '',
        status_code: statusCodeEl ? statusCodeEl.value.trim() : ''
    });
    
    try {
        const res = await api(`/call-records?${params}`);
        if (!res) return;
        
        const data = await res.json();
        
        if (!data || !Array.isArray(data.data)) {
            console.warn('数据格式异常:', data);
            if (totalRecordsEl) totalRecordsEl.textContent = '0';
            tbodyEl.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #999; padding: 40px;">暂无数据</td></tr>';
            if (paginationEl) paginationEl.innerHTML = '';
            return;
        }
        
        if (totalRecordsEl) totalRecordsEl.textContent = data.total || 0;
        
        if (data.data.length === 0) {
            tbodyEl.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #999; padding: 40px;">暂无数据</td></tr>';
            if (paginationEl) paginationEl.innerHTML = '';
            return;
        }
        
        tbodyEl.innerHTML = data.data.map(record => {
            const statusInfo = getStatusInfo(record);
            
            return `
                <tr>
                    <td style="font-family: 'Courier New', monospace; font-size: 13px;">${formatDateTime(record.invite_time)}</td>
                    <td>${record.caller_number || '-'}</td>
                    <td>${record.called_number || '-'}</td>
                    <td style="font-family: 'Courier New', monospace; font-size: 13px;">${record.inbound_ip || '-'}</td>
                    <td style="font-family: 'Courier New', monospace; font-size: 13px;">${record.outbound_gateway_ip || '-'}</td>
                    <td><span class="status-badge ${statusInfo.class}" title="${statusInfo.tooltip}">${statusInfo.text}</span></td>
                    <td>${formatDuration(record.talk_duration)}</td>
                </tr>
            `;
        }).join('');
        
        if (paginationEl) {
            renderPagination(data.total, page);
        }
    } catch (error) {
        console.error('加载呼叫记录失败:', error);
        tbodyEl.innerHTML = `<tr><td colspan="7" style="text-align: center; color: #ff4d4f; padding: 40px;">加载失败: ${error.message}</td></tr>`;
    }
}

// 渲染分页
function renderPagination(total, current) {
    const totalPages = Math.ceil(total / pageSize);
    const pagination = document.getElementById('pagination');
    
    if (!pagination) return;
    
    if (totalPages <= 1) {
        pagination.innerHTML = '';
        return;
    }
    
    let html = '';
    html += `<button ${current === 1 ? 'disabled' : ''} onclick="loadCallRecords(${current - 1})">上一页</button>`;
    
    for (let i = 1; i <= totalPages; i++) {
        if (i === 1 || i === totalPages || (i >= current - 2 && i <= current + 2)) {
            html += `<button class="${i === current ? 'active' : ''}" onclick="loadCallRecords(${i})">${i}</button>`;
        } else if (i === current - 3 || i === current + 3) {
            html += `<button disabled>...</button>`;
        }
    }
    
    html += `<button ${current === totalPages ? 'disabled' : ''} onclick="loadCallRecords(${current + 1})">下一页</button>`;
    
    pagination.innerHTML = html;
}

// 查询
function searchCallRecords() {
    loadCallRecords(1);
}

// 重置筛选
function resetFilters() {
    const now = new Date();
    const today = now.toISOString().split('T')[0];
    
    const startTimeEl = document.getElementById('startTime');
    const endTimeEl = document.getElementById('endTime');
    const callerNumberEl = document.getElementById('callerNumber');
    const calledNumberEl = document.getElementById('calledNumber');
    const statusCodeEl = document.getElementById('statusCode');
    
    if (startTimeEl) startTimeEl.value = `${today}T00:00`;
    if (endTimeEl) endTimeEl.value = `${today}T23:59`;
    if (callerNumberEl) callerNumberEl.value = '';
    if (calledNumberEl) calledNumberEl.value = '';
    if (statusCodeEl) statusCodeEl.value = '';
    
    loadCallRecords(1);
}

// 导出CSV - 修复权限问题
async function exportCallRecords() {
    const startTimeEl = document.getElementById('startTime');
    const endTimeEl = document.getElementById('endTime');
    const callerNumberEl = document.getElementById('callerNumber');
    const calledNumberEl = document.getElementById('calledNumber');
    const statusCodeEl = document.getElementById('statusCode');
    
    const params = new URLSearchParams({
        start_time: startTimeEl ? startTimeEl.value.replace('T', ' ') : '',
        end_time: endTimeEl ? endTimeEl.value.replace('T', ' ') : '',
        caller_number: callerNumberEl ? callerNumberEl.value : '',
        called_number: calledNumberEl ? calledNumberEl.value : '',
        status_code: statusCodeEl ? statusCodeEl.value : ''
    });
    
    // 使用fetch下载,携带token
    try {
        const response = await fetch(`${API_BASE}/call-records/export?${params}`, {
            headers: {
                'Authorization': 'Bearer ' + token
            }
        });
        
        if (!response.ok) {
            throw new Error('导出失败');
        }
        
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `call_records_${new Date().toISOString().split('T')[0]}.csv`;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);
    } catch (error) {
        console.error('导出失败:', error);
        alert('导出失败: ' + error.message);
    }
}

// 辅助函数
function formatDateTime(datetime) {
    if (!datetime) return '-';
    // 将 2026-04-19 13:12:57 +0800 CST 格式化为 2026/04/19 13:12:57
    const date = new Date(datetime);
    if (isNaN(date.getTime())) return datetime;
    
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    const seconds = String(date.getSeconds()).padStart(2, '0');
    
    return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
}

function formatDuration(seconds) {
    if (!seconds || seconds === 0) return '0秒';
    if (seconds < 60) return `${seconds}秒`;
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return secs > 0 ? `${minutes}分${secs}秒` : `${minutes}分钟`;
}

// 获取状态信息（综合判断）
// 根据 status_code 和 end_time 判断最终展示状态
function getStatusInfo(record) {
    const code = record.status_code;
    const endTime = record.end_time;
    
    // 默认状态
    let result = {
        text: '-',
        class: 'status-disabled',
        tooltip: ''
    };
    
    // status_code 为 0/null/undefined 表示呼叫中（还没收到最终响应）
    if (!code || code === 0) {
        result.text = '呼叫中';
        result.class = 'status-calling';  // 使用蓝色表示进行中
        result.tooltip = '呼叫进行中，等待响应...';
        return result;
    }
    
    // 内部错误（负数）- 显示中文友好提示，橙色
    if (code < 0) {
        const internalErrorMap = {
            '-1': 'CPS超频',
            '-2': '无可用出局网关',
            '-3': '超并发'
        };
        result.text = internalErrorMap[code] || `内部错误(${code})`;
        result.class = 'status-internal-error';
        result.tooltip = getStatusTooltip(code);
        return result;
    }
    
    // 200 成功状态 - 根据 end_time 判断是"通话中"还是"成功"
    if (code === 200) {
        // 如果没有结束时间，说明通话还在进行中
        if (!endTime || endTime === '0001-01-01T00:00:00Z' || new Date(endTime).getTime() <= 0) {
            result.text = '通话中';
            result.class = 'status-enabled';
            result.tooltip = '200 通话中';
        } else {
            result.text = '成功';
            result.class = 'status-enabled';
            result.tooltip = '200 成功接通';
        }
        return result;
    }
    
    // 其他 SIP 状态码 - 显示纯数字，红色
    result.text = String(code);
    result.class = 'status-disabled';
    result.tooltip = getStatusTooltip(code);
    return result;
}

// 获取状态tooltip提示文本
function getStatusTooltip(code) {
    if (!code) return '';
    
    // 内部错误
    // 错误码定义: -1=CPS超频, -2=无可用出局网关, -3=超并发
    if (code < 0) {
        const internalErrorMap = {
            '-1': 'CPS超过每秒限制',
            '-2': '无可用出局网关（请检查网关配置）',
            '-3': '出局网关并发已满'
        };
        return internalErrorMap[code] || `内部错误码: ${code}`;
    }
    
    // SIP状态码映射
    const sipStatusMap = {
        200: '成功',
        100: 'Trying',
        180: 'Ringing',
        183: 'Session Progress',
        302: 'Moved Temporarily',
        400: 'Bad Request',
        401: 'Unauthorized',
        403: 'Forbidden',
        404: 'Not Found',
        405: 'Method Not Allowed',
        407: 'Proxy Authentication Required',
        408: 'Request Timeout',
        480: 'Temporarily Unavailable',
        481: 'Call/Transaction Does Not Exist',
        482: 'Loop Detected',
        483: 'Too Many Hops',
        484: 'Address Incomplete',
        485: 'Ambiguous',
        486: 'Busy Here',
        487: 'Request Terminated',
        488: 'Not Acceptable Here',
        489: 'Bad Event',
        491: 'Request Pending',
        493: 'Undecipherable',
        500: 'Server Internal Error',
        501: 'Not Implemented',
        502: 'Bad Gateway',
        503: 'Service Unavailable',
        504: 'Server Time-out',
        505: 'Version Not Supported',
        513: 'Message Too Large',
        600: 'Busy Everywhere',
        603: 'Decline',
        604: 'Does Not Exist Anywhere',
        606: 'Not Acceptable'
    };
    
    if (sipStatusMap[code]) {
        return `${code} ${sipStatusMap[code]}`;
    }
    
    // 按范围分类
    if (code >= 100 && code < 200) return `${code} 临时响应`;
    if (code >= 200 && code < 300) return `${code} 成功`;
    if (code >= 300 && code < 400) return `${code} 重定向`;
    if (code >= 400 && code < 500) return `${code} 客户端错误`;
    if (code >= 500 && code < 600) return `${code} 服务器错误`;
    if (code >= 600) return `${code} 全局错误`;
    
    return `状态码 ${code}`;
}
