// 系统设置页面逻辑
window.addEventListener('pageLoaded', () => {
    loadSettings();
});

// 加载系统设置
async function loadSettings() {
    try {
        const res = await api('/settings');
        if (!res) return;
        
        const data = await res.json();
        
        const callRecordEnabledEl = document.getElementById('callRecordEnabled');
        const callRecordRetentionDaysEl = document.getElementById('callRecordRetentionDays');
        
        if (callRecordEnabledEl) callRecordEnabledEl.value = data.call_record_enabled || '1';
        if (callRecordRetentionDaysEl) callRecordRetentionDaysEl.value = data.call_record_retention_days || '7';
    } catch (error) {
        console.error('加载设置失败:', error);
    }
}

// 保存设置
async function saveSettings() {
    const callRecordEnabledEl = document.getElementById('callRecordEnabled');
    const callRecordRetentionDaysEl = document.getElementById('callRecordRetentionDays');
    
    if (!callRecordEnabledEl || !callRecordRetentionDaysEl) {
        alert('❌ 页面元素未找到，请刷新重试');
        return;
    }
    
    const config = {
        call_record_enabled: callRecordEnabledEl.value,
        call_record_retention_days: callRecordRetentionDaysEl.value
    };
    
    try {
        const res = await api('/settings', {
            method: 'PUT',
            body: JSON.stringify(config)
        });
        
        if (res.ok) {
            alert('✅ 设置已保存');
        } else {
            const err = await res.json();
            alert('❌ 保存失败: ' + (err.error || '未知错误'));
        }
    } catch (error) {
        console.error('保存设置失败:', error);
        alert('❌ 保存失败: ' + error.message);
    }
}
