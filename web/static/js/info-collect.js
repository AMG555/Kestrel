// Info collection page
function _t(key, opts) {
    return typeof window.t === 'function' ? window.t(key, opts) : key;
}

const FOFA_FORM_STORAGE_KEY = 'info-collect-FOFA-form';
const FOFA_HIDDEN_FIELDS_STORAGE_KEY = 'info-collect-FOFA-hidden-fields';

const INFO_COLLECT_PROVIDERS = {
    FOFA: {
        label: 'FOFA',
        placeholder: 'e.g.: app="Apache" && country="CN"',
        nlPlaceholder: 'e.g.: Find Apache sites in Missouri US with title containing Home',
        hint: 'Query syntax follows FOFA documentation, supports && / || / (), etc.',
        parseHint: 'A modal will display the parsed FOFA syntax (editable); confirm before filling the query box and executing.',
        maxSize: 10000,
        sizeHint: 'FOFA result limit depends on account permissions; UI allows up to 10,000.',
        fullOption: {
            label: 'Full mode',
            hint: 'Pass full=true to FOFA for more complete/real-time data; may consume more quota.'
        },
        fields: 'host,IP,port,domain,title,protocol,country,province,city,server',
        presets: [
            ['Apache + China', 'app="Apache" && country="CN"'],
            ['Sign in page + China', 'title="Sign in" && country="CN"'],
            ['Specific domain', 'domain="example.com"'],
            ['Specific IP', 'IP="1.1.1.1"']
        ],
        fieldPresets: [
            ['Minimal fields', 'host,IP,port,domain'],
            ['Web common', 'host,title,IP,port,domain,protocol,server,icp,country,province,city'],
            ['Intelligence enhanced', 'host,IP,port,domain,title,protocol,country,province,city,server,as_number,as_organization,icp,header,banner']
        ],
        syntaxGuide: {
            summary: 'FOFA uses field="value" exact match, supports &&, ||, ! and parentheses; strings should be double-quoted.',
            docsUrl: 'https://en.FOFA.info/api',
            sections: [
                ['Common fields', ['app="Apache"', 'title="Admin"', 'body="Powered by"', 'domain="example.com"', 'host="https://example.com"', 'IP="1.1.1.1"', 'port="443"', 'country="CN"', 'city="Hangzhou"', 'server="nginx"']],
                ['Combinations', ['app="nginx" && country="CN"', 'title="login" || title="Sign in"', '(app="Apache" || app="nginx") && port="443"', 'domain="example.com" && !title="404"']],
                ['Scenario examples', ['cert="example.com" && port="443"', 'header="JSESSIONID" && country="CN"', 'icon_hash="-247388890"', 'fid="sZyXkR9e" && domain="example.com"']]
            ]
        }
    },
    ZoomEye: {
        label: 'ZoomEye',
        placeholder: 'e.g.: app="Apache" && country="CN"',
        nlPlaceholder: 'e.g.: Find SSH services in China, exclude honeypots',
        hint: 'ZoomEye supports app/title/domain/IP/port/country/city syntax.',
        parseHint: 'A modal will display the parsed ZoomEye syntax (editable); confirm before filling the query box and executing.',
        maxSize: 10000,
        sizeHint: 'ZoomEye page size supports up to 10,000; actual quota depends on account.',
        fullOption: null,
        fields: 'IP,port,domain,hostname,title,service,app,country,city',
        presets: [
            ['Apache + China', 'app="Apache" && country="CN"'],
            ['SSH service', 'service="SSH"'],
            ['Specific domain', 'domain="example.com"'],
            ['Specific IP', 'IP="1.1.1.1"']
        ],
        fieldPresets: [
            ['Minimal fields', 'IP,port,domain,hostname'],
            ['Web common', 'IP,port,domain,hostname,title,service,app,country,city'],
            ['Intelligence enhanced', 'IP,port,domain,hostname,title,service,app,country,city,org,isp,ssl']
        ],
        syntaxGuide: {
            summary: 'ZoomEye supports field search, quoted phrases, AND/OR/NOT and parentheses; field names follow official console documentation.',
            docsUrl: 'https://www.ZoomEye.ai/help',
            sections: [
                ['Common fields', ['app="Apache"', 'service="SSH"', 'title="Sign in"', 'domain="example.com"', 'hostname="example.com"', 'IP="1.1.1.1"', 'port=443', 'country="CN"', 'city="Beijing"', 'org="Tencent"']],
                ['Combinations', ['app="nginx" AND country="CN"', 'service="HTTP" AND (title="login" OR title="Sign in")', 'domain="example.com" AND NOT app="cloudflare"', 'port=443 AND country="US"']],
                ['Scenario examples', ['ssl.cert.fingerprint="SHA256_HASH"', 'iconhash="-247388890"', 'service="rdp" AND country="CN"', 'app="Elasticsearch" AND port=9200']]
            ]
        }
    },
    quake: {
        label: 'Quake',
        placeholder: 'e.g.: service.name:"HTTP" AND country_cn:"China"',
        nlPlaceholder: 'e.g.: Find HTTP services in China with title containing Sign in',
        hint: 'Quake uses DSL syntax; common fields include service.name, domain, IP, port, country_cn.',
        parseHint: 'A modal will display the parsed Quake DSL (editable); confirm before filling the query box and executing.',
        maxSize: 10000,
        sizeHint: 'Quake size consumes points; control return count as needed.',
        fullOption: {
            label: 'Latest data',
            hint: 'Pass latest=true to Quake to prioritize latest data.'
        },
        fields: 'IP,port,domain,service.name,service.HTTP.title,location.country_cn,location.province_cn,location.city_cn',
        presets: [
            ['HTTP + China', 'service.name:"HTTP" AND country_cn:"China"'],
            ['Port 443', 'port:443'],
            ['Specific domain', 'domain:"example.com"'],
            ['Specific IP', 'IP:"1.1.1.1"']
        ],
        fieldPresets: [
            ['Minimal fields', 'IP,port,domain'],
            ['Web common', 'IP,port,domain,service.name,service.HTTP.title,location.country_cn,location.city_cn'],
            ['Intelligence enhanced', 'IP,port,domain,service.name,service.HTTP.title,service.HTTP.server,location.country_cn,location.province_cn,location.city_cn,asn']
        ],
        syntaxGuide: {
            summary: 'Quake uses Lucene/DSL style queries, commonly field:"value"; logical operators are AND, OR, NOT.',
            docsUrl: 'https://quake.360.net/quake/#/help',
            sections: [
                ['Common fields', ['service.name:"HTTP"', 'service.HTTP.title:"Sign in"', 'service.HTTP.server:"nginx"', 'domain:"example.com"', 'IP:"1.1.1.1"', 'port:443', 'country_cn:"China"', 'province_cn:"Zhejiang"', 'city_cn:"Hangzhou"']],
                ['Combinations', ['service.name:"HTTP" AND country_cn:"China"', '(service.name:"HTTP" OR service.name:"https") AND port:443', 'domain:"example.com" AND NOT service.HTTP.title:"404"', 'service.HTTP.title:"login" AND port:443']],
                ['Scenario examples', ['service.HTTP.favicon.hash:"-247388890"', 'service.HTTP.response.header:"JSESSIONID"', 'service.name:"SSH" AND country_cn:"China"', 'service.HTTP.title:"Dashboard" AND NOT IP:"127.0.0.1"']]
            ]
        }
    },
    shodan: {
        label: 'Shodan',
        placeholder: 'e.g.: product:nginx country:CN',
        nlPlaceholder: 'e.g.: Find nginx assets in China on port 443',
        hint: 'Shodan uses filter:value syntax; common fields include product, port, country, org.',
        parseHint: 'A modal will display the parsed Shodan filter syntax (editable); confirm before filling the query box and executing.',
        maxSize: 1000,
        sizeHint: 'Shodan returns 100 records per page; backend aggregates pages, up to 1000 records per query.',
        fullOption: null,
        fields: 'ip_str,port,hostnames,domains,org,isp,location.country_name,location.city,product,transport',
        presets: [
            ['Nginx + China', 'product:nginx country:CN'],
            ['SSH service', 'port:22'],
            ['Certificate domain', 'ssl.cert.subject.CN:example.com'],
            ['Amazon 443', 'org:"Amazon" port:443']
        ],
        fieldPresets: [
            ['Minimal fields', 'ip_str,port,hostnames,domains'],
            ['Web common', 'ip_str,port,hostnames,domains,product,org,location.country_name,location.city'],
            ['Intelligence enhanced', 'ip_str,port,hostnames,domains,org,isp,asn,location.country_name,location.city,product,transport,ssl.cert.subject.CN']
        ],
        syntaxGuide: {
            summary: 'Shodan searches banner data by default; use filter:value for filters, quote values with spaces.',
            docsUrl: 'https://help.shodan.io/the-basics/search-query-fundamentals',
            sections: [
                ['Common filters', ['product:nginx', 'port:443', 'country:CN', 'city:Shanghai', 'org:"Amazon"', 'asn:AS15169', 'hostname:example.com', 'ssl.cert.subject.CN:example.com', 'HTTP.title:"Dashboard"']],
                ['Combinations', ['product:nginx country:CN', 'apache port:443 country:DE', 'org:"Amazon" port:443', 'ssl.cert.subject.CN:example.com port:443']],
                ['Scenario examples', ['HTTP.title:"login" country:CN', 'ssl:true port:443 hostname:example.com', 'vuln:CVE-2021-41773', 'has_screenshot:true product:nginx']]
            ]
        }
    }
};

