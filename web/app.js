const currentYear = new Date().getFullYear();

const state = {
    currentTrack: null,
    socket: null,
    locked: false,
    lastRecs: [],
    sort: { field: 'score', dir: 'desc' },
    filters: {
        key: true,
        genre: false,
        energyStep: false,
        hidePlayed: false,
        bpmMin: '',
        bpmMax: '',
        maxResults: 50,
        yearMin: currentYear,
        yearMax: currentYear,
        yearSource: 'added' // 'added' | 'year'
    }
};

// DOM Elements
const els = {
    connectionStatus: document.getElementById('connectionStatus'),
    currentTitle: document.getElementById('currentTitle'),
    currentArtist: document.getElementById('currentArtist'),
    currentBPM: document.getElementById('currentBPM'),
    currentKey: document.getElementById('currentKey'),
    currentEnergy: document.getElementById('currentEnergy'),
    lockBtn: document.getElementById('lockBtn'),
    liveBadge: document.getElementById('liveBadge'),
    recommendationsList: document.getElementById('recommendationsList'),
    filterKey: document.getElementById('filterKey'),
    filterGenre: document.getElementById('filterGenre'),
    filterEnergyStep: document.getElementById('filterEnergyStep'),
    filterHidePlayed: document.getElementById('filterHidePlayed'),
    filterBpmMin: document.getElementById('filterBpmMin'),
    filterBpmMax: document.getElementById('filterBpmMax'),
    filterMaxResults: document.getElementById('filterMaxResults'),
    filterYearMin: document.getElementById('filterYearMin'),
    filterYearMax: document.getElementById('filterYearMax'),
    filterYearAll: document.getElementById('filterYearAll'),
    yearSourceBtns: document.querySelectorAll('.year-source-btn'),
    zoomSlider: document.getElementById('zoomSlider'),
    zoomValue: document.getElementById('zoomValue'),
    pill: document.getElementById('pill'),
    pillTitle: document.getElementById('pillTitle'),
    pillArtist: document.getElementById('pillArtist'),
    pillBPM: document.getElementById('pillBPM'),
    pillKey: document.getElementById('pillKey'),
    appContainer: document.getElementById('appContainer'),
    recommendationsSection: document.getElementById('recommendationsSection'),
    toggleFiltersBtn: document.getElementById('toggleFiltersBtn')
};

// ── Electron + expand/collapse state ──────────────────────────────────────
const isElectron = window.electronAPI && window.electronAPI.isElectron;
let collapseTimer = null;
let expanded = false;

function expand() {
    if (expanded) return;
    expanded = true;
    clearTimeout(collapseTimer);
    document.body.classList.add('expanded');
    if (isElectron) window.electronAPI.expandWindow();
}

function collapse() {
    if (!expanded) return;
    expanded = false;
    clearTimeout(collapseTimer);
    document.body.classList.remove('expanded');
    if (isElectron) window.electronAPI.collapseWindow();
}

function scheduleCollapse() {
    clearTimeout(collapseTimer);
    collapseTimer = setTimeout(collapse, 1500);
}

// mousemove fires whenever the cursor is inside the window — use it to
// expand on first entry and cancel any pending collapse timer.
document.addEventListener('mousemove', () => {
    if (!expanded) expand();
    else clearTimeout(collapseTimer);
});

// Only collapse once the mouse has genuinely left the window.
document.addEventListener('mouseleave', scheduleCollapse);

// ── Pill drag ──────────────────────────────────────────────────────────────
// -webkit-app-region: drag is unreliable on transparent frameless windows,
// so we implement custom drag via IPC.
(function setupPillDrag() {
    let dragging = false;
    let lastX = 0, lastY = 0;

    els.pill.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;
        dragging = true;
        lastX = e.screenX;
        lastY = e.screenY;
        e.preventDefault();
    });

    document.addEventListener('mousemove', (e) => {
        if (!dragging) return;
        const dx = e.screenX - lastX;
        const dy = e.screenY - lastY;
        lastX = e.screenX;
        lastY = e.screenY;
        if (isElectron) window.electronAPI.moveWindowBy(dx, dy);
    });

    document.addEventListener('mouseup', () => { dragging = false; });
})();

// ── Zoom ───────────────────────────────────────────────────────────────────
function applyZoom(pct) {
    const factor = pct / 100;
    if (els.zoomSlider) els.zoomSlider.value = pct;
    if (els.zoomValue)  els.zoomValue.textContent = pct + '%';
    if (isElectron) window.electronAPI.setZoom(factor);
    localStorage.setItem('sc_zoom', pct);
}

