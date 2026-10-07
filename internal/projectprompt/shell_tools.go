package projectprompt

// ShellExecExecuteGuidanceSection 供单代理/多代理system prompt追加：exec 与 execute 分工（尽量短）。
func ShellExecExecuteGuidanceSection() string {
	return `Shell（exec/execute）：有专用 MCP tool时优先专用tool；system命令（管道、workdir、后台 &）用 exec；skills/ 内脚本（配合 read_file、skill）用 execute；多步scan分拆调用，禁止一条 shell 串多个scan器。长脚本、request体或 Payload 必须先用 write_file 写入会话working directory，再用 exec/execute 执行短命令；禁止把长内容嵌入 command。download/临时file须写入system prompt中的「会话working directory」，禁止用 /tmp。`
}

// ShellExecExecuteGuidanceReconSuffix 侦察sub-agent可选追加（一行）。
func ShellExecExecuteGuidanceReconSuffix() string {
	return `枚举优先 subfinder、amass 等专用 MCP，勿 exec/execute 拼长链。`
}
