// UI Elements
const tabButtons = document.querySelectorAll('.tab-btn');
const tabContents = document.querySelectorAll('.tab-content');

const registerModal = document.getElementById('register-modal');
const btnOpenRegister = document.getElementById('btn-open-register');
const btnCloseRegister = document.getElementById('btn-close-register');
const btnCancelRegister = document.getElementById('btn-cancel-register');
const registerForm = document.getElementById('register-form');

// Hide API registration button if not admin
if (typeof IS_ADMIN !== 'undefined' && !IS_ADMIN) {
    btnOpenRegister.style.display = 'none';
}

const healthGrid = document.getElementById('health-grid');
const healthEmptyState = document.getElementById('health-empty-state');

// 1. Tab Navigation Logic
tabButtons.forEach(btn => {
    btn.addEventListener('click', () => {
        const targetTabId = btn.getAttribute('data-tab');

        tabButtons.forEach(b => b.classList.remove('active'));
        tabContents.forEach(c => c.classList.remove('active'));

        btn.classList.add('active');
        document.getElementById(targetTabId).classList.add('active');

        // Immediately fetch data on tab switch
        if (targetTabId === 'health-tab') {
            fetchHealthStatus();
        }
    });
});

// 2. Modal Overlay Logic
function openModal() {
    registerModal.classList.add('active');
    document.getElementById('api-name').focus();
}

function closeModal() {
    registerModal.classList.remove('active');
    registerForm.reset();
}

btnOpenRegister.addEventListener('click', openModal);
btnCloseRegister.addEventListener('click', closeModal);
btnCancelRegister.addEventListener('click', closeModal);

// Close modal when clicking outside the container
registerModal.addEventListener('click', (e) => {
    if (e.target === registerModal) {
        closeModal();
    }
});

// 3. API Target Registration Submit
registerForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    const name = document.getElementById('api-name').value;
    const url = document.getElementById('api-url').value;
    const interval = parseInt(document.getElementById('api-interval').value);
    const timeout = parseInt(document.getElementById('api-timeout').value);

    try {
        const response = await fetch('/api/health/targets', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                name: name,
                url: url,
                interval_seconds: interval,
                timeout_seconds: timeout
            })
        });

        if (!response.ok) {
            throw new Error(`등록 실패: ${response.statusText}`);
        }

        closeModal();
        fetchHealthStatus(); // Refresh UI
    } catch (error) {
        alert(`API 등록 중 오류 발생: ${error.message}`);
    }
});

