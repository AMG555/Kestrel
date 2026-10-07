# Tool Configuration File Reference

## Overview

Each tool has its own configuration file stored in the `tools/` directory. This approach makes tool configuration clearer, easier to maintain, and simpler to manage. The system automatically loads all `.yaml` and `.yml` files under `tools/`.

## Configuration File Format

Each tool configuration file is a YAML file. The table below lists the supported top-level fields and whether they are required. Review each field before submitting:

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | ✅ | string | Unique tool identifier. Recommended: lowercase letters, digits, and hyphens. |
| `command` | ✅ | string | The actual command or script name to execute. Must be on the system PATH or specified as an absolute path. |
| `enabled` | ✅ | bool | Whether to register the tool with MCP. Set to `false` to ignore this tool. |
| `description` | ✅ | string | Detailed description supporting multi-line Markdown, used for deep AI understanding and `resources/read` queries. |
| `short_description` | optional | string | 20–50 character summary for tool lists to reduce token consumption. If omitted, the system automatically extracts the first line or first 100 characters of `description`. |
| `args` | optional | string[] | Fixed arguments prepended to the command line in order. Commonly used to define default scan modes. |
| `parameters` | optional | array | Runtime-configurable parameter list. See the "Parameter Definitions" section. |
| `arg_mapping` | optional | string | Argument mapping mode (`auto`/`manual`/`template`). Default `auto`. No need to set unless you have special requirements. |

> If a required field is missing or filled in incorrectly, the system will skip that tool at load time and print a warning in the log, but this will not affect other tools.

## Tool Descriptions

### Short Description (`short_description`)

- **Purpose**: Used in tool lists to reduce token consumption sent to the model
- **Requirement**: One sentence (20–50 characters) describing the tool's core purpose
- **Example**: `"Network scanning tool for discovering hosts, open ports, and services"`

### Detailed Description (`description`)

Supports multi-line text. Should include:

1. **Tool capabilities**: What the tool does
2. **Use cases**: When to use this tool
3. **Notes**: Warnings and caveats when using the tool
4. **Examples**: Usage examples (optional)

**Important notes:**
- When sending the tool list to the model, `short_description` is used (if present)
- If `short_description` is absent, the system auto-extracts the first line or first 100 characters from `description`
- The detailed description can be retrieved via the MCP `resources/read` endpoint (URI: `tool://tool_name`)

This significantly reduces token consumption, especially when there are many tools (e.g. 100 tools).

## Parameter Definitions

Each parameter can include the following fields:

- `name`: Parameter name
- `type`: Parameter type (string, int, bool, array)
- `description`: Detailed parameter description (multi-line supported)
- `required`: Whether the parameter is required (true/false)
- `default`: Default value
- `flag`: Command-line flag (e.g. "-u", "--url", "-p")
- `position`: Position for positional parameters (integer, starting at 0)
- `format`: Parameter format ("flag", "positional", "combined", "template")
- `template`: Template string (used with format="template")
- `options`: List of allowed values (for enum types)

### Parameter Format Descriptions

- **`flag`**: Flag parameter, formatted as `--flag value` or `-f value`
  - Example: `flag: "-u"` → `-u http://example.com`
  
- **`positional`**: Positional parameter, added to the command in order
  - Example: `position: 0` → first positional argument
  
- **`combined`**: Combined format, formatted as `--flag=value`
  - Example: `flag: "--level"`, `format: "combined"` → `--level=3`
  
- **`template`**: Template format using a custom template string
  - Example: `template: "{flag} {value}"` → custom format

### Special Parameters

#### `additional_args` Parameter

`additional_args` is a special parameter for passing extra command-line options not defined in the parameter list. This parameter is parsed and split by spaces into multiple arguments.

**Use cases:**
- Pass advanced tool options
- Pass parameters not defined in the configuration
- Pass complex parameter combinations

**Example:**
```yaml
- name: "additional_args"
  type: "string"
  description: "Extra tool arguments; separate multiple with spaces"
  required: false
  format: "positional"
```

**Usage examples:**
- `additional_args: "--script vuln -O"` → parsed as `["--script", "vuln", "-O"]`
- `additional_args: "-T4 --max-retries 3"` → parsed as `["-T4", "--max-retries", "3"]`

**Notes:**
- Parameters are split by spaces, but content inside quotes is preserved
- Ensure parameter format is correct to avoid command injection risks
- This parameter is appended at the end of the command

#### `scan_type` Parameter (specific tools)

Some tools (e.g. `nmap`) support a `scan_type` parameter to override the default scan type argument.

**Example (nmap):**
```yaml
- name: "scan_type"
  type: "string"
  description: "Scan type options, e.g. '-sV -sC'"
  required: false
  format: "positional"
```

**Usage examples:**
- `scan_type: "-sV -sC"` → version detection and script scanning
- `scan_type: "-A"` → comprehensive scan

**Notes:**
- If `scan_type` is specified, it replaces the default scan type argument in the tool configuration
- Separate multiple options with spaces

### Parameter Description Requirements

