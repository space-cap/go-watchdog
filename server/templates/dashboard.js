const grid = document.getElementById('dashboard-grid');
const emptyState = document.getElementById('empty-state');

// Format relative time (e.g. 5 seconds ago)
function formatRelativeTime(timestampStr) {
    const date = new Date(timestampStr);
    const now = new Date();
    const diffSeconds = Math.floor((now - date) / 1000);

    if (isNaN(diffSeconds)) return '시간 모름';
    if (diffSeconds < 5) return '방금 전';
    if (diffSeconds < 60) return `${diffSeconds}초 전`;
    
    const diffMinutes = Math.floor(diffSeconds / 60);
    return `${diffMinutes}분 전`;
}

// Get progress bar color class based on percentage
function getProgressBarClass(percent) {
    if (percent >= 90) return 'progress-bar bar-danger';
    if (percent >= 70) return 'progress-bar bar-warn';
    return 'progress-bar bar-normal';
}

// Create HTML string for a single Disk
function createDiskHTML(disk) {
    const diskClass = getProgressBarClass(disk.percent);
    return `
        <div class="metric-group">
            <div class="metric-header">
                <span class="metric-label" style="font-size:0.8rem; color:var(--text-secondary); font-weight:500;">
                    📂 ${disk.path} (${disk.used_gb.toFixed(1)} / ${disk.total_gb.toFixed(1)} GB)
                </span>
                <span class="metric-value" style="font-size:0.8rem;">${disk.percent.toFixed(1)}%</span>
            </div>
            <div class="progress-container" style="height: 6px;">
                <div class="${diskClass}" style="width: ${disk.percent}%"></div>
            </div>
        </div>
    `;
}

// Update values of an existing card in place to prevent layout flicker
function updateCard(cardElement, agent) {
    const isOnline = agent.status === 'ONLINE';
    
    // Handle card border state
    if (isOnline) {
        cardElement.classList.remove('offline-state');
    } else {
        cardElement.classList.add('offline-state');
    }

    // Update Status Badge
    const badge = cardElement.querySelector('.status-badge');
    if (isOnline) {
        badge.className = 'status-badge online';
        badge.innerHTML = '<div class="badge-dot"></div>Online';
    } else {
        badge.className = 'status-badge offline';
        badge.innerHTML = '<div class="badge-dot"></div>Offline';
    }

    // Update Last Seen text
    const lastSeenText = isOnline 
        ? `마지막 응답: ${formatRelativeTime(agent.timestamp)}` 
        : `장비 오프라인 (${formatRelativeTime(agent.timestamp)})`;
    cardElement.querySelector('.last-seen').textContent = lastSeenText;

    // Update CPU usage
    const cpuVal = cardElement.querySelector('.cpu-val');
    cpuVal.textContent = `${agent.cpu_percent.toFixed(1)}%`;
    const cpuBar = cardElement.querySelector('.cpu-bar');
    cpuBar.className = 'cpu-bar ' + getProgressBarClass(agent.cpu_percent);
    cpuBar.style.width = `${agent.cpu_percent}%`;

    // Update RAM usage
    const ramVal = cardElement.querySelector('.ram-val');
    ramVal.textContent = `${agent.mem_percent.toFixed(1)}% (${agent.mem_used_gb.toFixed(1)} / ${agent.mem_total_gb.toFixed(1)} GB)`;
    const ramBar = cardElement.querySelector('.ram-bar');
    ramBar.className = 'ram-bar ' + getProgressBarClass(agent.mem_percent);
    ramBar.style.width = `${agent.mem_percent}%`;

    // Update Disk drive list (re-render disk sub-grid to match configuration)
    const diskGrid = cardElement.querySelector('.disk-grid');
    if (agent.disks && agent.disks.length > 0) {
        diskGrid.innerHTML = agent.disks.map(createDiskHTML).join('');
    } else {
        diskGrid.innerHTML = '<p style="font-size:0.8rem; color:var(--text-muted);">디스크 정보 없음</p>';
    }
}