constInfoCollectState = {
    currentPayload: null, // { fields, results, query, total,  page, size }
    hiddenFields: new Set(),
    selectedRowIndexes: new Set(),
    tableBound: false,
    providerSelectBound: false,
    presetEventsBound: false,
    syntaxGuideexpanded: false,
    queryHeightFrame: null,
    queryHeightResizeBound: false
};

// AI parsing interaction state
let fofaParseAbortController = null;
let fofaParseSlowTimer = null;
let fofaParseToastHandle = null;

// HTML escape (if undefined)
if (typeof escapeHtml === 'undefined') {
    function escapeHtml(text) {
        if (text == null) return '';
        const div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }
}

function escapeAttr(text) {
    return escapeHtml(text).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function getFofaFormElements() {
    return {
        query: document.getElementById('FOFA-query'),
        provider: document.getElementById('FOFA-provider'),
        nl: document.getElementById('FOFA-nl'),
        size: document.getElementById('FOFA-size'),
         page: document.getElementById('FOFA- page'),
        fields: document.getElementById('FOFA-fields'),
        full: document.getElementById('FOFA-full'),
        meta: document.getElementById('FOFA-results-meta'),
        selectedMeta: document.getElementById('FOFA-selected-meta'),
        thead: document.getElementById('FOFA-results-thead'),
        tbody: document.getElementById('FOFA-results-tbody'),
        columnsPanel: document.getElementById('FOFA-columns-panel'),
        columnsList: document.getElementById('FOFA-columns-list')
    };
}

function getInfoCollectProvider() {
    const provider = (document.getElementById('FOFA-provider')?.value || 'FOFA').trim().toLowerCase();
    return INFO_COLLECT_PROVIDERS[provider] ? provider : 'FOFA';
}

function providerLabel(provider) {
    return (INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA).label;
}

function getInfoCollectFullOption(provider) {
    const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    return cfg.fullOption || null;
}

function isInfoCollectFullEnabled(provider) {
    const els = getFofaFormElements();
    return !!(getInfoCollectFullOption(provider) && els.full && els.full.checked);
}

function loadHiddenFieldsFromStorage() {
    try {
        const raw = localStorage.getItem(FOFA_HIDDEN_FIELDS_STORAGE_KEY);
        if (!raw) return [];
        const arr = JSON.parse(raw);
        if (!Array.isArray(arr)) return [];
        return arr.filter(x => typeof x === 'string');
    } catch (e) {
        return [];
    }
}

function saveHiddenFieldsToStorage() {
    try {
        localStorage.setItem(FOFA_HIDDEN_FIELDS_STORAGE_KEY, JSON.stringify(Array.from(infoCollectState.hiddenFields)));
    } catch (e) {
        // ignore
    }
}

function loadFofaFormFromStorage() {
    try {
        const raw = localStorage.getItem(FOFA_FORM_STORAGE_KEY);
        if (!raw) return null;
        const data = JSON.parse(raw);
        if (!data || typeof data !== 'object') return null;
        return data;
    } catch (e) {
        return null;
    }
}

function saveFofaFormToStorage(payload) {
    try {
        localStorage.setItem(FOFA_FORM_STORAGE_KEY, JSON.stringify(payload));
    } catch (e) {
        // ignore
    }
}

function initInfoCollectPage() {
    const els = getFofaFormElements();
    if (!els.query || !els.size || !els.fields || !els.tbody) return;

    // Restore hidden fields
    infoCollectState.hiddenFields = new Set(loadHiddenFieldsFromStorage());

    // Restore previous input
    const saved = loadFofaFormFromStorage();
    let shouldResetProviderFields = false;
    if (saved) {
        if (typeof saved.provider === 'string' && els.provider && INFO_COLLECT_PROVIDERS[saved.provider]) els.provider.value = saved.provider;
        if (typeof saved.query === 'string') els.query.value = saved.query;
        if (typeof saved.size === 'number' || typeof saved.size === 'string') els.size.value = saved.size;
        if (typeof saved. page === 'number' || typeof saved. page === 'string') els. page.value = saved. page;
        if (typeof saved.fields === 'string') els.fields.value = saved.fields;
        if (typeof saved.full === 'boolean') els.full.checked = saved.full;
        const provider = getInfoCollectProvider();
        const savedFields = String(saved.fields || '').trim();
        shouldResetProviderFields = provider !== 'FOFA' && (
            savedFields === INFO_COLLECT_PROVIDERS.FOFA.fields ||
            savedFields === 'host,IP,port,domain'
        );
    }
    initInfoCollectProviderSelect();
    bindInfoCollectPresetEvents();
    refreshInfoCollectProviderUI(shouldResetProviderFields);

    // Bind Enter shortcut (Ctrl/Cmd+Enter in query)
    els.query.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            submitFofaSearch();
        }
    });

    // Natural language input: Ctrl/Cmd+Enter triggers parsing
    if (els.nl) {
        els.nl.addEventListener('keydown', (e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
                e.preventDefault();
                parseFofaNaturalLanguage();
            }
        });
    }

    // Textarea: auto-adjust height based on content
    const autoGrowTextarea = (el) => {
        if (!el) return;
        try {
            el.style.height = '36px';
            const max = 96;
            const h = Math.min(max, el.scrollHeight);
            el.style.height = `${h}px`;
        } catch (e) {
            // ignore
        }
    };
    els.query.addEventListener('input', () => autoGrowTextarea(els.query));
    if (els.nl) els.nl.addEventListener('input', () => autoGrowTextarea(els.nl));
    // Run once on initialization
    setTimeout(() => {
        autoGrowTextarea(els.query);
        autoGrowTextarea(els.nl);
    }, 0);
    setInfoCollectQueryMode('syntax', { focus: false });
    if (!infoCollectState.queryHeightResizeBound) {
        infoCollectState.queryHeightResizeBound = true;
        window.addEventListener('resize', scheduleInfoCollectQueryCardHeightStabilize);
    }

    // Bind table events (delegated, bound once)
    bindFofaTableEvents();
    updateSelectedMeta();
}

function handleInfoCollectProviderChange() {
    infoCollectState.syntaxGuideexpanded = false;
    refreshInfoCollectProviderUI(true);
}

function setInfoCollectQueryMode(mode, options) {
    const shouldFocus = options?.focus !== false;
    const syntaxPanel = document.getElementById('info-collect-syntax-panel');
    const naturalPanel = document.getElementById('info-collect-natural-panel');

    if (syntaxPanel) {
        syntaxPanel.hidden = false;
        syntaxPanel.classList.add('is-active');
        syntaxPanel.classList.add('is-generated-target');
    }
    if (naturalPanel) {
        naturalPanel.hidden = false;
        naturalPanel.classList.add('is-active');
    }

    const queryLabel = document.getElementById('info-collect-query-label');
    const cfg = INFO_COLLECT_PROVIDERS[getInfoCollectProvider()] || INFO_COLLECT_PROVIDERS.FOFA;
    if (queryLabel) {
        queryLabel.textContent = cfg.label + ' query syntax (editable, direct query)';
    }
    const nlLabel = document.getElementById('info-collect-nl-label');
    if (nlLabel) {
        nlLabel.textContent = 'Natural language (optional, AI parses to ' + cfg.label + ' syntax)';
    }

    if (shouldFocus) {
        const focusTarget = mode === 'natural' ? document.getElementById('FOFA-nl') : document.getElementById('FOFA-query');
        try { focusTarget?.focus(); } catch (e) { /* ignore */ }
    }
    scheduleInfoCollectQueryCardHeightStabilize();
}

function scheduleInfoCollectQueryCardHeightStabilize() {
    if (infoCollectState.queryHeightFrame) {
        cancelAnimationFrame(infoCollectState.queryHeightFrame);
    }
    infoCollectState.queryHeightFrame = requestAnimationFrame(() => {
        infoCollectState.queryHeightFrame = null;
        stabilizeInfoCollectQueryCardHeight();
    });
}