function updatePill(track) {
    if (!track) return;
    els.pillTitle.textContent = track.Title || 'Unknown';
    els.pillArtist.textContent = track.Artist || '';
    els.pillBPM.textContent = track.BPM ? Math.round(track.BPM) + ' BPM' : '';
    els.pillKey.textContent = track.Key || '';
}

// Initialize
function init() {
    // Restore saved zoom
    const savedZoom = parseInt(localStorage.getItem('sc_zoom'), 10) || 100;
    applyZoom(savedZoom);

    // Set year input defaults
    els.filterYearMin.value = state.filters.yearMin;
    els.filterYearMax.value = state.filters.yearMax;

    connectWebSocket();
    setupEventListeners();
}

function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    state.socket = new WebSocket(wsUrl);

    state.socket.onopen = () => {
        updateConnectionStatus(true);
    };

    state.socket.onclose = () => {
        updateConnectionStatus(false);
        setTimeout(connectWebSocket, 3000);
    };

    state.socket.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        handleMessage(msg);
    };
}

function updateConnectionStatus(connected) {
    if (connected) {
        els.connectionStatus.innerHTML = '<span class="dot"></span><span class="text">Connected</span>';
        els.connectionStatus.style.color = 'var(--success-color)';
    } else {
        els.connectionStatus.innerHTML = '<span class="dot" style="background: #ef4444; box-shadow: 0 0 8px #ef4444;"></span><span class="text">Disconnected</span>';
        els.connectionStatus.style.color = '#ef4444';
    }
}

function handleMessage(msg) {
    if (msg.type === 'current_track') {
        if (state.locked) return; // ignore watcher updates while locked
        updateCurrentTrack(msg.data);
        fetchRecommendations();
    }
}

function updateCurrentTrack(track) {
    state.currentTrack = track;

    els.currentTitle.textContent = track.Title || 'Unknown Title';
    els.currentArtist.textContent = track.Artist || 'Unknown Artist';
    els.currentBPM.textContent = track.BPM ? Math.round(track.BPM) : '--';
    els.currentKey.textContent = track.Key || '--';
    els.currentEnergy.textContent = track.Energy > 0 ? track.Energy : '--';

    updatePill(track);

    // Animate
    const card = document.getElementById('currentTrackCard');
    card.classList.remove('active');
    void card.offsetWidth;
    card.classList.add('active');
}

async function fetchRecommendations() {
    if (!state.currentTrack) return;

    const params = new URLSearchParams({
        bpm: state.currentTrack.BPM || 0,
        key: state.currentTrack.Key || '',
        genre: state.currentTrack.Genres ? state.currentTrack.Genres[0] : '',
        energy: state.currentTrack.Energy || 0,
        match_key: state.filters.key ? '1' : '0',
        match_genre: state.filters.genre ? '1' : '0',
        energy_step: state.filters.energyStep ? '1' : '0',
        hide_played: state.filters.hidePlayed ? '1' : '0',
        max_results: state.filters.maxResults
    });

    if (state.filters.bpmMin) params.set('bpm_min', state.filters.bpmMin);
    if (state.filters.bpmMax) params.set('bpm_max', state.filters.bpmMax);
    if (state.filters.yearMin) params.set('year_min', state.filters.yearMin);
    if (state.filters.yearMax) params.set('year_max', state.filters.yearMax);
    if (state.filters.yearMin || state.filters.yearMax) params.set('year_source', state.filters.yearSource);

    try {
        const res = await fetch(`/api/recommendations?${params.toString()}`);
        const recs = await res.json();
        state.lastRecs = recs || [];
        renderRecommendations(state.lastRecs);
    } catch (err) {
        console.error('Failed to fetch recommendations:', err);
    }
}

