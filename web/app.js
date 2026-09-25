const currentYear = new Date().getFullYear();

const state = {
    currentTrack: null,
    socket: null,
    locked: false,
    lastRecs: [],
    settings: null, // server settings (defaults)
    sort: { field: 'score', dir: 'desc' },
    filters: {
        key: true,
        genre: false,
        energyMode: 'within', // any|exact|within|higher|lower
        energyRange: 1,
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
    filterEnergyMode: document.getElementById('filterEnergyMode'),
    filterEnergyRange: document.getElementById('filterEnergyRange'),
    energyRangeGroup: document.getElementById('energyRangeGroup'),
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
    toggleFiltersBtn: document.getElementById('toggleFiltersBtn'),
    settingsBtn: document.getElementById('settingsBtn'),
    settingsOverlay: document.getElementById('settingsOverlay'),
    settingsCloseBtn: document.getElementById('settingsCloseBtn'),
    settingsSaved: document.getElementById('settingsSaved'),
    sessionStats: document.getElementById('sessionStats')
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
    document.body.classList.remove('idle');
    if (isElectron) window.electronAPI.expandWindow();
}

function collapse() {
    if (!expanded) return;
    expanded = false;
    clearTimeout(collapseTimer);
    document.body.classList.remove('expanded');
    if (isElectron) window.electronAPI.collapseWindow();
}

function toggleExpand() {
    if (expanded) collapse();
    else expand();
}

function scheduleCollapse() {
    clearTimeout(collapseTimer);
    collapseTimer = setTimeout(collapse, 1500);
}

// mousemove fires whenever the cursor is inside the window — use it to
// expand on first entry, cancel any pending collapse timer, and reset idle dim.
document.addEventListener('mousemove', () => {
    document.body.classList.remove('idle');
    resetIdleDim();
    if (!expanded) expand();
    else clearTimeout(collapseTimer);
});

// Only collapse once the mouse has genuinely left the window.
document.addEventListener('mouseleave', scheduleCollapse);

// ── Idle dim (collapsed only) ─────────────────────────────────────────────
let idleDimTimer = null;

function resetIdleDim() {
    clearTimeout(idleDimTimer);
    const s = state.settings;
    if (!s || !s.dim_enabled) return;
    idleDimTimer = setTimeout(() => {
        if (!expanded) document.body.classList.add('idle');
    }, (s.dim_delay_sec || 8) * 1000);
}

// ── Double-click pill toggles expand/collapse ─────────────────────────────
els.pill.addEventListener('dblclick', toggleExpand);

// Global hotkey from Electron main (via settings)
if (isElectron && window.electronAPI.onToggleExpand) {
    window.electronAPI.onToggleExpand(toggleExpand);
}

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

    // Right-click anywhere in the window opens a native menu with
    // "Reset Window Size" — escape hatch when the window is oversized and
    // the settings gear is unreachable.
    if (isElectron && window.electronAPI.showContextMenu) {
        document.addEventListener('contextmenu', (e) => {
            // Don't hijack right-clicks on text inputs
            if (e.target.closest('input, textarea, select')) return;
            e.preventDefault();
            window.electronAPI.showContextMenu();
        });
    }

    loadSettings();
    connectWebSocket();
    setupEventListeners();
    setupSettingsPanel();
    setupShortcuts();
    refreshSessionStats();
    resetIdleDim();
    setInterval(refreshSessionStats, 60000);
}

async function loadSettings() {
    try {
        const res = await fetch('/api/settings');
        state.settings = await res.json();
        applySettingsToUI();
    } catch (err) {
        console.error('Failed to load settings:', err);
    }
}

// Push settings defaults into the filter panel (settings panel populated separately)
function applySettingsToUI() {
    const s = state.settings;
    if (!s) return;

    state.filters.energyMode = s.energy_mode || 'within';
    state.filters.energyRange = s.energy_range || 1;
    state.filters.hidePlayed = !!s.hide_played;
    if (s.max_results > 0 || s.max_results === 0) {
        state.filters.maxResults = s.max_results || 50;
    }

    els.filterEnergyMode.value = state.filters.energyMode;
    els.filterEnergyRange.value = String(state.filters.energyRange);
    els.energyRangeGroup.style.display = state.filters.energyMode === 'within' ? '' : 'none';
    els.filterHidePlayed.checked = state.filters.hidePlayed;
    els.filterMaxResults.value = String(state.filters.maxResults);
}