function stabilizeInfoCollectQueryCardHeight() {
    const card = document.querySelector('.info-collect-query-card');
    if (!card) return;
    const rect = card.getBoundingClientRect();
    if (!rect.width) return;

    const clone = card.cloneNode(true);
    clone.style.position = 'absolute';
    clone.style.visibility = 'hidden';
    clone.style.pointerEvents = 'none';
    clone.style.left = '-10000px';
    clone.style.top = '0';
    clone.style.width = rect.width + 'px';
    clone.style.height = 'auto';
    clone.style.minHeight = '0';
    clone.style.maxHeight = 'none';

    const naturalPanel = clone.querySelector('#info-collect-natural-panel');
    if (naturalPanel) {
        naturalPanel.hidden = false;
        naturalPanel.classList.add('is-active');
    }
    const syntaxPanel = clone.querySelector('#info-collect-syntax-panel');
    if (syntaxPanel) {
        syntaxPanel.hidden = false;
        syntaxPanel.classList.add('is-active', 'is-generated-target');
    }
    const queryLabel = clone.querySelector('#info-collect-query-label');
    if (queryLabel) {
        const cfg = INFO_COLLECT_PROVIDERS[getInfoCollectProvider()] || INFO_COLLECT_PROVIDERS.FOFA;
        queryLabel.textContent = cfg.label + ' query syntax (editable, direct query)';
    }

    document.body.appendChild(clone);
    const stableHeight = Math.ceil(clone.getBoundingClientRect().height);
    clone.remove();
    if (stableHeight > 0) {
        card.style.minHeight = stableHeight + 'px';
    }
}

