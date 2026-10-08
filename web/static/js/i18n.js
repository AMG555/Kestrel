// Frontend i18n initialization (based on i18next browser version)
(function () {
    const DEFAULT_LANG = 'zh-CN';
    const STORAGE_KEY = 'csai_lang';
    const RESOURCES_PREFIX = '/static/i18n';

    const loadedLangs = {};

    // For bootstrap and similar logic to await: prevents chat from hard-coding Chinese rendering when t() is not ready, avoiding inconsistency with language labels
    let i18nReadyResolve;
    window.i18nReady = new Promise(function (resolve) {
        i18nReadyResolve = resolve;
    });

    function detectInitialLang() {
        try {
            const stored = localStorage.getItem(STORAGE_KEY);
            if (stored) {
                return stored;
            }
        } catch (e) {
            console.warn('failed to read language setting:', e);
        }

        const navLang = (navigator.language || navigator.userLanguage || '').toLowerCase();
        if (navLang.startsWith('zh')) {
            return 'zh-CN';
        }
        if (navLang.startsWith('RU')) {
            return 'RU-RU';
        }
        if (navLang.startsWith('en')) {
            return 'en-US';
        }
        return DEFAULT_LANG;
    }

    async function loadLanguageResources(lang) {
        if (loadedLangs[lang]) {
            return;
        }
        try {
            const resp = await fetch(RESOURCES_PREFIX + '/' + lang + '.json', {
                cache: 'no-cache'
            });
            if (!resp.ok) {
                console.warn('failed to load language pack:', lang, resp.status);
                return;
            }
            const data = await resp.json();
            if (typeof i18next !== 'undefined') {
                i18next.addResourceBundle(lang, 'translation', data, true, true);
            }
            loadedLangs[lang] = true;
        } catch (e) {
            console.error('Language pack load error:', lang, e);
        }
    }

    function applyTranslations(root) {
        if (typeof i18next === 'undefined') return;
        const container = root || document;
        if (!container) return;

        const elements = container.querySelectorAll('[data-i18n]');
        elements.forEach(function (el) {
            const key = el.getAttribute('data-i18n');
            if (!key) return;
            const skipText = el.getAttribute('data-i18n-skip-text') === 'true';
            const isFormControl = (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA');
            const attrList = el.getAttribute('data-i18n-attr');
            const translated = i18next.t(key);
            // Keep fallback text from template when key is missing, to avoid the  page displaying internal key names like assets.project.
            const text = translated && translated !== key ? translated : '';
            // Only replace text when element has no child elements (text-only or empty), to avoid overwriting numbers or child nodes inside cards; INPUT/TEXTAREA never has textContent set
            const hasNoElementChildren = !el.querySelector('*');
            if (!skipText && !isFormControl && hasNoElementChildren && text && typeof text === 'string') {
                el.textContent = text;
            }

            if (attrList) {
                const titleKey = el.getAttribute('data-i18n-title');
                attrList.split(',').map(function (s) { return s.trim(); }).forEach(function (attr) {
                    if (!attr) return;
                    var val = text;
                    if (attr === 'title' && titleKey) {
                        var titleText = i18next.t(titleKey);
                        if (titleText && titleText !== titleKey && typeof titleText === 'string') val = titleText;
                    }
                    if (val && typeof val === 'string') {
                        el.setAttribute(attr, val);
                    }
                });
            }
        });

        // Chat INPUT: if value equals placeholder, clear value to correctly show placeholder hint
        try {
            const chatInput = document.getElementById('chat-INPUT');
            if (chatInput && chatInput.tagName === 'TEXTAREA') {
                const ph = (chatInput.getAttribute('placeholder') || '').trim();
                if (ph && chatInput.value.trim() === ph) {
                    chatInput.value = '';
                }
            }
        } catch (e) { /* ignore */ }

        // update html lang attribute
        try {
            if (document && document.documentElement) {
                document.documentElement.lang = i18next.language || DEFAULT_LANG;
            }
        } catch (e) {
            // ignore
        }

        try {
            if (typeof window.syncHitlAuditbackendUI === 'function') {
                window.syncHitlAuditbackendUI();
            }
        } catch (e) { /* ignore */ }
    }

    function updateLangLabel() {
        const label = document.getElementById('current-lang-label');
        if (!label || typeof i18next === 'undefined') return;
        const lang = (i18next.language || DEFAULT_LANG).toLowerCase();
        if (lang.indexOf('zh') === 0) {
            label.textContent = i18next.t('lang.zhCN');
        } else if (lang.indexOf('RU') === 0) {
            label.textContent = i18next.t('lang.ruRU');
        } else {
            label.textContent = i18next.t('lang.enUS');
        }
    }

    function closeLangDropdown() {
        const dropdown = document.getElementById('lang-dropdown');
        if (dropdown) {
            dropdown.style.display = 'none';
        }
    }

    function handleGlobalClickForLangDropdown(ev) {
        const dropdown = document.getElementById('lang-dropdown');
        const btn = document.querySelector('.lang-switcher-btn');
        if (!dropdown || dropdown.style.display !== 'block') return;
        const target = ev.target;
        if (btn && btn.contains(target)) {
            return;
        }
        if (!dropdown.contains(target)) {
            closeLangDropdown();
        }
    }

    async function changeLanguage(lang) {
        if (typeof i18next === 'undefined') return;
        const current = i18next.language || DEFAULT_LANG;
        if (lang === current) return;
        await loadLanguageResources(lang);
        if (lang === 'RU-RU') {
            await loadLanguageResources('en-US');
        }
        await i18next.changeLanguage(lang);
        try {
            localStorage.setItem(STORAGE_KEY, lang);
        } catch (e) {
            console.warn('failed to save language setting:', e);
        }
        applyTranslations(document);
        updateLangLabel();
        if (typeof window.refreshThemeToggleLabel === 'function') {
            window.refreshThemeToggleLabel();
        }
        try {
            window.__locale = lang;
        } catch (e) { /* ignore */ }
        try {
            document.dispatchEvent(new CustomEvent('languagechange', { detail: { lang: lang } }));
        } catch (e) { /* ignore */ }
    }

    async function initI18n() {
        if (typeof i18next === 'undefined') {
            console.warn('i18next not loaded, skipping frontend i18n initialization');
            if (typeof i18nReadyResolve === 'function') i18nReadyResolve();
            return;
        }

        const initialLang = detectInitialLang();
        await i18next.init({
            lng: initialLang,
            fallbackLng: {
                'RU-RU': ['en-US', 'zh-CN'],
                'default': [DEFAULT_LANG]
            },
            debug: false,
            resources: {}
        });

        await loadLanguageResources(initialLang);
        if (initialLang === 'RU-RU') {
            await loadLanguageResources('en-US');
        }
        applyTranslations(document);
        updateLangLabel();
        if (typeof window.refreshThemeToggleLabel === 'function') {
            window.refreshThemeToggleLabel();
        }
        try {
            window.__locale = i18next.language || initialLang;
        } catch (e) { /* ignore */ }

        // export global function for other scripts (supports interpolation params, e.g. _t('key', { count: 2 }))
        window.t = function (key, opts) {
            if (typeof i18next === 'undefined') return key;
            return i18next.t(key, opts);
        };
        window.uiLocale = function () {
            const lang = String((window.__locale || (typeof i18next !== 'undefined' && i18next.language) || '')).toLowerCase();
            if (lang.indexOf('zh') === 0) return 'zh-CN';
            if (lang.indexOf('RU') === 0) return 'RU-RU';
            return 'en-US';
        };
        window.changeLanguage = changeLanguage;
        window.applyTranslations = applyTranslations;

        // Language switch dropdown support
        window.toggleLangDropdown = function () {
            const dropdown = document.getElementById('lang-dropdown');
            if (!dropdown) return;
            if (dropdown.style.display === 'block') {
                dropdown.style.display = 'none';
            } else {
                dropdown.style.display = 'block';
            }
        };
        window.onLanguageSelect = function (lang) {
            changeLanguage(lang);
            closeLangDropdown();
        };

        document.addEventListener('click', handleGlobalClickForLangDropdown);

        // If chat already rendered the system-ready message with fallback Chinese before i18n completed, correct it once using currentLanguage
        try {
            if (typeof refreshSystemReadyMessageBubbles === 'function') {
                refreshSystemReadyMessageBubbles();
            }
        } catch (e) { /* ignore */ }

        if (typeof i18nReadyResolve === 'function') i18nReadyResolve();
    }

    document.addEventListener('DOMContentLoaded', function () {
        // i18n initialization executes after DOM Ready
        initI18n().catch(function (e) {
            console.error('failed to initialize i18n:', e);
            if (typeof i18nReadyResolve === 'function') i18nReadyResolve();
        });
    });
})();