Parameter descriptions should include:

1. **Parameter purpose**: What this parameter does
2. **Format requirements**: Required format for the value (e.g. URL format, port range format)
3. **Example values**: Concrete example values (multiple examples in a list)
4. **Notes**: Things to be aware of when using the parameter (permission requirements, performance impact, security warnings)

**Description format recommendations:**
- Use Markdown formatting to improve readability
- Use `**bold**` to highlight important information
- Use lists for multiple examples or options
- Use code blocks for complex formats

**Example:**
```yaml
description: |
  Target IP address or domain. Can be a single IP, IP range, CIDR, or domain name.
  
  **Example values:**
  - Single IP: "192.168.1.1"
  - IP range: "192.168.1.1-100"
  - CIDR: "192.168.1.0/24"
  - Domain: "example.com"
  
  **Notes:**
  - Ensure the target address format is correct
  - Required parameter, cannot be empty
```

## Parameter Type Descriptions

### Boolean Type (bool)

Boolean parameters have special handling:
- `true`: only the flag is added, no value (e.g. `--flag`)
- `false`: no argument is added at all
- Supports multiple input formats: `true`/`false`, `1`/`0`, `"true"`/`"false"`

**Example:**
```yaml
- name: "verbose"
  type: "bool"
  description: "Verbose output mode"
  required: false
  default: false
  flag: "-v"
  format: "flag"
```

### String Type (string)

The most common parameter type; accepts any string value.

### Integer Type (int/integer)

Used for numeric parameters such as port numbers and levels.

**Example:**
```yaml
- name: "level"
  type: "int"
  description: "Test level, range 1-5"
  required: false
  default: 3
  flag: "--level"
  format: "combined"  # --level=3
```

### Array Type (array)

Arrays are automatically converted to a comma-separated string.

**Example:**
```yaml
- name: "ports"
  type: "array"
  item_type: "number"
  description: "Port list"
  required: false
  # Input: [80, 443, 8080]
  # Output: "80,443,8080"
```

## Examples

Refer to existing tool configuration files under `tools/`:

- `nmap.yaml`: Network scanning tool (includes `scan_type` and `additional_args` examples)
- `sqlmap.yaml`: SQL injection detection tool (includes `additional_args` example)
- `nikto.yaml`: Web server scanning tool
- `dirb.yaml`: Web directory scanning tool
- `exec.yaml`: System command execution tool

### Complete Example: nmap Tool Configuration

```yaml
name: "nmap"
command: "nmap"
args: ["-sT", "-sV", "-sC"]  # default scan types
enabled: true

short_description: "Network scanning tool for discovering hosts, open ports, and services"

description: |
  Network mapping and port scanning tool for discovering hosts, services, and open ports.
  
  **Key Features:**
  - Host discovery: detect active hosts on the network
  - Port scanning: identify open ports on target hosts
  - Service detection: detect service types and versions running on ports
  - OS detection: identify the target host's operating system
  - Vulnerability detection: use NSE scripts to detect common vulnerabilities

parameters:
  - name: "target"
    type: "string"
    description: "Target IP address or domain"
    required: true
    position: 0
    format: "positional"
  
  - name: "ports"
    type: "string"
    description: "Port range, e.g. 1-1000"
    required: false
    flag: "-p"
    format: "flag"
  
  - name: "scan_type"
    type: "string"
    description: "Scan type options, e.g. '-sV -sC'"
    required: false
    format: "positional"
  
  - name: "additional_args"
    type: "string"
    description: "Extra Nmap arguments, e.g. '--script vuln -O'"
    required: false
    format: "positional"
```

## Adding a New Tool

To add a new tool, create a new YAML file in the `tools/` directory, e.g. `my_tool.yaml`:

```yaml
name: "my_tool"
command: "my-command"
args: ["--default-arg"]  # fixed arguments (optional)
enabled: true

# Short description (recommended) - used in tool lists to reduce token consumption
short_description: "One sentence describing the tool's purpose"

# Detailed description - used for tool documentation and AI understanding
description: |
  Detailed tool description supporting multi-line text and Markdown.
  
  **Key Features:**
  - Feature 1
  - Feature 2
  
  **Use Cases:**
  - Use case 1
  - Use case 2
  
  **Notes:**
  - Usage notes
  - Permission requirements
  - Performance impact

parameters:
  - name: "target"
    type: "string"
    description: |
      Detailed description of the target parameter.
      
      **Example values:**
      - "value1"
      - "value2"
      
      **Notes:**
      - Format requirements
      - Usage restrictions
    required: true
    position: 0  # positional parameter
    format: "positional"
  
  - name: "option"
    type: "string"
    description: "Option parameter description"
    required: false
    flag: "--option"
    format: "flag"
  
  - name: "verbose"
    type: "bool"
    description: "Verbose output mode"
    required: false
    default: false
    flag: "-v"
    format: "flag"
  
  - name: "additional_args"
    type: "string"
    description: "Extra tool arguments; separate multiple with spaces"
    required: false
    format: "positional"
```

After saving the file, restart the service to automatically load the new tool.