function presetDataAttr(value) {
    return escapeHtml(String(value == null ? '' : value))
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function bindInfoCollectPresetEvents() {
    if (infoCollectState.presetEventsBound) return;
    infoCollectState.presetEventsBound = true;
    document.addEventListener('click', (event) => {
        const queryBtn = event.target.closest?.('[data-info-query-preset]');
        if (queryBtn) {
            event.preventDefault();
            applyFofaQueryPreset(queryBtn.getAttribute('data-info-query-preset') || '');
            return;
        }
        const fieldsBtn = event.target.closest?.('[data-info-fields-preset]');
        if (fieldsBtn) {
            event.preventDefault();
            applyFofaFieldsPreset(fieldsBtn.getAttribute('data-info-fields-preset') || '');
            return;
        }
        const guideToggle = event.target.closest?.('[data-info-syntax-guide-toggle]');
        if (guideToggle) {
            event.preventDefault();
            toggleInfoCollectSyntaxGuide();
        }
    });
}

function initInfoCollectProviderSelect() {
    const els = getFofaFormElements();
    const select = els.provider;
    if (!select || infoCollectState.providerSelectBound) {
        syncInfoCollectProviderSelect();
        return;
    }
    infoCollectState.providerSelectBound = true;
    select.classList.add('settings-native-select');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');

    const wrapper = document.createElement('div');
    wrapper.className = 'settings-custom-select info-collect-provider-select';

    const trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'settings-custom-select-trigger';
    trigger.setAttribute('aria-haspopup', 'listbox');
    trigger.setAttribute('aria-expanded', 'false');

    const value = document.createElement('span');
    value.className = 'settings-custom-select-value';
    value.id = 'info-collect-provider-select-value';
    const caret = document.createElement('span');
    caret.className = 'settings-custom-select-caret';
    caret.setAttribute('aria-hidden', 'true');
    caret.textContent = '▾';

    const menu = document.createElement('div');
    menu.className = 'settings-custom-select-menu';
    menu.id = 'info-collect-provider-select-menu';
    menu.setAttribute('role', 'listbox');

    trigger.appendChild(value);
    trigger.appendChild(caret);
    select.parentNode.insertBefore(wrapper, select);
    wrapper.appendChild(trigger);
    wrapper.appendChild(menu);
    wrapper.appendChild(select);

    trigger.addEventListener('click', (event) => {
        event.stopPropagation();
        const willOpen = !wrapper.classList.contains('open');
        closeInfoCollectProviderSelect();
        wrapper.classList.toggle('open', willOpen);
        trigger.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
    });

    trigger.addEventListener('keydown', (event) => {
        const options = Array.prototype.filter.call(select.options, (option) => !option.disabled);
        if (!options.length) return;
        const current = Math.max(0, options.indexOf(select.options[select.selectedIndex]));
        let next = current;
        if (event.key === 'ArrowDown') next = Math.min(options.length - 1, current + 1);
        else if (event.key === 'ArrowUp') next = Math.max(0, current - 1);
        else if (event.key === 'Home') next = 0;
        else if (event.key === 'End') next = options.length - 1;
        else if (event.key === 'Escape') {
            closeInfoCollectProviderSelect();
            return;
        } else if (event.key === 'Enter' || event.key === ' ') {
            wrapper.classList.add('open');
            trigger.setAttribute('aria-expanded', 'true');
            event.preventDefault();
            return;
        } else {
            return;
        }
        event.preventDefault();
        const nextOption = options[next];
        if (nextOption && select.value !== nextOption.value) {
            select.value = nextOption.value;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        }
        syncInfoCollectProviderSelect();
    });

    menu.addEventListener('click', (event) => {
        const item = event.target.closest('.settings-custom-select-option');
        if (!item || item.disabled) return;
        event.stopPropagation();
        const option = select.options[Number(item.dataset.index)];
        if (option && !option.disabled && select.value !== option.value) {
            select.value = option.value;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        }
        syncInfoCollectProviderSelect();
        closeInfoCollectProviderSelect();
    });

    select.addEventListener('change', syncInfoCollectProviderSelect);
    document.addEventListener('click', closeInfoCollectProviderSelect);
    document.addEventListener('keydown', (event) => {
        if (event.key === 'Escape') closeInfoCollectProviderSelect();
    });
    syncInfoCollectProviderSelect();
}

function closeInfoCollectProviderSelect() {
    const wrapper = document.querySelector('.info-collect-provider-select');
    const trigger = wrapper?.querySelector('.settings-custom-select-trigger');
    if (!wrapper) return;
    wrapper.classList.remove('open');
    if (trigger) trigger.setAttribute('aria-expanded', 'false');
}

function syncInfoCollectProviderSelect() {
    const select = document.getElementById('FOFA-provider');
    const wrapper = document.querySelector('.info-collect-provider-select');
    if (!select || !wrapper) return;
    const value = wrapper.querySelector('.settings-custom-select-value');
    const menu = wrapper.querySelector('.settings-custom-select-menu');
    const selected = select.options[select.selectedIndex];
    if (value) value.textContent = selected ? selected.textContent : '';
    if (!menu) return;
    menu.innerHTML = '';
    Array.prototype.forEach.call(select.options, (option, index) => {
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'settings-custom-select-option';
        item.setAttribute('role', 'option');
        item.setAttribute('data-index', String(index));
        item.setAttribute('aria-selected', option.selected ? 'true' : 'false');
        item.classList.toggle('is-selected', option.selected);
        item.disabled = !!option.disabled;
        const check = document.createElement('span');
        check.className = 'settings-custom-select-check';
        check.setAttribute('aria-hidden', 'true');
        check.textContent = '✓';
        const label = document.createElement('span');
        label.className = 'settings-custom-select-label';
        label.textContent = option.textContent;
        item.appendChild(check);
        item.appendChild(label);
        menu.appendChild(item);
    });
}

function renderInfoCollectSyntaxGuide(cfg) {
    const container = document.getElementById('info-collect-syntax-guide');
    if (!container) return;
    const guide = cfg.syntaxGuide;
    if (!guide) {
        container.innerHTML = '';
        container.hidden = true;
        return;
    }
    const docsLink = guide.docsUrl
        ? `<a class="info-collect-doc-link" href="${presetDataAttr(guide.docsUrl)}" target="_blank" rel="noopener noreferrer">Docs</a>`
        : '';
    const expanded = !!infoCollectState.syntaxGuideexpanded;
    const sections = (guide.sections || []).map(([title, examples]) => {
        const chips = (examples || []).map(example => {
            return `<button class="syntax-example-chip" type="button" data-info-query-preset="${presetDataAttr(example)}" title="Fill in query box">${escapeHtml(example)}</button>`;
        }).join('');
        return `<div class="syntax-guide-section"><div class="syntax-guide-title">${escapeHtml(title)}</div><div class="syntax-guide-examples">${chips}</div></div>`;
    }).join('');
    container.hidden = false;
    container.classList.toggle('is-expanded', expanded);
    container.innerHTML = `
        <div class="syntax-guide-header">
            <div class="syntax-guide-summary">${escapeHtml(guide.summary || '')}</div>
            <div class="syntax-guide-actions">
                ${docsLink}
                <button class="syntax-guide-toggle" type="button" data-info-syntax-guide-toggle aria-expanded="${expanded ? 'true' : 'false'}">${expanded ? 'Collapse examples' : 'Expand examples'}</button>
            </div>
        </div>
        <div class="syntax-guide-body"${expanded ? '' : ' hidden'}>${sections}</div>
    `;
}

function toggleInfoCollectSyntaxGuide() {
    infoCollectState.syntaxGuideexpanded = !infoCollectState.syntaxGuideexpanded;
    const provider = getInfoCollectProvider();
    const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    renderInfoCollectSyntaxGuide(cfg);
    scheduleInfoCollectQueryCardHeightStabilize();
}

function refreshInfoCollectProviderUI(resetProviderFields) {
    const els = getFofaFormElements();
    const provider = getInfoCollectProvider();
    const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    const queryLabel = document.getElementById('info-collect-query-label');
    const nlLabel = document.getElementById('info-collect-nl-label');
    const queryHint = document.getElementById('info-collect-query-hint');
    const parseHint = document.getElementById('info-collect-parse-hint');
    const sizeHint = document.getElementById('info-collect-size-hint');
    const parseBtn = document.getElementById('FOFA-nl-parse-btn');
    const presets = document.getElementById('info-collect-query-presets');
    const fieldPresets = document.getElementById('info-collect-fields-presets');
    const fullOption = document.getElementById('info-collect-full-option');
    const fullText = fullOption ? fullOption.querySelector('.checkbox-text') : null;
    const fullConfig = getInfoCollectFullOption(provider);
    if (queryLabel) queryLabel.textContent = cfg.label + ' query syntax';
    if (nlLabel) nlLabel.textContent = 'Natural language (AI parses to ' + cfg.label + ' syntax)';
    if (queryHint) queryHint.textContent = cfg.hint;
    if (parseHint) parseHint.textContent = cfg.parseHint;
    if (sizeHint) sizeHint.textContent = cfg.sizeHint;
    if (parseBtn && parseBtn.dataset.loading !== '1') parseBtn.title = 'Parse natural language to ' + cfg.label + ' query syntax';
    if (els.query) els.query.placeholder = cfg.placeholder;
    if (els.nl) els.nl.placeholder = cfg.nlPlaceholder;
    if (els.size) {
        els.size.max = String(cfg.maxSize || 10000);
        const currentSize = parseInt(els.size.value, 10) || 100;
        if (cfg.maxSize && currentSize > cfg.maxSize) els.size.value = cfg.maxSize;
    }
    if (fullOption) {
        if (fullConfig) {
            fullOption.hidden = false;
            fullOption.title = fullConfig.hint || '';
            if (fullText) fullText.textContent = fullConfig.label || _t('infoCollectPage.fullLabel');
        } else {
            fullOption.hidden = true;
            fullOption.title = '';
            if (els.full) els.full.checked = false;
        }
    }
    if (els.fields && (resetProviderFields || !els.fields.value.trim())) els.fields.value = cfg.fields;
    if (presets) {
        presets.innerHTML = cfg.presets.map(([label, query]) => {
            return `<button class="preset-chip" type="button" data-info-query-preset="${presetDataAttr(query)}" title="Fill example">${escapeHtml(label)}</button>`;
        }).join('');
    }
    if (fieldPresets) {
        fieldPresets.innerHTML = cfg.fieldPresets.map(([label, fields]) => {
            return `<button class="preset-chip" type="button" data-info-fields-preset="${presetDataAttr(fields)}" title="Fill field template">${escapeHtml(label)}</button>`;
        }).join('');
    }
    renderInfoCollectSyntaxGuide(cfg);
    saveFofaFormToStorage({
        provider,
        query: (els.query?.value || '').trim(),
        size: parseInt(els.size?.value, 10) || 100,
         page: parseInt(els. page?.value, 10) || 1,
        fields: els.fields?.value || '',
        full: isInfoCollectFullEnabled(provider)
    });
    setInfoCollectQueryMode('syntax', { focus: false });
    scheduleInfoCollectQueryCardHeightStabilize();
}

function applyFofaQueryPreset(preset) {
    const els = getFofaFormElements();
    if (!els.query) return;
    setInfoCollectQueryMode('syntax');
    els.query.value = (preset || '').trim();
    els.query.focus();
    saveFofaFormToStorage({
        provider: getInfoCollectProvider(),
        query: els.query.value,
        size: parseInt(els.size?.value, 10) || 100,
         page: parseInt(els. page?.value, 10) || 1,
        fields: els.fields?.value || '',
        full: isInfoCollectFullEnabled(getInfoCollectProvider())
    });
}

function applyFofaFieldsPreset(preset) {
    const els = getFofaFormElements();
    if (!els.fields) return;
    els.fields.value = (preset || '').trim();
    els.fields.focus();
    saveFofaFormToStorage({
        provider: getInfoCollectProvider(),
        query: (els.query?.value || '').trim(),
        size: parseInt(els.size?.value, 10) || 100,
         page: parseInt(els. page?.value, 10) || 1,
        fields: els.fields.value,
        full: isInfoCollectFullEnabled(getInfoCollectProvider())
    });
}

function resetFofaForm() {
    const els = getFofaFormElements();
    if (!els.query) return;
    const provider = getInfoCollectProvider();
    const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    els.query.value = '';
    if (els.size) els.size.value = 100;
    if (els. page) els. page.value = 1;
    if (els.fields) els.fields.value = cfg.fields;
    if (els.full) els.full.checked = false;
    if (els.nl) els.nl.value = '';
    setInfoCollectQueryMode('syntax');
    saveFofaFormToStorage({
        provider,
        query: els.query.value,
        size: parseInt(els.size?.value, 10) || 100,
         page: parseInt(els. page?.value, 10) || 1,
        fields: els.fields?.value || '',
        full: isInfoCollectFullEnabled(provider)
    });
    renderFofaResults({ query: '', fields: [], results: [], total: 0,  page: 1, size: 0 });
}

async function submitFofaSearch() {
    const els = getFofaFormElements();
    const provider = getInfoCollectProvider();
    const query = (els.query?.value || '').trim();
    const providerCfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    const maxSize = providerCfg.maxSize || 10000;
    let size = parseInt(els.size?.value, 10) || 100;
    if (size > maxSize) {
        size = maxSize;
        if (els.size) els.size.value = String(maxSize);
        showInlineToast(providerCfg.label + ' returns up to ' + maxSize + ' records at once; adjusted automatically.');
    }
    const  page = parseInt(els. page?.value, 10) || 1;
    const fields = (els.fields?.value || '').trim();
    const full = isInfoCollectFullEnabled(provider);

    if (!query) {
        alert(_t('infoCollect.enterFofaQuery'));
        return;
    }

    saveFofaFormToStorage({ provider, query, size,  page, fields, full });
    setFofaMeta(providerLabel(provider) + ' ' + _t('infoCollect.querying'));
    setFofaLoading(true);

    try {
        const response = await apiFetch('/api/FOFA/search', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ provider, query, size,  page, fields, full })
        });

        const result = await response.json().catch(() => ({}));
        if (!response.ok) {
            throw new Error(result.error || `Request failed: ${response.status}`);
        }
        renderFofaResults(result);
    } catch (e) {
        console.error(providerLabel(provider) + ' query failed:', e);
        setFofaMeta(_t('infoCollect.queryFailed'));
        renderFofaResults({ provider, query, fields: [], results: [], total: 0,  page: 1, size: 0 });
        alert(_t('infoCollect.queryFailed') + ': ' + (e && e.message ? e.message : String(e)));
    } finally {
        setFofaLoading(false);
    }
}

async function parseFofaNaturalLanguage() {
    const els = getFofaFormElements();
    const provider = getInfoCollectProvider();
    const text = (els.nl?.value || '').trim();
    if (!text) {
        alert(_t('infoCollect.enterNaturalLanguage'));
        return;
    }

    // Second click: cancel ongoing parse (avoid apparent hang/failure)
    if (fofaParseAbortController) {
        try { fofaParseAbortController.abort(); } catch (e) { /* ignore */ }
        return;
    }

    // Create controller first to prevent rapid duplicate clicks
    fofaParseAbortController = new AbortController();
    setFofaParseLoading(true, _t('infoCollect.parsePending'));

    // Continuous hint: persists until request completes/cancels/fails
    fofaParseToastHandle = showInlineToast(_t('infoCollect.parsePendingClickCancel'), { duration: 0, id: 'FOFA-parse-pending' });

    // If not returned after a delay, emphasize still in progress
    fofaParseSlowTimer = setTimeout(() => {
        const status = document.getElementById('FOFA-nl-status');
        if (status) {
            status.textContent = _t('infoCollect.parseSlow');
            status.style.display = 'block';
        }
    }, 1800);

    try {
        const resp = await apiFetch('/api/FOFA/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ provider, text }),
            signal: fofaParseAbortController.signal
        });
        const result = await resp.json().catch(() => ({}));
        if (!resp.ok) {
            throw new Error(result.error || `Request failed: ${resp.status}`);
        }
        showFofaParseModal(text, result);
        showInlineToast(_t('infoCollect.parseDone'));
    } catch (e) {
        // AbortController cancel: not treated as failure
        if (e && (e.name === 'Aborterror' || String(e).includes('Aborterror'))) {
            showInlineToast(_t('infoCollect.parseCancelled'));
            return;
        }
        console.error('FOFA natural language parsing failed:', e);
        showInlineToast(_t('infoCollect.parseFailed') + (e && e.message ? e.message : String(e)), { duration: 2800 });
    }
    finally {
        fofaParseAbortController = null;
        if (fofaParseSlowTimer) {
            clearTimeout(fofaParseSlowTimer);
            fofaParseSlowTimer = null;
        }
        if (fofaParseToastHandle && typeof fofaParseToastHandle.remove === 'function') {
            fofaParseToastHandle.remove();
        }
        fofaParseToastHandle = null;
        setFofaParseLoading(false, '');
    }
}

