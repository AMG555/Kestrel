const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = process.cwd();
const settings = fs.readFileSync(path.join(root, 'web/static/js/settings.js'), 'utf8');
const template = fs.readFileSync(path.join(root, 'web/templates/index.html'), 'utf8');
const zh = JSON.parse(fs.readFileSync(path.join(root, 'web/static/i18n/zh-CN.json'), 'utf8'));
const en = JSON.parse(fs.readFileSync(path.join(root, 'web/static/i18n/en-US.json'), 'utf8'));

test('Eino model retry/failover settings page read/write pipeline is complete', () => {
    [
        'eino-model-retry-max-retries',
        'eino-model-retry-max-backoff-sec',
        'eino-model-failover-channels',
        'eino-model-failover-max-retries'
    ].forEach((id) => {
        assert.match(template, new RegExp(`id="${id}"`));
        assert.match(settings, new RegExp(`getElementById\\('${id}'\\)`));
    });

    [
        'model_retry_max_retries',
        'model_retry_max_backoff_sec',
        'model_failover_channels',
        'model_failover_max_retries'
    ].forEach((key) => {
        assert.match(settings, new RegExp(`${key}`));
    });

    assert.match(settings, /failoverChannelsRaw\.split\(\s*\/\[\\n,，\]\//);    assert.match(settings, /Array\.from\(new Set\(/);
});

test('Eino model retry/failover settings have Chinese and English copy', () => {
    [
        'einoModelRetryMaxRetries',
        'einoModelRetryMaxRetriesHint',
        'einoModelRetryMaxBackoffSec',
        'einoModelRetryMaxBackoffSecHint',
        'einoModelFailoverChannels',
        'einoModelFailoverChannelsPlaceholder',
        'einoModelFailoverChannelsHint',
        'einoModelFailoverMaxRetries',
        'einoModelFailoverMaxRetriesHint'
    ].forEach((key) => {
        assert.equal(typeof zh.settingsBasic[key], 'string', `zh ${key}`);
        assert.ok(zh.settingsBasic[key].length > 0, `zh ${key} is empty`);
        assert.equal(typeof en.settingsBasic[key], 'string', `en ${key}`);
        assert.ok(en.settingsBasic[key].length > 0, `en ${key} is empty`);
    });
});

test('AI channel auto-detects DeepSeek official route before save', () => {
    assert.match(settings, /function\s+isOfficialDeepSeekBaseURL/);
    assert.match(settings, /api\.deepseek\.com/);
    assert.match(settings, /profile:\s*'deepseek'/);
    assert.match(settings, /normalizeAIChannelProviderProfile\(\{/);
    assert.match(settings, /normalizeAIConfigProviderProfiles\(currentConfig\.ai\)/);
    assert.match(template, /<option value="deepseek">deepseek<\/option>/);
});

test('Switching or adding AI channel refreshes the reasoning route dropdown', () => {
    assert.match(settings, /const profileEl = document\.getElementById\('openai-reasoning-profile'\);/);
    assert.match(settings, /syncSettingsCustomSelect\(profileEl\);/);
    assert.match(settings, /reasoning:\s*\{\s*mode:\s*'auto',\s*effort:\s*'',\s*profile:\s*'auto'/);
});