// ── Session stats ─────────────────────────────────────────────────────────
async function refreshSessionStats() {
    if (!state.settings || state.settings.session_stats === false) {
        els.sessionStats.style.display = 'none';
        return;
    }
    try {
        const res = await fetch('/api/session-stats');
        const st = await res.json();
        if (!st || (!st.count && !(st.top_genres || []).length)) {
            els.sessionStats.style.display = 'none';
            return;
        }
        els.sessionStats.style.display = '';
        const genres = (st.top_genres || []).slice(0, 3).join(' · ');
        els.sessionStats.textContent = `${st.count} played${genres ? ' — ' + genres : ''}`;
    } catch (err) {
        els.sessionStats.style.display = 'none';
    }
}

// ── Keyboard shortcuts (ignored while typing in inputs) ───────────────────
function setupShortcuts() {
    document.addEventListener('keydown', (e) => {
        const tag = (e.target.tagName || '').toLowerCase();
        if (tag === 'input' || tag === 'select' || tag === 'textarea' || e.metaKey || e.ctrlKey || e.altKey) return;
        if (!expanded) return;
        switch (e.key.toLowerCase()) {
            case 'r':
                fetchRecommendations();
                break;
            case 'c': {
                const top = sortRecs([...state.lastRecs])[0];
                if (top) {
                    const a = top.Track.Artist || '';
                    const t = top.Track.Title || '';
                    copyToClipboard(els.recommendationsList.querySelector('.rec-item'), `${a} ${t}`.trim());
                }
                break;
            }
            case 'f':
                els.toggleFiltersBtn.click();
                break;
        }
    });
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

    refreshSessionStats();

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
        energy_mode: state.filters.energyMode,
        energy_range: state.filters.energyRange,
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

    els.filterEnergyMode.addEventListener('change', (e) => {
        state.filters.energyMode = e.target.value;
        els.energyRangeGroup.style.display = state.filters.energyMode === 'within' ? '' : 'none';
        fetchRecommendations();
    });

    els.filterEnergyRange.addEventListener('change', (e) => {
        state.filters.energyRange = parseInt(e.target.value, 10) || 1;
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

// ── Settings panel ────────────────────────────────────────────────────────
let settingsSaveTimer = null;

function setupSettingsPanel() {
    const setEls = {
        energyMode: document.getElementById('setEnergyMode'),
        energyRange: document.getElementById('setEnergyRange'),
        genreMode: document.getElementById('setGenreMode'),
        genreCount: document.getElementById('setGenreCount'),
        bpmPercent: document.getElementById('setBpmPercent'),
        bpmHalfDouble: document.getElementById('setBpmHalfDouble'),
        maxResults: document.getElementById('setMaxResults'),
        hidePlayed: document.getElementById('setHidePlayed'),
        cooldownMin: document.getElementById('setCooldownMin'),
        collapsedW: document.getElementById('setCollapsedW'),
        collapsedH: document.getElementById('setCollapsedH'),
        expandedW: document.getElementById('setExpandedW'),
        expandedH: document.getElementById('setExpandedH'),
        dimEnabled: document.getElementById('setDimEnabled'),
        dimDelay: document.getElementById('setDimDelay'),
        dimOpacity: document.getElementById('setDimOpacity'),
        hotkey: document.getElementById('setHotkey'),
        trayMode: document.getElementById('setTrayMode'),
        sessionStats: document.getElementById('setSessionStats'),
        resetWindow: document.getElementById('setResetWindow')
    };

    const tierBoxes = document.querySelectorAll('#setKeyTiers input[data-tier]');

    function fillPanel() {
        const s = state.settings;
        if (!s) return;
        setEls.energyMode.value = s.energy_mode || 'within';
        setEls.energyRange.value = String(s.energy_range || 1);
        setEls.energyRange.style.display = (s.energy_mode === 'within') ? '' : 'none';
        setEls.genreMode.value = s.genre_mode || 'any';
        setEls.genreCount.value = String(s.genre_count || 0);
        setEls.bpmPercent.value = s.bpm_percent || 8;
        setEls.bpmHalfDouble.checked = !!s.bpm_half_double;
        setEls.maxResults.value = String(s.max_results || 50);
        setEls.hidePlayed.checked = !!s.hide_played;
        setEls.cooldownMin.value = String(s.cooldown_min || 0);
        setEls.collapsedW.value = s.collapsed_width || 340;
        setEls.collapsedH.value = s.collapsed_height || 64;
        setEls.expandedW.value = s.expanded_width || 860;
        setEls.expandedH.value = s.expanded_height || 720;
        setEls.dimEnabled.checked = !!s.dim_enabled;
        setEls.dimDelay.value = s.dim_delay_sec || 8;
        setEls.dimOpacity.value = s.dim_opacity || 50;
        setEls.hotkey.value = s.hotkey || '';
        setEls.trayMode.checked = !!s.tray_mode;
        setEls.sessionStats.checked = s.session_stats !== false;
        tierBoxes.forEach(box => {
            const tier = box.dataset.tier;
            box.checked = !s.key_tiers || s.key_tiers[tier] !== false;
        });
    }

    function collectSettings() {
        const s = { ...(state.settings || {}) };
        s.energy_mode = setEls.energyMode.value;
        s.energy_range = parseInt(setEls.energyRange.value, 10) || 1;
        s.genre_mode = setEls.genreMode.value;
        s.genre_count = parseInt(setEls.genreCount.value, 10) || 0;
        s.bpm_percent = parseFloat(setEls.bpmPercent.value) || 8;
        s.bpm_half_double = setEls.bpmHalfDouble.checked;
        s.max_results = parseInt(setEls.maxResults.value, 10) || 0;
        s.hide_played = setEls.hidePlayed.checked;
        s.cooldown_min = parseInt(setEls.cooldownMin.value, 10) || 0;
        s.collapsed_width = parseInt(setEls.collapsedW.value, 10) || 340;
        s.collapsed_height = parseInt(setEls.collapsedH.value, 10) || 64;
        s.expanded_width = parseInt(setEls.expandedW.value, 10) || 860;
        s.expanded_height = parseInt(setEls.expandedH.value, 10) || 720;
        s.dim_enabled = setEls.dimEnabled.checked;
        s.dim_delay_sec = parseInt(setEls.dimDelay.value, 10) || 8;
        s.dim_opacity = parseInt(setEls.dimOpacity.value, 10) || 50;
        s.hotkey = setEls.hotkey.value.trim();
        s.tray_mode = setEls.trayMode.checked;
        s.session_stats = setEls.sessionStats.checked;
        const tiers = {};
        tierBoxes.forEach(box => { tiers[box.dataset.tier] = box.checked; });
        s.key_tiers = tiers;
        return s;
    }

    function flashSaved() {
        els.settingsSaved.textContent = 'Saved ✓';
        setTimeout(() => { els.settingsSaved.textContent = ''; }, 1500);
    }

    function scheduleSave() {
        clearTimeout(settingsSaveTimer);
        settingsSaveTimer = setTimeout(async () => {
            const payload = collectSettings();
            try {
                const res = await fetch('/api/settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                state.settings = await res.json();
                fillPanel();
                applySettingsToUI();
                resetIdleDim();
                fetchRecommendations();
                refreshSessionStats();
                flashSaved();
                // Notify Electron about hotkey/window-size changes
                if (isElectron && window.electronAPI.settingsChanged) {
                    const res = await window.electronAPI.settingsChanged(state.settings);
                    if (res && res.hotkeyOk === false && state.settings.hotkey) {
                        els.settingsSaved.textContent = 'Hotkey registration failed — try another combo';
                        els.settingsSaved.style.color = '#f87171';
                        setTimeout(() => {
                            els.settingsSaved.textContent = '';
                            els.settingsSaved.style.color = '';
                        }, 3000);
                    }
                }
            } catch (err) {
                console.error('Failed to save settings:', err);
                els.settingsSaved.textContent = 'Save failed';
            }
        }, 600);
    }

    els.settingsBtn.addEventListener('click', () => {
        fillPanel();
        els.settingsOverlay.classList.add('open');
    });
    els.settingsCloseBtn.addEventListener('click', () => {
        els.settingsOverlay.classList.remove('open');
    });
    els.settingsOverlay.addEventListener('click', (e) => {
        if (e.target === els.settingsOverlay) els.settingsOverlay.classList.remove('open');
    });

    setEls.energyMode.addEventListener('change', () => {
        setEls.energyRange.style.display = setEls.energyMode.value === 'within' ? '' : 'none';
        scheduleSave();
    });

    // Hotkey capture: focus the field, press any combo, it records the
    // accelerator. Escape/Backspace clears. Manual typing still works too.
    setEls.hotkey.addEventListener('keydown', (e) => {
        e.preventDefault();
        e.stopPropagation();

        if (e.key === 'Escape' || e.key === 'Backspace') {
            setEls.hotkey.value = '';
            scheduleSave();
            return;
        }
        if (['Shift', 'Control', 'Alt', 'Meta', 'CapsLock', 'Tab', 'Enter', 'Dead'].includes(e.key)) {
            return; // modifier alone — wait for the full combo
        }

        const key = hotkeyKeyName(e);
        if (!key) return;

        const isMac = navigator.platform.toUpperCase().includes('MAC');
        const parts = [];
        if (e.ctrlKey) parts.push(isMac ? 'Control' : 'CommandOrControl');
        if (e.metaKey) parts.push('Command');
        if (e.altKey) parts.push('Alt');
        if (e.shiftKey) parts.push('Shift');

        // Require a modifier (or an F-key) so we never hijack plain typing
        if (parts.length === 0 && !/^F([1-9]|1[0-2])$/.test(key)) {
            els.settingsSaved.textContent = 'Add a modifier (Cmd/Ctrl/Alt)';
            els.settingsSaved.style.color = '#fbbf24';
            setTimeout(() => {
                els.settingsSaved.textContent = '';
                els.settingsSaved.style.color = '';
            }, 2500);
            return;
        }

        parts.push(key);
        setEls.hotkey.value = parts.join('+');
        scheduleSave();
    });

    Object.values(setEls).forEach(el => {
        if (el === setEls.energyMode) return;
        if (el === setEls.resetWindow) return;
        el.addEventListener('change', scheduleSave);
        if (el.type === 'text' || el.type === 'number') {
            el.addEventListener('input', scheduleSave);
        }
    });
    tierBoxes.forEach(box => box.addEventListener('change', scheduleSave));

    // Reset window size to defaults (also available via right-click / Dock menu)
    setEls.resetWindow.addEventListener('click', async () => {
        if (!isElectron || !window.electronAPI.resetWindowSize) {
            els.settingsSaved.textContent = 'Electron only';
            els.settingsSaved.style.color = '#fbbf24';
            setTimeout(() => { els.settingsSaved.textContent = ''; els.settingsSaved.style.color = ''; }, 2500);
            return;
        }
        try {
            const st = await window.electronAPI.resetWindowSize();
            if (st) {
                state.settings = st;
                fillPanel();
                applySettingsToUI();
            }
            flashSaved();
        } catch (err) {
            console.error('Window reset failed:', err);
            els.settingsSaved.textContent = 'Reset failed';
            els.settingsSaved.style.color = '#f87171';
            setTimeout(() => { els.settingsSaved.textContent = ''; els.settingsSaved.style.color = ''; }, 2500);
        }
    });
}

// Map a KeyboardEvent to an Electron accelerator key name, or null if unusable.
function hotkeyKeyName(e) {
    const k = e.key;
    if (/^[a-zA-Z]$/.test(k)) return k.toUpperCase();
    if (/^[0-9]$/.test(k)) return k;
    if (k === ' ') return 'Space';
    const named = {
        ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right'
    };
    if (named[k]) return named[k];
    if (/^F([1-9]|1[0-2])$/.test(k)) return k;
    if (k === '+') return 'Plus';
    if (k.length === 1) return k.toUpperCase() === k.toLowerCase() ? k : k.toUpperCase(); // , . / ; ' [ ] etc
    return null;
}

// Start
init();