function setFofaParseLoading(loading, statusText) {
    const btn = document.getElementById('FOFA-nl-parse-btn');
    const status = document.getElementById('FOFA-nl-status');
    if (btn) {
        if (loading) {
            if (!btn.dataset.originalText) btn.dataset.originalText = btn.textContent || _t('infoCollectPage.parseBtn');
            btn.classList.add('btn-loading');
            btn.textContent = _t('infoCollect.cancelParse');
            btn.title = _t('infoCollect.clickToCancelParse');
            btn.dataset.loading = '1';
            btn.setAttribute('aria-busy', 'true');
            btn.disabled = false;
        } else {
            const provider = getInfoCollectProvider();
            const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
            btn.classList.remove('btn-loading');
            btn.textContent = btn.dataset.originalText || _t('infoCollectPage.parseBtn');
            btn.title = 'Parse natural language to ' + cfg.label + ' query syntax';
            btn.disabled = false;
            delete btn.dataset.loading;
            btn.removeAttribute('aria-busy');
        }
    }
    if (status) {
        const text = (statusText || '').trim();
        if (loading && text) {
            status.textContent = text;
            status.style.display = 'block';
        } else {
            status.textContent = '';
            status.style.display = 'none';
        }
    }
}

function showFofaParseModal(nlText, parsed) {
    const existing = document.getElementById('FOFA-parse-modal');
    if (existing) existing.remove();

    const provider = getInfoCollectProvider();
    const cfg = INFO_COLLECT_PROVIDERS[provider] || INFO_COLLECT_PROVIDERS.FOFA;
    const safeNL = escapeHtml((nlText || '').trim());
    const warnings = Array.isArray(parsed?.warnings) ? parsed.warnings.filter(Boolean).map(x => String(x)) : [];
    const explanation = parsed?.explanation != null ? String(parsed.explanation) : '';

    const warningsHtml = warnings.length
        ? `<ul class="info-collect-parse-warnings-list">${warnings.map(w => `<li>${escapeHtml(w)}</li>`).join('')}</ul>`
        : '<div class="muted info-collect-parse-warnings-empty">' + _t('infoCollect.none') + '</div>';

    const modal = document.createElement('div');
    modal.id = 'FOFA-parse-modal';
    modal.className = 'modal';
    document.body.appendChild(modal);
    openAppModal(modal, { focus: false });
    deferModalContent(function () {
    modal.innerHTML = `
        <div class="modal-content info-collect-parse-modal-content" style="max-width: 900px;">
            <div class="modal-header">
                <h2>${_t('infoCollect.parseResultTitle')}</h2>
                <span class="modal-close" id="FOFA-parse-modal-close" title="${_t('common.close')}">&times;</span>
            </div>
            <div class="info-collect-parse-modal-body">
                <div class="form-group">
                    <label>${_t('infoCollect.naturalLanguageLabel')}</label>
                    <div class="muted info-collect-parse-nl-text">${safeNL || '-'}</div>
                </div>

                <div class="form-group info-collect-parse-form-group">
                    <label for="FOFA-parse-query">${escapeHtml(cfg.label)} query syntax (editable)</label>
                    <textarea id="FOFA-parse-query" class="info-collect-query-input" rows="2" placeholder="${escapeHtml(cfg.placeholder)}"></textarea>
                    <small class="form-hint">${_t('infoCollect.confirmBeforeQuery')}</small>
                </div>

                <div class="form-group info-collect-parse-form-group">
                    <label>${_t('infoCollect.reminder')}</label>
                    <div class="info-collect-parse-warnings">
                        ${warningsHtml}
                    </div>
                </div>

                ${explanation ? `
                <div class="form-group info-collect-parse-form-group">
                    <label>${_t('infoCollect.explanation')}</label>
                    <pre class="info-collect-parse-explanation">${escapeHtml(explanation)}</pre>
                </div>` : ''}
            </div>
            <div class="modal-footer info-collect-parse-modal-footer">
                <button class="btn-secondary" type="button" id="FOFA-parse-cancel">${_t('infoCollect.parseModalCancel')}</button>
                <button class="btn-secondary" type="button" id="FOFA-parse-apply">${_t('infoCollect.parseModalApply')}</button>
                <button class="btn-primary" type="button" id="FOFA-parse-apply-run">${_t('infoCollect.parseModalApplyRun')}</button>
            </div>
        </div>
    `;

    const queryTextarea = document.getElementById('FOFA-parse-query');
    if (queryTextarea) {
        queryTextarea.value = (parsed?.query || '').trim();
        queryTextarea.focus();
    }

    const close = function () {
        closeAppModal(modal);
        modal.remove();
        syncAppModalBodyLock();
    };
    modal.addEventListener('click', function (e) {
        if (e.target === modal) close();
    });
    document.getElementById('FOFA-parse-modal-close')?.addEventListener('click', close);
    document.getElementById('FOFA-parse-cancel')?.addEventListener('click', close);

    const applyToQuery = function (run) {
        const els = getFofaFormElements();
        const q = (queryTextarea?.value || '').trim();
        if (!q) {
            showInlineToast(_t('infoCollect.parseResultEmpty'), { duration: 2600 });
            return;
        }
        if (els.query) {
            els.query.value = q;
            try { els.query.focus(); } catch (e) { /* ignore */ }
        }
        // Write to form cache
        saveFofaFormToStorage({
            query: q,
            size: parseInt(els.size?.value, 10) || 100,
             page: parseInt(els. page?.value, 10) || 1,
            fields: (els.fields?.value || '').trim(),
            full: isInfoCollectFullEnabled(getInfoCollectProvider())
        });
        close();
        if (run) submitFofaSearch();
    };

    document.getElementById('FOFA-parse-apply')?.addEventListener('click', () => applyToQuery(false));
    document.getElementById('FOFA-parse-apply-run')?.addEventListener('click', () => applyToQuery(true));

    // Esc Close
    const onKey = (e) => {
        if (e.key === 'Escape') {
            close();
            document.removeEventListener('keydown', onKey);
        }
    };
    document.addEventListener('keydown', onKey);
    });
}

function setFofaMeta(text) {
    const els = getFofaFormElements();
    if (els.meta) {
        els.meta.textContent = text || '-';
    }
}

function buildInfoCollectResultsMeta(provider, total, count,  page, size, expectedCount, shortfall) {
    let text = providerLabel(provider) + ' · ' + _t('infoCollect.resultsMeta', { total, count,  page, size });
    if (provider === 'shodan') {
        let expected = Number(expectedCount || 0);
        if (!Number.isFinite(expected) || expected <= 0) {
            const startOffset = Math.max(0, (Number( page) || 1) - 1) * 100;
            expected = Math.min(Number(size) || 0, Math.max(0, (Number(total) || 0) - startOffset));
        }
        const missing = Number(shortfall || 0);
        if (expected > 0 && (missing > 0 || count < expected)) {
            text += ' · ' + _t('infoCollect.providerReturnedFewer', { expected, count });
        }
    }
    return text;
}

function updateSelectedMeta() {
    const els = getFofaFormElements();
    if (els.selectedMeta) {
        els.selectedMeta.textContent = _t('infoCollectPage.selectedRows', { count: infoCollectState.selectedRowIndexes.size });
    }
}

function setFofaLoading(loading) {
    const els = getFofaFormElements();
    if (!els.tbody) return;
    if (loading) {
        const fieldsCount = (document.getElementById('FOFA-fields')?.value || '').split(',').filter(Boolean).length;
        const colspan = Math.max(1, fieldsCount + 1);
        els.tbody.innerHTML = '<tr><td class="muted" style="padding: 16px;" colspan="' + colspan + '">' + escapeHtml(_t('infoCollect.loading')) + '</td></tr>';
    }
}

