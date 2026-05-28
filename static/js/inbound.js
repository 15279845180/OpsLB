// 入局网关页面逻辑
window.addEventListener('pageLoaded', () => {
    loadInboundGateways();
    
    // 表单提交
    const form = document.getElementById('inboundForm');
    if (form) {
        form.addEventListener('submit', handleInboundSubmit);
    }
});

// 加载入局网关列表
async function loadInboundGateways() {
    try {
        const res = await api('/inbound');
        if (!res) return;
        
        const data = await res.json();
        const tbody = document.getElementById('inboundTable');
        
        if (!tbody) {
            // 元素不存在(可能在其他页面),静默返回
            return;
        }
        
        if (!Array.isArray(data)) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #ff4d4f; padding: 40px;">数据加载失败</td></tr>';
            return;
        }
        
        if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #999; padding: 40px;">暂无数据</td></tr>';
            return;
        }
        
        tbody.innerHTML = data.map(gw => {
            const ips = gw.ips.split('\n').filter(ip => ip.trim()).slice(0, 3).join('<br>') + 
                       (gw.ips.split('\n').length > 3 ? '<br>...' : '');
            const statusClass = gw.status === 1 ? 'status-enabled' : gw.status === 2 ? 'status-paused' : 'status-disabled';
            const statusText = gw.status === 1 ? '启用' : gw.status === 2 ? '暂停' : '禁用';
            
            return `
                <tr>
                    <td>${gw.id}</td>
                    <td>${gw.name}</td>
                    <td>${ips}</td>
                    <td>${gw.current_calls || 0} / ${gw.max_concurrent}</td>
                    <td>${gw.max_concurrent}</td>
                    <td><span class="status-badge ${statusClass}">${statusText}</span></td>
                    <td class="actions">
                        <button class="btn btn-primary btn-sm" onclick="editInbound(${gw.id})">编辑</button>
                        <button class="btn btn-danger btn-sm" onclick="deleteInbound(${gw.id})">删除</button>
                    </td>
                </tr>
            `;
        }).join('');
    } catch (error) {
        console.error('加载入局网关失败:', error);
    }
}

// 打开添加模态框
function openInboundModal() {
    document.getElementById('inboundModalTitle').textContent = '添加入局网关';
    document.getElementById('inboundForm').reset();
    document.getElementById('inboundId').value = '';
    document.getElementById('inboundModal').classList.add('active');
}

// 关闭模态框
function closeInboundModal() {
    document.getElementById('inboundModal').classList.remove('active');
}

// 编辑入局网关
async function editInbound(id) {
    const res = await api(`/inbound/${id}`);
    if (!res) return;
    
    const gw = await res.json();
    document.getElementById('inboundModalTitle').textContent = '编辑入局网关';
    document.getElementById('inboundId').value = gw.id;
    document.getElementById('inboundName').value = gw.name;
    document.getElementById('inboundIPs').value = gw.ips;
    document.getElementById('inboundMaxConcurrent').value = gw.max_concurrent;
    document.getElementById('inboundStatus').value = gw.status;
    document.getElementById('inboundDesc').value = gw.description || '';
    document.getElementById('inboundModal').classList.add('active');
}

// 处理表单提交
async function handleInboundSubmit(e) {
    e.preventDefault();
    
    const id = document.getElementById('inboundId').value;
    const data = {
        name: document.getElementById('inboundName').value,
        ips: document.getElementById('inboundIPs').value,
        max_concurrent: parseInt(document.getElementById('inboundMaxConcurrent').value),
        status: parseInt(document.getElementById('inboundStatus').value),
        description: document.getElementById('inboundDesc').value
    };
    
    const url = id ? `/inbound/${id}` : '/inbound';
    const method = id ? 'PUT' : 'POST';
    
    const res = await api(url, { method, body: JSON.stringify(data) });
    if (res.ok) {
        closeInboundModal();
        loadInboundGateways();
    } else {
        const err = await res.json();
        alert(err.error || '操作失败');
    }
}

// 删除入局网关
async function deleteInbound(id) {
    if (!confirm('确定要删除这个网关吗？')) return;
    
    const res = await api(`/inbound/${id}`, { method: 'DELETE' });
    if (res.ok) {
        loadInboundGateways();
    } else {
        const err = await res.json();
        alert(err.error || '删除失败');
    }
}

// 刷新 OpenSIPS
async function reloadOpenSIPS() {
    const btn = event.target;
    btn.disabled = true;
    btn.textContent = '保存中...';
    
    try {
        const res = await api('/inbound/reload', { method: 'POST' });
        const data = await res.json();
        
        if (res.ok) {
            alert('✅ 配置已保存并生效');
        } else {
            alert('❌ 保存失败: ' + (data.error || '未知错误'));
        }
    } catch (err) {
        alert('❌ 网络错误: ' + err.message);
    } finally {
        btn.disabled = false;
        btn.textContent = '💾 保存并生效';
    }
}