function renderRecommendations(recs) {
    els.recommendationsList.innerHTML = '';

    if (!recs || recs.length === 0) {
        els.recommendationsList.innerHTML = '<div class="empty-state"><p>No recommendations found</p></div>';
        return;
    }

    // Sort a copy — don't mutate the stored recs
    const sorted = sortRecs([...recs]);

    // Sort header row
    const header = document.createElement('div');
    header.className = 'sort-header';
    [
        { label: 'Score',    field: 'score'  },
        { label: 'BPM',       field: 'bpm'    },
        { label: 'Key',       field: 'key'    },
        { label: 'Energy',    field: 'energy' },
    ].forEach(({ label, field }) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'sort-btn' + (state.sort.field === field ? ' active' : '');
        const arrow = state.sort.field === field
            ? (state.sort.dir === 'asc' ? ' ↑' : ' ↓')
            : '';
        btn.textContent = label + arrow;
        btn.addEventListener('click', () => {
            if (state.sort.field === field) {
                state.sort.dir = state.sort.dir === 'asc' ? 'desc' : 'asc';
            } else {
                state.sort.field = field;
                state.sort.dir = field === 'score' ? 'desc' : 'asc';
            }
            renderRecommendations(state.lastRecs);
        });
        header.appendChild(btn);
    });
    els.recommendationsList.appendChild(header);

    sorted.forEach(rec => {
        const artist   = rec.Track.Artist || '';
        const title    = rec.Track.Title  || '';
        const filePath = rec.Track.FilePath || '';
        const copyText = artist && title ? `${artist} ${title}` : (title || artist);

        const energyBadge = rec.Track.Energy > 0
            ? `<span class="rec-energy">E${rec.Track.Energy}</span>`
            : '';

        const inElectron = window.electronAPI && window.electronAPI.isElectron;
        const hintText   = inElectron ? 'Drag to deck  ·  Click to copy' : 'Click to copy';

        // Use <button> so browsers treat the click as a trusted user-activation
        // event, required for clipboard access.
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'rec-item';
        item.title = copyText;

        item.innerHTML = `
            <div class="rec-info">
                <div class="rec-title">${escapeHtml(title)}</div>
                <div class="rec-artist">${escapeHtml(artist)}</div>
            </div>
            <div class="rec-meta">
                <span>${Math.round(rec.Track.BPM)} BPM</span>
                <span>${rec.Track.Key || '--'}</span>
                ${energyBadge}
                ${rec.Reason ? `<span class="rec-match">${escapeHtml(rec.Reason)}</span>` : ''}
            </div>
            <div class="copy-hint">${hintText}</div>
        `;

        // Click → copy to clipboard
        item.addEventListener('click', () => copyToClipboard(item, copyText));

        // Drag → OS file drag (Electron only); Serato accepts it as CF_HDROP
        if (inElectron && filePath) {
            item.setAttribute('draggable', 'true');
            item.addEventListener('dragstart', (e) => {
                e.preventDefault(); // hand off to Electron's native drag
                window.electronAPI.startDrag(filePath);
            });
        }

        els.recommendationsList.appendChild(item);
    });
}

function sortRecs(recs) {
    const { field, dir } = state.sort;
    const mul = dir === 'asc' ? 1 : -1;

    return recs.sort((a, b) => {
        let av, bv;
        switch (field) {
            case 'bpm':
                av = a.Track.BPM    || 0;
                bv = b.Track.BPM    || 0;
                break;
            case 'key':
                av = a.Track.Key    || '';
                bv = b.Track.Key    || '';
                // Sort by Camelot number then mode (A before B)
                return mul * camelotSortKey(av).localeCompare(camelotSortKey(bv));
            case 'energy':
                av = a.Track.Energy || 0;
                bv = b.Track.Energy || 0;
                break;
            default: // 'score' — use server order (Score descending)
                av = a.Score || 0;
                bv = b.Score || 0;
                break;
        }
        return mul * (av - bv);
    });
}

// Returns a zero-padded sort string for Camelot keys e.g. "08A", "12B"
function camelotSortKey(key) {
    const m = key.match(/^(\d+)([AaBb])$/);
    if (!m) return key;
    return m[1].padStart(2, '0') + m[2].toUpperCase();
}

function copyToClipboard(itemEl, text) {
    navigator.clipboard.writeText(text).then(() => {
        showCopied(itemEl);
    }).catch(err => {
        console.error('Copy failed:', err.name, err.message);
        showError(itemEl);
    });
}

function showCopied(itemEl) {
    itemEl.classList.add('copied');
    const hint = itemEl.querySelector('.copy-hint');
    if (hint) hint.textContent = 'Copied!';
    setTimeout(() => {
        itemEl.classList.remove('copied');
        if (hint) hint.textContent = 'Click to copy to clipboard';
    }, 1500);
}