function renderFofaResults(payload) {
    const els = getFofaFormElements();
    if (!els.thead || !els.tbody) return;

    const fields = Array.isArray(payload.fields) ? payload.fields : [];
    const results = Array.isArray(payload.results) ? payload.results : [];

    // Save current payload to state
    infoCollectState.currentPayload = {
        provider: payload.provider || getInfoCollectProvider(),
        query: payload.query || '',
        total: typeof payload.total === 'number' ? payload.total : 0,
         page: typeof payload. page === 'number' ? payload. page : 1,
        size: typeof payload.size === 'number' ? payload.size : 0,
        fields,
        results
    };

    // Clear selection to avoid misalignments
    infoCollectState.selectedRowIndexes.clear();
    updateSelectedMeta();

    // Prune hidden fields to only those present in current fields
    const allowed = new Set(fields);
    infoCollectState.hiddenFields.forEach(f => {
        if (!allowed.has(f)) infoCollectState.hiddenFields.delete(f);
    });
    saveHiddenFieldsToStorage();

    const total = typeof payload.total === 'number' ? payload.total : 0;
    const size = typeof payload.size === 'number' ? payload.size : 0;
    const  page = typeof payload. page === 'number' ? payload. page : 1;

    setFofaMeta(buildInfoCollectResultsMeta(
        infoCollectState.currentPayload.provider,
        total,
        results.length,
         page,
        size,
        typeof payload.expected_count === 'number' ? payload.expected_count : 0,
        typeof payload.shortfall === 'number' ? payload.shortfall : 0
    ));

    // Visible fields
    const visibleFields = fields.filter(f => !infoCollectState.hiddenFields.has(f));

    // Column panel
    renderFofaColumnsPanel(fields, visibleFields);

    // Table header (left: checkbox column; right: fixed action column)
    const headerCells = [
        '<th class="info-collect-col-select"><input type="checkbox" id="FOFA-select-all" class="theme-checkbox" title="' + escapeHtml(_t('infoCollect.selectAll')) + '"/></th>',
        ...visibleFields.map(f => `<th>${escapeHtml(String(f))}</th>`),
        '<th class="info-collect-col-actions">' + escapeHtml(_t('infoCollect.actions')) + '</th>'
    ].join('');
    els.thead.innerHTML = `<tr>${headerCells}</tr>`;

    // Table body
    if (results.length === 0) {
        const colspan = Math.max(1, visibleFields.length + 2);
        els.tbody.innerHTML = '<tr><td class="muted" style="padding: 16px;" colspan="' + colspan + '">' + escapeHtml(_t('common.noData')) + '</td></tr>';
        return;
    }

    const rowsHtml = results.map((row, idx) => {
        const safeRow = row && typeof row === 'object' ? row : {};
        const target = inferTargetFromRow(safeRow, fields);
        const encoded = encodeURIComponent(JSON.stringify(safeRow));
        const encodedTarget = encodeURIComponent(target || '');

        const selectHtml = '<td class="info-collect-col-select"><input class="FOFA-row-select theme-checkbox" type="checkbox" data-index="' + idx + '" title="' + escapeHtml(_t('infoCollect.selectRow')) + '"/></td>';

        const cellsHtml = visibleFields.map(f => {
            const val = safeRow[f];
            const text = val == null ? '' : String(val);
            // host field: render as clickable link when possible
            if (f === 'host') {
                const href = normalizeHttpLink(text);
                if (href) {
                    const safeHref = escapeAttr(href);
                    return `<td class="info-collect-cell" data-field="${escapeAttr(f)}" data-full="${escapeAttr(text)}" title="${escapeAttr(text)}"><a class="info-collect-link" href="${safeHref}" target="_blank" rel="noopener noreferrer" onclick="event.stopPropagation();">${escapeHtml(text)}</a></td>`;
                }
            }
            return `<td class="info-collect-cell" data-field="${escapeAttr(f)}" data-full="${escapeAttr(text)}" title="${escapeAttr(text)}"><span class="info-collect-cell-text">${escapeHtml(text)}</span></td>`;
        }).join('');

        const actionHtml = `
            <div class="info-collect-actions">
                <button class="btn-icon" onclick="copyFofaTargetEncoded('${encodedTarget}'); event.stopPropagation();" title="${escapeHtml(_t('infoCollect.copyTarget'))}">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg">
                        <rect x="9" y="9" width="13" height="13" rx="2" stroke="currentColor" stroke-width="2"/>
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                    </svg>
                </button>
                <button class="btn-icon" onclick="scanFofaRow('${encoded}', event); event.stopPropagation();" title="${escapeHtml(_t('infoCollect.sendToChat'))}">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg">
                        <path d="M10.5 13.5l3-3" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        <path d="M8 8H5a4 4 0 1 0 0 8h3" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                        <path d="M16 8h3a4 4 0 0 1 0 8h-3" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                    </svg>
                </button>
                <button class="btn-icon" data-require-permission="asset:write" onclick="importFofaRowAsset(${idx}); event.stopPropagation();" title="${escapeHtml(_t('assets.importOne'))}">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none"><path d="M12 3v12m0 0 4-4m-4 4-4-4M4 19h16" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                </button>
            </div>
        `;

        return `<tr data-index="${idx}">${selectHtml}${cellsHtml}<td class="info-collect-col-actions">${actionHtml}</td></tr>`;
    }).join('');

    els.tbody.innerHTML = rowsHtml;

    // Update select-all checkbox state
    syncSelectAllCheckbox();
    if (typeof applyRBACToUI === 'function') applyRBACToUI(els.tbody);
}

function inferTargetFromRow(row, fields) {
    // Prefer host (FOFA often returns http(s)://...)
    const host = row.host != null ? String(row.host).trim() : '';
    if (host) return host;

    const domain = row.domain != null ? String(row.domain).trim() : '';
    const IP = row.IP != null ? String(row.IP).trim() : '';
    const port = row.port != null ? String(row.port).trim() : '';
    const protocol = row.protocol != null ? String(row.protocol).trim().toLowerCase() : '';

    const base = domain || IP;
    if (!base) return '';

    if (port) {
        // Lightweight inference: 443 -> https, 80 -> http, others do not force scheme
        const p = parseInt(port, 10);
        if (!isNaN(p) && (p === 80 || p === 443)) {
            const scheme = p === 443 ? 'https' : 'HTTP';
            return `${scheme}://${base}:${p}`;
        }
        if (protocol === 'https' || protocol === 'HTTP') {
            return `${protocol}://${base}:${port}`;
        }
        return `${base}:${port}`;
    }

    return base;
}

function normalizeHttpLink(raw) {
    const v = (raw || '').trim();
    if (!v) return '';
    if (v.startsWith('HTTP://') || v.startsWith('https://')) return v;
    // Some hosts may be domain or IP:port; do not assemble forcefully
    return '';
}

function copyFofaTarget(target) {
    const text = (target || '').trim();
    if (!text) {
        alert(_t('infoCollect.noTargetToCopy'));
        return;
    }
    navigator.clipboard.writeText(text).then(() => {
        // Simple hint
        showInlineToast(_t('infoCollect.targetCopied'));
    }).catch(() => {
        alert(_t('infoCollect.manualCopyHint') + text);
    });
}

function copyFofaTargetEncoded(encodedTarget) {
    try {
        copyFofaTarget(decodeURIComponent(encodedTarget || ''));
    } catch (e) {
        copyFofaTarget(encodedTarget || '');
    }
}

// showInlineToast('xxx'); also supports showInlineToast('xxx', { duration: 0, id: '...' })
function showInlineToast(text, options) {
    const opts = options && typeof options === 'object' ? options : {};
    const duration = typeof opts.duration === 'number' ? opts.duration : 1200;
    const id = typeof opts.id === 'string' && opts.id.trim() ? opts.id.trim() : '';
    const replace = opts.replace !== false;

    if (id && replace) {
        document.getElementById(id)?.remove();
    }

    const toast = document.createElement('div');
    if (id) toast.id = id;
    toast.textContent = String(text == null ? '' : text);
    toast.style.cssText = 'position: fixed; top: 24px; right: 24px; background: rgba(0,0,0,0.85); color: #fff; padding: 10px 12px; border-radius: 8px; z-index: 10000; font-size: 13px; max-width: 420px; line-height: 1.4; box-shadow: 0 6px 18px rgba(0,0,0,0.22);';
    document.body.appendChild(toast);

    let timer = null;
    const remove = () => {
        try { if (timer) clearTimeout(timer); } catch (e) { /* ignore */ }
        timer = null;
        try { toast.remove(); } catch (e) { /* ignore */ }
    };

    if (duration > 0) {
        timer = setTimeout(remove, duration);
    }

    return { el: toast, remove };
}