// Create a new card from scratch
function createCard(agent) {
    const card = document.createElement('article');
    card.className = 'server-card';
    card.setAttribute('data-agent-id', agent.agent_id);

    const isOnline = agent.status === 'ONLINE';
    if (!isOnline) {
        card.classList.add('offline-state');
    }

    const badgeClass = isOnline ? 'online' : 'offline';
    const badgeLabel = isOnline ? 'Online' : 'Offline';
    const lastSeenText = isOnline 
        ? `마지막 응답: ${formatRelativeTime(agent.timestamp)}` 
        : `장비 오프라인 (${formatRelativeTime(agent.timestamp)})`;

    card.innerHTML = `
        <div class="card-header">
            <div class="agent-info">
                <span class="agent-id">${agent.agent_id}</span>
                <span class="last-seen">${lastSeenText}</span>
            </div>
            <div class="status-badge ${badgeClass}">
                <div class="badge-dot"></div>
                ${badgeLabel}
            </div>
        </div>
        <div class="card-content">
            <div class="metric-group">
                <div class="metric-header">
                    <span class="metric-label">💻 CPU 사용률</span>
                    <span class="metric-value cpu-val">${agent.cpu_percent.toFixed(1)}%</span>
                </div>
                <div class="progress-container">
                    <div class="cpu-bar" style="width: 0%"></div>
                </div>
            </div>
            <div class="metric-group">
                <div class="metric-header">
                    <span class="metric-label">🧠 메모리 사용량</span>
                    <span class="metric-value ram-val">${agent.mem_percent.toFixed(1)}% (${agent.mem_used_gb.toFixed(1)} / ${agent.mem_total_gb.toFixed(1)} GB)</span>
                </div>
                <div class="progress-container">
                    <div class="ram-bar" style="width: 0%"></div>
                </div>
            </div>
        </div>
        <div class="disk-section">
            <div class="disk-title">Storage Partition Status</div>
            <div class="disk-grid">
                ${agent.disks && agent.disks.length > 0 
                    ? agent.disks.map(createDiskHTML).join('') 
                    : '<p style="font-size:0.8rem; color:var(--text-muted);">디스크 정보 없음</p>'}
            </div>
        </div>
    `;

    grid.appendChild(card);

    // Animate progress bar widths on insertion
    setTimeout(() => {
        const cpuBar = card.querySelector('.cpu-bar');
        cpuBar.className = 'cpu-bar ' + getProgressBarClass(agent.cpu_percent);
        cpuBar.style.width = `${agent.cpu_percent}%`;

        const ramBar = card.querySelector('.ram-bar');
        ramBar.className = 'ram-bar ' + getProgressBarClass(agent.mem_percent);
        ramBar.style.width = `${agent.mem_percent}%`;
    }, 100);
}

// Fetch agents status from server
async function fetchStatus() {
    try {
        const response = await fetch('/api/status');
        if (!response.ok) {
            throw new Error(`Server returned status: ${response.status}`);
        }
        const agents = await response.json();

        if (!agents || agents.length === 0) {
            if (emptyState) emptyState.style.display = 'flex';
            // Remove any existing cards
            document.querySelectorAll('.server-card').forEach(card => card.remove());
            return;
        }

        if (emptyState) emptyState.style.display = 'none';

        // Keep track of active agent ids to clean up removed agents later
        const currentAgentIds = new Set(agents.map(a => a.agent_id));

        // Process agents and update/create cards
        agents.forEach(agent => {
            const existingCard = document.querySelector(`.server-card[data-agent-id="${agent.agent_id}"]`);
            if (existingCard) {
                updateCard(existingCard, agent);
            } else {
                createCard(agent);
            }
        });

        // Remove cards for agents that are no longer returned by the server
        document.querySelectorAll('.server-card').forEach(card => {
            const id = card.getAttribute('data-agent-id');
            if (!currentAgentIds.has(id)) {
                card.style.opacity = '0';
                card.style.transform = 'scale(0.9)';
                setTimeout(() => card.remove(), 300);
            }
        });

    } catch (error) {
        console.error('[Dashboard] Status fetch failed:', error);
        // Modify system status bar to warn
        const statusSpan = document.querySelector('.system-status-indicator span');
        const statusDot = document.querySelector('.pulse-dot');
        if (statusSpan && statusDot) {
            statusSpan.textContent = 'API Connection Error';
            statusSpan.style.color = 'var(--color-rose)';
            statusDot.style.backgroundColor = 'var(--color-rose)';
        }
    }
}

// Run fetch on load and schedule every 5 seconds
fetchStatus();
setInterval(fetchStatus, 5000);
