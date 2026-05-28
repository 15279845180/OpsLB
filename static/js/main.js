// 全局配置
const API_BASE = '/api';
let token = localStorage.getItem('token');
let currentUser = localStorage.getItem('username');

// 标签页管理
const tabs = new Map();
let activeTab = null;

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    // 初始化 Lucide 图标
    if (typeof lucide !== 'undefined') {
        lucide.createIcons();
    }
    
    if (token) {
        showMainApp();
    }
    
    // 登录表单
    document.getElementById('loginForm').addEventListener('submit', handleLogin);
    
    // 菜单点击
    document.querySelectorAll('.menu-item').forEach(item => {
        item.addEventListener('click', () => {
            const page = item.dataset.page;
            const title = item.dataset.title;
            openTab(page, title);
        });
    });
});

// 登录处理
async function handleLogin(e) {
    e.preventDefault();
    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;

    try {
        const res = await fetch(`${API_BASE}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        const data = await res.json();
        
        if (res.ok) {
            token = data.token;
            currentUser = data.username;
            localStorage.setItem('token', token);
            localStorage.setItem('username', currentUser);
            showMainApp();
        } else {
            alert(data.error || '登录失败');
        }
    } catch (err) {
        alert('网络错误');
    }
}

// 显示主应用
function showMainApp() {
    document.getElementById('loginPage').classList.add('hidden');
    document.getElementById('appContainer').classList.remove('hidden');
    document.getElementById('userInfo').textContent = currentUser;
    
    // 重新初始化图标(确保动态内容中的图标也能显示)
    if (typeof lucide !== 'undefined') {
        lucide.createIcons();
    }
    
    // 默认打开仪表盘(作为第一个标签,不可关闭)
    tabs.set('dashboard', { page: 'dashboard', title: '仪表盘' });
    renderTabs();
    loadDashboardAsHome();
}

// 加载仪表盘作为首页(不创建标签)
async function loadDashboardAsHome() {
    const contentArea = document.getElementById('contentArea');
    contentArea.innerHTML = '<div class="loading">加载中...</div>';
    
    try {
        const response = await fetch('/pages/dashboard.html');
        if (!response.ok) throw new Error('页面加载失败');
        
        const html = await response.text();
        contentArea.innerHTML = html;
        
        await loadScript('/js/dashboard.js?t=' + Date.now());
        
        // 初始化图标
        if (typeof lucide !== 'undefined') {
            lucide.createIcons();
        }
        
        // 设置activeTab为dashboard
        activeTab = 'dashboard';
        
        // 等待DOM渲染完成后再触发事件
        setTimeout(() => {
            window.dispatchEvent(new CustomEvent('pageLoaded', { detail: { page: 'dashboard' } }));
        }, 100);
    } catch (error) {
        contentArea.innerHTML = `<div class="empty-state"><p>页面加载失败: ${error.message}</p></div>`;
    }
}

// 退出登录
function logout() {
    if (confirm('确定要退出登录吗？')) {
        localStorage.removeItem('token');
        localStorage.removeItem('username');
        location.reload();
    }
}

// 打开标签页
function openTab(page, title) {
    // 如果标签页已存在，切换到该标签
    if (tabs.has(page)) {
        switchTab(page);
        return;
    }
    
    // 创建新标签
    tabs.set(page, { page, title });
    renderTabs();
    switchTab(page);
    
    // 更新菜单激活状态
    document.querySelectorAll('.menu-item').forEach(item => {
        item.classList.toggle('active', item.dataset.page === page);
    });
}

// 返回仪表盘(切换到仪表盘标签)
function goToDashboard() {
    switchTab('dashboard');
    
    // 仪表盘没有对应的菜单项，所以清除所有菜单高亮
    document.querySelectorAll('.menu-item').forEach(item => {
        item.classList.remove('active');
    });
}

// 切换标签
function switchTab(page) {
    activeTab = page;
    
    // 更新标签激活状态
    document.querySelectorAll('.tab-item').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.page === page);
    });
    
    // 更新菜单激活状态
    document.querySelectorAll('.menu-item').forEach(item => {
        item.classList.toggle('active', item.dataset.page === page);
    });
    
    // 加载页面内容
    loadPage(page);
}

// 关闭标签
function closeTab(page, event) {
    event.stopPropagation();
    
    // 不能关闭仪表盘
    if (page === 'dashboard') return;
    
    // 不能关闭最后一个标签
    if (tabs.size === 1) return;
    
    tabs.delete(page);
    
    // 如果关闭的是当前标签，切换到仪表盘
    if (activeTab === page) {
        switchTab('dashboard');
    }
    
    renderTabs();
}

// 渲染标签
function renderTabs() {
    const tabsBar = document.getElementById('tabsBar');
    tabsBar.innerHTML = '';
    
    // 确保仪表盘始终在第一个位置
    if (!tabs.has('dashboard')) {
        tabs.set('dashboard', { page: 'dashboard', title: '仪表盘' });
    }
    
    // 按照顺序渲染: 先仪表盘, 再其他
    const orderedTabs = new Map();
    orderedTabs.set('dashboard', tabs.get('dashboard'));
    tabs.forEach((tab, page) => {
        if (page !== 'dashboard') {
            orderedTabs.set(page, tab);
        }
    });
    
    orderedTabs.forEach((tab, page) => {
        const tabElement = document.createElement('div');
        tabElement.className = `tab-item ${page === activeTab ? 'active' : ''}`;
        tabElement.dataset.page = page;
        
        tabElement.innerHTML = `
            <span>${tab.title}</span>
            ${page !== 'dashboard' ? '<span class="tab-close" onclick="closeTab(\'' + page + '\', event)">×</span>' : ''}
        `;
        
        tabElement.addEventListener('click', () => switchTab(page));
        tabsBar.appendChild(tabElement);
    });
}

// 加载页面内容
async function loadPage(page) {
    const contentArea = document.getElementById('contentArea');
    contentArea.innerHTML = '<div class="loading">加载中...</div>';

    // 关闭当前页面的WebSocket连接（避免连接泄漏）
    if (window.__dashWs && window.__dashWs.readyState === WebSocket.OPEN) {
        window.__dashWsIntentionalClose = true;  // 标记为人为关闭，阻止onclose重连
        window.__dashWs.close();
        window.__dashWs = null;
    }

    try {
        const response = await fetch(`/pages/${page}.html`);
        if (!response.ok) {
            throw new Error('页面加载失败');
        }
        const html = await response.text();
        contentArea.innerHTML = html;
        
        // 加载对应的 JS 文件
        await loadScript(`/js/${page}.js`);
        
        // 重新初始化图标
        if (typeof lucide !== 'undefined') {
            lucide.createIcons();
        }
        
        // 触发页面初始化事件
        window.dispatchEvent(new CustomEvent('pageLoaded', { detail: { page } }));
    } catch (error) {
        contentArea.innerHTML = `<div class="empty-state"><p>页面加载失败: ${error.message}</p></div>`;
    }
}

// 动态加载脚本
function loadScript(src) {
    return new Promise((resolve, reject) => {
        // 移除旧的脚本(忽略查询参数)
        const baseSrc = src.split('?')[0];
        document.querySelectorAll('script').forEach(script => {
            if (script.src && script.src.includes(baseSrc)) {
                script.remove();
            }
        });
        
        const script = document.createElement('script');
        script.src = `${src}?t=${Date.now()}`; // 添加时间戳避免缓存
        script.onload = resolve;
        script.onerror = reject;
        document.head.appendChild(script);
    });
}

// API 请求封装
async function api(url, options = {}) {
    const res = await fetch(API_BASE + url, {
        ...options,
        headers: {
            'Authorization': 'Bearer ' + token,
            'Content-Type': 'application/json',
            ...options.headers
        }
    });
    
    if (res.status === 401) {
        logout();
        return null;
    }
    
    return res;
}

// 格式化时间
function formatDateTime(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

// 格式化时长
function formatDuration(seconds) {
    if (!seconds) return '0秒';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    
    if (hours > 0) {
        return `${hours}小时${minutes}分钟`;
    } else if (minutes > 0) {
        return `${minutes}分钟${secs}秒`;
    } else {
        return `${secs}秒`;
    }
}

// 状态码转文本
function getStatusText(code) {
    const statusMap = {
        200: '成功',
        180: '振铃中',
        183: '会话进行中',
        408: '无应答',
        486: '用户忙',
        487: '已取消',
        488: '不可接受',
        500: '服务器错误',
        503: '服务不可用'
    };
    return statusMap[code] || `状态码 ${code}`;
}