function truncateForPreview(value, maxLen) {
    const s = value == null ? '' : String(value);
    if (maxLen <= 0 || s.length <= maxLen) return s;
    return s.slice(0, maxLen) + '...(' + _t('infoCollect.truncated') + ')';
}

function formatFofaRowSummary(row, fields) {
    const r = row && typeof row === 'object' ? row : {};
    const order = [];
    const seen = new Set();

    const preferred = Array.isArray(fields) ? fields : [];
    preferred.forEach(k => {
        const key = String(k || '').trim();
        if (!key || seen.has(key)) return;
        seen.add(key);
        order.push(key);
    });

    Object.keys(r).sort().forEach(k => {
        if (seen.has(k)) return;
        seen.add(k);
        order.push(k);
    });

    if (order.length === 0) return '-';

    const lines = order.map((k) => {
        const v = r[k];
        let text = '';
        if (v === null) text = 'null';
        else if (v === undefined) text = '';
        else if (typeof v === 'string') text = v === '' ? '""' : v;
        else if (typeof v === 'number' || typeof v === 'boolean') text = String(v);
        else {
            try { text = JSON.stringify(v); } catch (e) { text = String(v); }
        }
        text = truncateForPreview(text, 800);
        return `- ${k}: ${text}`;
    });

    return lines.join('\n');
}

function scanFofaRow(encodedRowJson, clickEvent) {
    let row = {};
    try {
        row = JSON.parse(decodeURIComponent(encodedRowJson));
    } catch (e) {
        console.warn('Failed to parse row data', e);
    }

    const fields = (document.getElementById('FOFA-fields')?.value || '').split(',').map(s => s.trim()).filter(Boolean);
    const target = inferTargetFromRow(row, fields);
    if (!target) {
        alert(_t('infoCollect.cannotInferTarget'));
        return;
    }

    // Switch to chat page and send message (create new conversation each time)
    if (typeof switchPage === 'function') {
        switchPage('chat');
    } else {
        window.location.hash = 'chat';
    }

    const message = buildScanMessage(target, row, { fields });
    const autoSend = !!(clickEvent && (clickEvent.ctrlKey || clickEvent.metaKey));

    setTimeout(async () => {
        // Create new conversation: wait for completion to avoid clearing input
        try {
            if (typeof startNewConversation === 'function') {
                const maybePromise = startNewConversation();
                if (maybePromise && typeof maybePromise.then === 'function') {
                    await maybePromise;
                }
            }
        } catch (e) {
            // ignore
        }

        const input = document.getElementById('chat-input');
        if (input) {
            input.value = message;
            // Trigger auto height adjustment
            input.dispatchEvent(new Event('input', { bubbles: true }));
            input.focus();
        }
        if (autoSend) {
            if (typeof sendMessage === 'function') {
                window.__csNextChatFinalizationPolicy = { requireExecutionEvidence: true };
                sendMessage();
            } else {
                alert(_t('infoCollect.noSendMessage'));
            }
        } else {
            showInlineToast(_t('infoCollect.filledToInput'));
        }
    }, 250);
}

function buildScanMessage(target, row, options) {
    const opts = options && typeof options === 'object' ? options : {};
    const fields = Array.isArray(opts.fields) ? opts.fields : [];

    const summary = formatFofaRowSummary(row || {}, fields);
    const provider = providerLabel(infoCollectState.currentPayload?.provider || getInfoCollectProvider());
    return `Perform info collection and baseline scanning on the following target:
${target}

Requirements:
1) Identify service/framework and key fingerprints
2) Enumerate open ports and common management entry points
3) Rapidly verify accessible attack surface via httpx/fingerprints/directory probing
4) Output reproducible commands and conclusions

Known info (from ${provider} all fields for this row):
${summary}`.trim();
}

function bindFofaTableEvents() {
    if (infoCollectState.tableBound) return;
    infoCollectState.tableBound = true;

    const els = getFofaFormElements();
    if (!els.tbody) return;

    // Event delegation: selection / cell expand
    els.tbody.addEventListener('click', (e) => {
        const checkbox = e.target && e.target.classList && e.target.classList.contains('FOFA-row-select') ? e.target : null;
        if (checkbox) {
            const idx = parseInt(checkbox.getAttribute('data-index'), 10);
            if (!isNaN(idx)) {
                if (checkbox.checked) infoCollectState.selectedRowIndexes.add(idx);
                elseInfoCollectState.selectedRowIndexes.delete(idx);
                updateSelectedMeta();
                syncSelectAllCheckbox();
            }
            return;
        }

        const cell = e.target && e.target.closest ? e.target.closest('.info-collect-cell') : null;
        if (cell) {
            const full = cell.getAttribute('data-full') || '';
            const field = cell.getAttribute('data-field') || '';
            // Clicking link does not open modal
            if (e.target && e.target.tagName === 'A') return;
            if (full && full.length > 0) {
                showCellDetailModal(field, full);
            }
        }
    });

    // Select all in thead
    document.addEventListener('change', (e) => {
        const t = e.target;
        if (!t || t.id !== 'FOFA-select-all') return;
        const checked = !!t.checked;
        toggleSelectAllRows(checked);
    });
}

function toggleSelectAllRows(checked) {
    const els = getFofaFormElements();
    if (!els.tbody) return;
    const boxes = els.tbody.querySelectorAll('input.FOFA-row-select');
    infoCollectState.selectedRowIndexes.clear();
    boxes.forEach(b => {
        b.checked = checked;
        const idx = parseInt(b.getAttribute('data-index'), 10);
        if (checked && !isNaN(idx)) infoCollectState.selectedRowIndexes.add(idx);
    });
    updateSelectedMeta();
    syncSelectAllCheckbox();
}

function syncSelectAllCheckbox() {
    const selectAll = document.getElementById('FOFA-select-all');
    const els = getFofaFormElements();
    if (!selectAll || !els.tbody) return;
    const boxes = els.tbody.querySelectorAll('input.FOFA-row-select');
    const total = boxes.length;
    const selected = infoCollectState.selectedRowIndexes.size;
    if (total === 0) {
        selectAll.checked = false;
        selectAll.indeterminate = false;
        return;
    }
    if (selected === 0) {
        selectAll.checked = false;
        selectAll.indeterminate = false;
    } else if (selected === total) {
        selectAll.checked = true;
        selectAll.indeterminate = false;
    } else {
        selectAll.checked = false;
        selectAll.indeterminate = true;
    }
}

function renderFofaColumnsPanel(allFields, visibleFields) {
    const els = getFofaFormElements();
    if (!els.columnsList) return;
    const currentVisible = new Set(visibleFields);
    els.columnsList.innerHTML = allFields.map(f => {
        const checked = currentVisible.has(f);
        const safe = escapeHtml(f);
        return `
            <label class="info-collect-col-item" title="${safe}">
                <input type="checkbox" ${checked ? 'checked' : ''} onchange="toggleFofaColumn('${safe}', this.checked)" />
                <span>${safe}</span>
            </label>
        `;
    }).join('');
}

function toggleFofaColumn(field, visible) {
    const f = String(field || '').trim();
    if (!f) return;
    if (visible) infoCollectState.hiddenFields.delete(f);
    elseInfoCollectState.hiddenFields.add(f);
    saveHiddenFieldsToStorage();
    // Re-render table using cached payload in state
    if (infoCollectState.currentPayload) {
        renderFofaResults(infoCollectState.currentPayload);
    }
}

function toggleFofaColumnsPanel() {
    const els = getFofaFormElements();
    if (!els.columnsPanel) return;
    const show = els.columnsPanel.style.display === 'none' || !els.columnsPanel.style.display;
    els.columnsPanel.style.display = show ? 'block' : 'none';
}

function closeFofaColumnsPanel() {
    const els = getFofaFormElements();
    if (els.columnsPanel) els.columnsPanel.style.display = 'none';
}

// Click outside panel to close
document.addEventListener('click', (e) => {
    const panel = document.getElementById('FOFA-columns-panel');
    const btn = e.target && e.target.closest ? e.target.closest('button') : null;
    const isColumnsBtn = btn && btn.getAttribute && btn.getAttribute('onclick') && String(btn.getAttribute('onclick')).includes('toggleFofaColumnsPanel');
    if (!panel || panel.style.display === 'none') return;
    if (panel.contains(e.target) || isColumnsBtn) return;
    panel.style.display = 'none';
});

function showAllFofaColumns() {
    infoCollectState.hiddenFields.clear();
    saveHiddenFieldsToStorage();
    if (infoCollectState.currentPayload) renderFofaResults(infoCollectState.currentPayload);
}

function hideAllFofaColumns() {
    const p = infoCollectState.currentPayload;
    if (!p || !Array.isArray(p.fields)) return;
    // Allow hiding all, but retain at least host/IP/domain if present
    const keep = ['host', 'IP', 'domain'].find(x => p.fields.includes(x));
    infoCollectState.hiddenFields = new Set(p.fields.filter(f => f !== keep));
    saveHiddenFieldsToStorage();
    renderFofaResults(p);
}