### Tool Configuration Best Practices

1. **Parameter design**
   - Define commonly used parameters individually for clearer AI understanding
   - Use `additional_args` for flexibility and advanced usage
   - Provide clear descriptions and examples for all parameters

2. **Description optimization**
   - Use `short_description` to reduce token consumption
   - `description` should be detailed to help AI understand the tool's purpose
   - Use Markdown formatting to improve readability

3. **Default values**
   - Set reasonable default values for commonly used parameters
   - Boolean type defaults are usually `false`
   - Numeric type defaults should reflect tool characteristics

4. **Parameter validation**
   - Clearly state parameter format requirements in descriptions
   - Provide multiple example values
   - Describe parameter constraints and notes

5. **Security**
   - Add warnings for dangerous operations
   - State permission requirements
   - Remind users to only use on authorized targets

6. **Execution duration and timeouts (best practices)**
   - If a tool frequently runs for a long time (e.g. stuck on "Running" beyond 10–30 minutes), this is an abnormal hang. Recommendations:
     - Set `agent.tool_timeout_minutes` in **config.yaml** (default 10) to automatically terminate tools that exceed this limit and free resources.
     - Increase the value (e.g. 20, 30) only for tools that genuinely need longer runs. Setting it to 0 (unlimited) is not recommended.
     - Use "Stop Task" on the task monitoring page to immediately interrupt the current session and subsequent tool calls.
     - Where possible, implement interruptible tool logic or internal timeouts (e.g. via shell timeout) to work cooperatively with the system timeout.

## Disabling a Tool

To disable a tool, set the `enabled` field to `false` in its configuration file, or delete/rename the file.

Once disabled, the tool will not appear in the tool list and cannot be invoked by the AI.

## Tool Configuration Validation

The system performs basic validation when loading tool configurations:

- ✅ Check required fields (`name`, `command`, `enabled`)
- ✅ Validate parameter definition format
- ✅ Check that parameter types are supported

If a configuration is invalid, the system prints a warning in the startup log but does not prevent the server from starting. Invalid tool configurations are skipped; other tools continue to work normally.

## FAQ

### Q: How do I pass multiple parameter values?

A: For array-type parameters, the system automatically converts them to a comma-separated string. For cases requiring multiple independent parameters, use the `additional_args` parameter.

### Q: How do I override a tool's default parameters?

A: Some tools (e.g. `nmap`) support a `scan_type` parameter to override the default scan type. For other cases, use the `additional_args` parameter.

### Q: The tool has been "Running" for over 30 minutes — what should I do?

A: This is an abnormal hang. Recommendations:
1. Configure `agent.tool_timeout_minutes` in **config.yaml** (default 10); tools exceeding that many minutes are automatically terminated.
2. Use "Stop Task" on the monitoring page to immediately interrupt the task.
3. If the tool genuinely needs more time, increase `tool_timeout_minutes`, but do not set it to 0.

### Q: The tool execution failed — what should I do?

A: Check the following:
1. Is the tool installed and on the system PATH?
2. Is the tool configuration correct?
3. Does the parameter format meet the requirements?
4. Check the server log for detailed error information.

### Q: How do I test a tool configuration?

A: Use the `cmd/test-config/main.go` tool to test configuration loading:
```bash
go run cmd/test-config/main.go
```

### Q: How is parameter order controlled?

A: Use the `position` field to control the order of positional parameters. **Position 0 parameters (e.g. gobuster's `dir` subcommand) are placed immediately after the command name, before all flag parameters**, to support CLIs that require a "subcommand + options" form. Remaining flag parameters are added in the order they appear in the `parameters` list, followed by positional parameters at positions 1, 2, etc. `additional_args` is appended at the end.

## Tool Configuration Templates

### Basic Tool Template

```yaml
name: "tool_name"
command: "command"
enabled: true

short_description: "Short description (20-50 characters)"

description: |
  Detailed description of the tool's functionality, use cases, and notes.

parameters:
  - name: "target"
    type: "string"
    description: "Target parameter description"
    required: true
    position: 0
    format: "positional"
  
  - name: "additional_args"
    type: "string"
    description: "Extra tool arguments"
    required: false
    format: "positional"
```

### Tool Template with Flag Parameters

```yaml
name: "tool_name"
command: "command"
enabled: true

short_description: "Short description"

description: |
  Detailed description.

parameters:
  - name: "target"
    type: "string"
    description: "Target"
    required: true
    flag: "-t"
    format: "flag"
  
  - name: "option"
    type: "bool"
    description: "Option"
    required: false
    default: false
    flag: "--option"
    format: "flag"
  
  - name: "level"
    type: "int"
    description: "Level"
    required: false
    default: 3
    flag: "--level"
    format: "combined"
  
  - name: "additional_args"
    type: "string"
    description: "Extra arguments"
    required: false
    format: "positional"
```

## Related Documentation

- Project README: See `README.md` for the full project documentation
- Tool list: Browse all tool configuration files in the `tools/` directory
- API documentation: See the API section in the main README