function showError(itemEl) {
    const hint = itemEl.querySelector('.copy-hint');
    if (hint) {
        hint.textContent = 'Copy failed — check browser console';
        hint.style.color = '#f87171';
        setTimeout(() => {
            hint.textContent = 'Click to copy to clipboard';
            hint.style.color = '';
        }, 3000);
    }
}

function escapeHtml(str) {
    if (!str) return '';
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function setupEventListeners() {
    els.filterKey.addEventListener('change', (e) => {
        state.filters.key = e.target.checked;
        fetchRecommendations();
    });

    els.filterGenre.addEventListener('change', (e) => {
        state.filters.genre = e.target.checked;
        fetchRecommendations();
    });

    els.filterEnergyStep.addEventListener('change', (e) => {
        state.filters.energyStep = e.target.checked;
        fetchRecommendations();
    });

    els.filterHidePlayed.addEventListener('change', (e) => {
        state.filters.hidePlayed = e.target.checked;
        fetchRecommendations();
    });

    // BPM inputs — debounced
    let bpmDebounce = null;
    const onBpmChange = () => {
        clearTimeout(bpmDebounce);
        bpmDebounce = setTimeout(() => {
            state.filters.bpmMin = els.filterBpmMin.value;
            state.filters.bpmMax = els.filterBpmMax.value;
            fetchRecommendations();
        }, 400);
    };
    els.filterBpmMin.addEventListener('input', onBpmChange);
    els.filterBpmMax.addEventListener('input', onBpmChange);

    els.filterMaxResults.addEventListener('change', (e) => {
        state.filters.maxResults = parseInt(e.target.value, 10) || 50;
        fetchRecommendations();
    });

    // Year range — debounced
    let yearDebounce = null;
    const onYearChange = () => {
        clearTimeout(yearDebounce);
        yearDebounce = setTimeout(() => {
            state.filters.yearMin = parseInt(els.filterYearMin.value, 10) || 0;
            state.filters.yearMax = parseInt(els.filterYearMax.value, 10) || 0;
            // If both cleared, treat as All
            const isAll = !state.filters.yearMin && !state.filters.yearMax;
            els.filterYearAll.classList.toggle('active', isAll);
            fetchRecommendations();
        }, 400);
    };
    els.filterYearMin.addEventListener('input', onYearChange);
    els.filterYearMax.addEventListener('input', onYearChange);

    els.filterYearAll.addEventListener('click', () => {
        state.filters.yearMin = 0;
        state.filters.yearMax = 0;
        els.filterYearMin.value = '';
        els.filterYearMax.value = '';
        els.filterYearAll.classList.add('active');
        fetchRecommendations();
    });

    els.yearSourceBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            state.filters.yearSource = btn.dataset.source;
            els.yearSourceBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            fetchRecommendations();
        });
    });

    if (els.zoomSlider) {
        els.zoomSlider.addEventListener('input', () => {
            applyZoom(parseInt(els.zoomSlider.value, 10));
        });
    }

    // ── Filters panel collapse ──────────────────────────────────────────────
    const FILTERS_KEY = 'sc_filters_visible';
    function setFiltersVisible(visible, save = true) {
        els.recommendationsSection.classList.toggle('filters-hidden', !visible);
        els.toggleFiltersBtn.textContent = visible ? 'Filters ▲' : 'Filters ▼';
        if (save) localStorage.setItem(FILTERS_KEY, visible ? '1' : '0');
    }
    // Restore saved preference (default: visible)
    const savedFiltersVisible = localStorage.getItem(FILTERS_KEY);
    setFiltersVisible(savedFiltersVisible !== '0', false);

    els.toggleFiltersBtn.addEventListener('click', () => {
        const nowHidden = els.recommendationsSection.classList.contains('filters-hidden');
        setFiltersVisible(nowHidden); // toggle: if currently hidden, make visible
    });

    els.lockBtn.addEventListener('click', () => {
        if (state.locked) {
            // Sync to current track then re-lock
            fetch('/api/current-track')
                .then(r => r.json())
                .then(track => {
                    if (track) {
                        updateCurrentTrack(track);
                        fetchRecommendations();
                    }
                });
        } else {
            // Locking — just update badge/button
            state.locked = true;
            els.lockBtn.textContent = '\u21BA Sync';
            els.lockBtn.classList.add('locked');
            els.liveBadge.textContent = 'LOCKED';
            els.liveBadge.classList.add('locked');
        }
    });
}

// Start
init();