// 4. API Target Delete Handler
async function deleteTarget(id, name) {
    if (!confirm(`'${name}' 헬스체크 대상을 삭제하시겠습니까?`)) {
        return;
    }

    try {
        const response = await fetch(`/api/health/targets?id=${id}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            throw new Error(`삭제 실패: ${response.statusText}`);
        }

        fetchHealthStatus(); // Refresh UI
    } catch (error) {
        alert(`대상 삭제 중 오류 발생: ${error.message}`);
    }
}

// 5. Generate Dots for History (10 Indicators)
function renderHistoryDots(history) {
    const totalDots = 10;
    let dotsHTML = '';
    const paddingCount = totalDots - history.length;

    // Gray placeholder dots
    for (let i = 0; i < paddingCount; i++) {
        dotsHTML += '<div class="history-dot" title="이력 없음"></div>';
    }

    // Success/Fail colored dots (left-to-right oldest-to-newest)
    for (let i = 0; i < history.length; i++) {
        const stateClass = history[i] === 1 ? 'success' : 'fail';
        const title = history[i] === 1 ? '정상 (성공)' : '장애 (실패)';
        dotsHTML += `<div class="history-dot ${stateClass}" title="${title}"></div>`;
    }

    return `<div class="history-dots">${dotsHTML}</div>`;
}

// 6. Update Existing API Card to Prevent Layout Flicker
function updateAPICard(cardElement, target) {
    const isOnline = target.status === 'ONLINE';

    if (isOnline) {
        cardElement.classList.remove('offline-state');
    } else {
        cardElement.classList.add('offline-state');
    }

    // Status Badge
    const badge = cardElement.querySelector('.status-badge');
    if (target.status === 'PENDING') {
        badge.className = 'status-badge offline';
        badge.style.color = 'var(--text-muted)';
        badge.style.borderColor = 'rgba(255, 255, 255, 0.05)';
        badge.innerHTML = '<div class="badge-dot" style="background-color:var(--text-muted);"></div>Pending';
    } else if (isOnline) {
        badge.className = 'status-badge online';
        badge.style.color = '';
        badge.style.borderColor = '';
        badge.innerHTML = '<div class="badge-dot"></div>Online';
    } else {
        badge.className = 'status-badge offline';
        badge.style.color = '';
        badge.style.borderColor = '';
        badge.innerHTML = '<div class="badge-dot"></div>Offline';
    }

    // Latency and Status Code Info
    cardElement.querySelector('.api-latency-val').textContent = target.status === 'PENDING' ? '-' : `${target.last_latency_ms}ms`;
    cardElement.querySelector('.api-status-code').textContent = target.status === 'PENDING' ? '-' : (target.last_status_code > 0 ? target.last_status_code : 'Err');

    // Last Check Timestamp
    const lastCheckText = target.status === 'PENDING' 
        ? '첫 체크 대기 중...' 
        : `최근 점검: ${new Date(target.last_check).toLocaleTimeString()}`;
    cardElement.querySelector('.api-last-check').textContent = lastCheckText;

    // History Dots
    cardElement.querySelector('.dots-container').innerHTML = renderHistoryDots(target.history || []);

    // Handle Error Message Box
    let errorBox = cardElement.querySelector('.error-box');
    if (!isOnline && target.error_message) {
        if (!errorBox) {
            errorBox = document.createElement('div');
            errorBox.className = 'error-box';
            cardElement.appendChild(errorBox);
        }
        errorBox.textContent = `⚠️ ${target.error_message}`;
    } else if (errorBox) {
        errorBox.remove();
    }
}

// 7. Create API Card from Scratch
function createAPICard(target) {
    const card = document.createElement('article');
    card.className = 'api-card';
    card.setAttribute('data-target-id', target.id);

    const isOnline = target.status === 'ONLINE';
    if (target.status === 'OFFLINE') {
        card.classList.add('offline-state');
    }

    const badgeClass = target.status === 'ONLINE' ? 'online' : 'offline';
    const badgeLabel = target.status;
    const lastCheckText = target.status === 'PENDING' 
        ? '첫 체크 대기 중...' 
        : `최근 점검: ${new Date(target.last_check).toLocaleTimeString()}`;

    const isAdmin = typeof IS_ADMIN !== 'undefined' ? IS_ADMIN : false;
    
    // URL 표시 형태 결정 (비인증 시 링크 차단 및 텍스트 렌더링)
    const urlHTML = isAdmin 
        ? `<a href="${target.url}" target="_blank" style="color:var(--color-blue); text-decoration:none;">${target.url}</a>`
        : `<span style="color:var(--text-muted); cursor:default;">${target.url}</span>`;
        
    // 삭제 버튼 표시 여부
    const deleteButtonHTML = isAdmin 
        ? `<button class="btn-delete" title="대상 삭제">🗑️</button>` 
        : '';

    card.innerHTML = `
        <div class="card-header">
            <div class="agent-info">
                <span class="agent-id" style="font-size:1.05rem;">${target.name}</span>
                <span class="last-seen" style="font-size:0.75rem; word-break:break-all; max-width:280px;">
                    ${urlHTML}
                </span>
            </div>
            <div style="display:flex; align-items:center; gap:0.5rem;">
                <div class="status-badge ${badgeClass}">
                    <div class="badge-dot"></div>
                    ${badgeLabel}
                </div>
                ${deleteButtonHTML}
            </div>
        </div>
        <div class="card-content" style="gap:0.75rem;">
            <div class="api-info-row">
                <span>⚡ 응답 속도</span>
                <span class="api-latency-val" style="font-weight:600;">${target.status === 'PENDING' ? '-' : target.last_latency_ms + 'ms'}</span>
            </div>
            <div class="api-info-row">
                <span>📟 응답 코드</span>
                <span class="api-status-code" style="font-weight:600;">${target.status === 'PENDING' ? '-' : (target.last_status_code > 0 ? target.last_status_code : 'Err')}</span>
            </div>
            <div class="api-info-row" style="flex-direction:column; gap:0.35rem; border-bottom:none;">
                <span style="font-size:0.8rem; color:var(--text-muted);">최근 10회 가동 이력 (주기: ${target.interval_seconds}초)</span>
                <div class="dots-container">
                    ${renderHistoryDots(target.history || [])}
                </div>
            </div>
            <div class="api-last-check" style="font-size:0.7rem; color:var(--text-muted); text-align:right;">
                ${lastCheckText}
            </div>
        </div>
    `;

    // Bind delete event only if admin
    if (isAdmin) {
        card.querySelector('.btn-delete').addEventListener('click', () => {
            deleteTarget(target.id, target.name);
        });
    }

    // Error Message Box
    if (target.status === 'OFFLINE' && target.error_message) {
        const errorBox = document.createElement('div');
        errorBox.className = 'error-box';
        errorBox.textContent = `⚠️ ${target.error_message}`;
        card.appendChild(errorBox);
    }

    healthGrid.appendChild(card);
}

// 8. Fetch Status and Update UI Grid
async function fetchHealthStatus() {
    // Only fetch if Health Check tab is active
    const activeTab = document.querySelector('.tab-btn.active');
    if (!activeTab || activeTab.getAttribute('data-tab') !== 'health-tab') {
        return;
    }

    try {
        const response = await fetch('/api/health/status');
        if (!response.ok) {
            throw new Error(`상태 조회 실패: ${response.status}`);
        }
        const targets = await response.json();

        if (!targets || targets.length === 0) {
            healthEmptyState.style.display = 'flex';
            document.querySelectorAll('.api-card').forEach(card => card.remove());
            return;
        }

        healthEmptyState.style.display = 'none';
        const currentTargetIds = new Set(targets.map(t => t.id));

        // Process status data
        targets.forEach(target => {
            const existingCard = document.querySelector(`.api-card[data-target-id="${target.id}"]`);
            if (existingCard) {
                updateAPICard(existingCard, target);
            } else {
                createAPICard(target);
            }
        });

        // Remove cards for deleted targets
        document.querySelectorAll('.api-card').forEach(card => {
            const id = parseInt(card.getAttribute('data-target-id'));
            if (!currentTargetIds.has(id)) {
                card.style.opacity = '0';
                card.style.transform = 'scale(0.9)';
                setTimeout(() => card.remove(), 300);
            }
        });

    } catch (error) {
        console.error('[HealthCheck] Status fetch failed:', error);
    }
}

// 9. Scheduler for Health Tab Polling (Runs every 5 seconds)
setInterval(fetchHealthStatus, 5000);
