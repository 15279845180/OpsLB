// 出局网关页面逻辑
window.addEventListener('pageLoaded', () => {
    loadOutboundGateways();
    
    // 表单提交
    const form = document.getElementById('outboundForm');
    if (form) {
        form.addEventListener('submit', handleOutboundSubmit);
    }
});

// 加载出局网关列表
async function loadOutboundGateways() {
    try {
        const res = await api('/outbound');
        if (!res) return;
        
        const data = await res.json();
        const tbody = document.getElementById('outboundTable');
        
        if (!tbody) {
            // 元素不存在(可能在其他页面),静默返回
            return;
        }
        
        if (!Array.isArray(data)) {
            tbody.innerHTML = '<tr><td colspan="9" style="text-align: center; color: #ff4d4f; padding: 40px;">数据加载失败</td></tr>';
            return;
        }
        
        if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="9" style="text-align: center; color: #999; padding: 40px;">暂无数据</td></tr>';
            return;
        }
        
        tbody.innerHTML = data.map(gw => {
            const statusClass = gw.status === 1 ? 'status-enabled' : gw.status === 2 ? 'status-paused' : 'status-disabled';
            const statusText = gw.status === 1 ? '启用' : gw.status === 2 ? '暂停' : '禁用';
            
            return `
                <tr>
                    <td>${gw.id}</td>
                    <td>${gw.name}</td>
                    <td>${gw.ip}:${gw.port}</td>
                    <td>${gw.priority}</td>
                    <td>${gw.weight}</td>
                    <td>${gw.current_calls || 0} / ${gw.max_concurrent}</td>
                    <td>${gw.max_cps || '不限'}</td>
                    <td><span class="status-badge ${statusClass}">${statusText}</span></td>
                    <td class="actions">
                        <button class="btn btn-primary btn-sm" onclick="editOutbound(${gw.id})">编辑</button>
                        <button class="btn btn-danger btn-sm" onclick="deleteOutbound(${gw.id})">删除</button>
                    </td>
                </tr>
            `;
        }).join('');
    } catch (error) {
        console.error('加载出局网关失败:', error);
    }
}

// 打开添加模态框
function openOutboundModal() {
    document.getElementById('outboundModalTitle').textContent = '添加出局网关';
    document.getElementById('outboundForm').reset();
    document.getElementById('outboundId').value = '';
    document.getElementById('outboundModal').classList.add('active');
}

// 关闭模态框
function closeOutboundModal() {
    document.getElementById('outboundModal').classList.remove('active');
}

// 编辑出局网关
async function editOutbound(id) {
    const res = await api(`/outbound/${id}`);
    if (!res) return;
    
    const gw = await res.json();
    document.getElementById('outboundModalTitle').textContent = '编辑出局网关';
    document.getElementById('outboundId').value = gw.id;
    document.getElementById('outboundName').value = gw.name;
    document.getElementById('outboundIP').value = gw.ip;
    document.getElementById('outboundPort').value = gw.port;
    document.getElementById('outboundProtocol').value = gw.protocol;
    document.getElementById('outboundPriority').value = gw.priority;
    document.getElementById('outboundWeight').value = gw.weight;
    document.getElementById('outboundMaxConcurrent').value = gw.max_concurrent;
    document.getElementById('outboundMaxCPS').value = gw.max_cps;
    document.getElementById('outboundStatus').value = gw.status;
    document.getElementById('outboundDesc').value = gw.description || '';
    document.getElementById('outboundModal').classList.add('active');
}

// 处理表单提交
async function handleOutboundSubmit(e) {
    e.preventDefault();
    
    const id = document.getElementById('outboundId').value;
    const data = {
        name: document.getElementById('outboundName').value,
        ip: document.getElementById('outboundIP').value,
        port: parseInt(document.getElementById('outboundPort').value),
        protocol: document.getElementById('outboundProtocol').value,
        priority: parseInt(document.getElementById('outboundPriority').value),
        weight: parseInt(document.getElementById('outboundWeight').value),
        max_concurrent: parseInt(document.getElementById('outboundMaxConcurrent').value),
        max_cps: parseInt(document.getElementById('outboundMaxCPS').value),
        status: parseInt(document.getElementById('outboundStatus').value),
        description: document.getElementById('outboundDesc').value
    };
    
    const url = id ? `/outbound/${id}` : '/outbound';
    const method = id ? 'PUT' : 'POST';
    
    const res = await api(url, { method, body: JSON.stringify(data) });
    if (res.ok) {
        closeOutboundModal();
        loadOutboundGateways();
    } else {
        const err = await res.json();
        alert(err.error || '操作失败');
    }
}

// 删除出局网关
async function deleteOutbound(id) {
    if (!confirm('确定要删除这个网关吗？')) return;
    
    const res = await api(`/outbound/${id}`, { method: 'DELETE' });
    if (res.ok) {
        loadOutboundGateways();
    } else {
        const err = await res.json();
        alert(err.error || '删除失败');
    }
}