function exportFofaResults(format) {
    const p = infoCollectState.currentPayload;
    if (!p || !Array.isArray(p.results) || p.results.length === 0) {
        alert(_t('infoCollect.noExportResult'));
        return;
    }

    const fields = p.fields || [];
    const visibleFields = fields.filter(f => !infoCollectState.hiddenFields.has(f));
    const provider = p.provider || 'FOFA';

    const now = new Date();
    const ts = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, '0')}${String(now.getDate()).padStart(2, '0')}_${String(now.getHours()).padStart(2, '0')}${String(now.getMinutes()).padStart(2, '0')}${String(now.getSeconds()).padStart(2, '0')}`;

    if (format === 'JSON') {
        const payload = {
            provider,
            query: p.query || '',
            total: p.total || 0,
             page: p. page || 1,
            size: p.size || 0,
            fields: fields,
            results: p.results
        };
        downloadBlob(JSON.stringify(payload, null, 2), `${provider}_results_${ts}.JSON`, 'application/json;charset=UTF-8');
        return;
    }

    if (format === 'XLSX') {
        // Use SheetJS to generate XLSX
        if (typeof XLSX === 'undefined') {
            alert(_t('infoCollect.xlsxNotLoaded'));
            return;
        }
        const aoa = [visibleFields].concat(p.results.map(row => {
            const r = row && typeof row === 'object' ? row : {};
            return visibleFields.map(f => r[f] != null ? r[f] : '');
        }));
        const ws = XLSX.utils.aoa_to_sheet(aoa);
        const wb = XLSX.utils.book_new();
        XLSX.utils.book_append_sheet(wb, ws, _t('infoCollect.batchScanTitle'));
        XLSX.writeFile(wb, `${provider}_results_${ts}.XLSX`);
        return;
    }

    // CSV: default export visible fields with UTF-8 BOM
    const header = visibleFields;
    const rows = p.results.map(row => {
        const r = row && typeof row === 'object' ? row : {};
        return header.map(f => csvEscape(r[f]));
    });
    const csv = [header.map(csvEscape).join(','), ...rows.map(cols => cols.join(','))].join('\n');
    const csvWithBom = '\uFEFF' + csv;
    downloadBlob(csvWithBom, `${provider}_results_${ts}.csv`, 'text/csv;charset=UTF-8');
}

function csvEscape(value) {
    if (value == null) return '""';
    const s = String(value).replace(/"/g, '""');
    return `"${s}"`;
}

function downloadBlob(content, filename, mime) {
    const blob = new Blob([content], { type: mime });
    const URL = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = URL;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(URL);
}

async function batchScanSelectedFofaRows() {
    const p = infoCollectState.currentPayload;
    if (!p || !Array.isArray(p.results) || p.results.length === 0) {
        alert(_t('infoCollect.noResults'));
        return;
    }
    const selected = Array.from(infoCollectState.selectedRowIndexes).sort((a, b) => a - b);
    if (selected.length === 0) {
        alert(_t('infoCollect.selectRowsFirst'));
        return;
    }

    const fields = p.fields || [];
    const tasks = [];
    const skipped = [];

    selected.forEach(idx => {
        const row = p.results[idx];
        const target = inferTargetFromRow(row || {}, fields);
        if (!target) {
            skipped.push(idx + 1);
            return;
        }
        // Batch task: same as single record, carries summary of row fields
        tasks.push(buildScanMessage(target, row || {}, {
            fields
        }));
    });

    if (tasks.length === 0) {
        alert(_t('infoCollect.noScanTarget'));
        return;
    }

    const title = (p.query ? _t('infoCollect.batchScanTitle') + ': ' + p.query : _t('infoCollect.batchScanTitle')).slice(0, 80);
    try {
        // Keep current selected role; pass empty string if default
        let role = '';
        if (typeof getCurrentRole === 'function') {
            try { role = getCurrentRole() || ''; } catch (e) { /* ignore */ }
        }
        if (role === 'default') role = '';

        const resp = await apiFetch('/api/batch-tasks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                title,
                tasks,
                role,
                projectId: typeof getActiveProjectId === 'function' ? getActiveProjectId() || '' : '',
            })
        });
        const result = await resp.json().catch(() => ({}));
        if (!resp.ok) {
            throw new Error(result.error || _t('infoCollect.createQueueFailed') + ': ' + resp.status);
        }
        const queueId = result.queueId;
        if (!queueId) {
            throw new Error('Created successfully but queueId was not returned');
        }

        // Navigate to Tasks and open queue details
        if (typeof switchPage === 'function') switchPage('tasks');
        setTimeout(() => {
            if (typeof showBatchQueueDetail === 'function') {
                showBatchQueueDetail(queueId);
            }
        }, 250);

        if (skipped.length > 0) {
            showInlineToast(_t('infoCollect.queueCreatedSkipped', { n: skipped.length }));
        } else {
            showInlineToast(_t('infoCollect.batchQueueCreated'));
        }
    } catch (e) {
        console.error('Batch scan failed:', e);
        alert(_t('infoCollect.batchScanFailed') + ': ' + (e && e.message ? e.message : String(e)));
    }
}

function showCellDetailModal(field, fullText) {
    const existing = document.getElementById('info-collect-cell-modal');
    if (existing) existing.remove();

    const text = fullText == null ? '' : String(fullText);
    const fieldName = field || _t('infoCollect.field');
    const charCountLabel = _t('infoCollect.cellValueLength', { count: Array.from(text).length });
    const modal = document.createElement('div');
    modal.id = 'info-collect-cell-modal';
    modal.className = 'info-collect-cell-modal';
    modal.innerHTML = `
        <div class="info-collect-cell-modal-content" role="dialog" aria-modal="true">
            <div class="info-collect-cell-modal-header">
                <div class="info-collect-cell-modal-heading">
                    <div class="info-collect-cell-modal-title">${escapeHtml(fieldName)}</div>
                    <div class="info-collect-cell-modal-subtitle">${escapeHtml(charCountLabel)}</div>
                </div>
                <button class="btn-icon" type="button" id="info-collect-cell-modal-close" title="${_t('common.close')}">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg">
                        <path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                    </svg>
                </button>
            </div>
            <div class="info-collect-cell-modal-body">
                <pre class="info-collect-cell-modal-pre">${escapeHtml(text)}</pre>
            </div>
            <div class="info-collect-cell-modal-footer">
                <button class="btn-secondary" type="button" id="info-collect-cell-modal-copy">${_t('common.copy')}</button>
                <button class="btn-primary" type="button" id="info-collect-cell-modal-ok">${_t('common.close')}</button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
    openAppModal(modal);

    const onKey = (e) => {
        if (e.key === 'Escape') {
            close();
        }
    };
    const close = function () {
        document.removeEventListener('keydown', onKey);
        closeAppModal(modal);
        modal.remove();
        syncAppModalBodyLock();
    };
    modal.addEventListener('click', (e) => {
        if (e.target === modal) close();
    });
    document.getElementById('info-collect-cell-modal-close')?.addEventListener('click', close);
    document.getElementById('info-collect-cell-modal-ok')?.addEventListener('click', close);
    document.getElementById('info-collect-cell-modal-copy')?.addEventListener('click', () => {
        navigator.clipboard.writeText(text).then(() => showInlineToast(_t('common.copied'))).catch(() => alert(_t('common.copyFailed')));
    });

    // Esc Close
    document.addEventListener('keydown', onKey);
}

// Expose globally for index.html onclick handlers
window.initInfoCollectPage = initInfoCollectPage;
window.resetFofaForm = resetFofaForm;
window.submitFofaSearch = submitFofaSearch;
window.parseFofaNaturalLanguage = parseFofaNaturalLanguage;
window.setInfoCollectQueryMode = setInfoCollectQueryMode;
window.scanFofaRow = scanFofaRow;
window.copyFofaTarget = copyFofaTarget;
window.copyFofaTargetEncoded = copyFofaTargetEncoded;
window.applyFofaQueryPreset = applyFofaQueryPreset;
window.applyFofaFieldsPreset = applyFofaFieldsPreset;
window.toggleFofaColumnsPanel = toggleFofaColumnsPanel;
window.closeFofaColumnsPanel = closeFofaColumnsPanel;
window.showAllFofaColumns = showAllFofaColumns;
window.hideAllFofaColumns = hideAllFofaColumns;
window.toggleFofaColumn = toggleFofaColumn;
window.exportFofaResults = exportFofaResults;
window.batchScanSelectedFofaRows = batchScanSelectedFofaRows;

document.addEventListener('languagechange', function () {
    updateSelectedMeta();
});

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { updateSelectedMeta(); });
} else {
    updateSelectedMeta();
}
