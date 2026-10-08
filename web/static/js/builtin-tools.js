/**
 * Built-in tool name constants.
 * All places in frontend code that use built-in tool names should use these constants
 * instead of hard-coded strings.
 *
 * Note: These constants must stay in sync with the backend constants in
 * internal/mcp/builtin/constants.go
 */

// Built-in tool name constants
const BuiltinTools = {
    // Vulnerability tool
    RECORD_VULNERABILITY: 'RECORD_VULNERABILITY',

    // Knowledge base tools
    LIST_KNOWLEDGE_RISK_TYPES: 'LIST_KNOWLEDGE_RISK_TYPES',
    SEARCH_KNOWLEDGE_BASE: 'SEARCH_KNOWLEDGE_BASE'
};

// Check whether a tool is a built-in tool
function isBuiltinTool(toolName) {
    return Object.values(BuiltinTools).includes(toolName);
}

// Get list of all built-in tool names
function getAllBuiltinTools() {
    return Object.values(BuiltinTools);
}

