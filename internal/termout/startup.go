package termout

import (
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// StartupWebUIOptions configures the startup banner.
type StartupWebUIOptions struct {
	Scheme     string
	Host       string
	Port       int
	SelfSigned bool
}

func startupHosts(host string) []string {
	host = strings.TrimSpace(host)
	if host != "" && host != "0.0.0.0" && host != "::" && host != "[::]" {
		return []string{host}
	}
	hosts := []string{"127.0.0.1"}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return hosts
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
			continue
		}
		ip := ipNet.IP.String()
		dup := false
		for _, h := range hosts {
			if h == ip {
				dup = true
				break
			}
		}
		if !dup {
			hosts = append(hosts, ip)
		}
	}
	return hosts
}

// PrintStartupWebUI prints a formatted operational banner on launch.
func PrintStartupWebUI(opts StartupWebUIOptions) {
	printStartupWebUI(os.Stdout, opts)
}

func printStartupWebUI(out io.Writer, opts StartupWebUIOptions) {
	s := New(out)
	scheme := opts.Scheme
	if scheme == "" {
		scheme = "http"
	}
	port := opts.Port
	if port <= 0 {
		port = 8080
	}
	hosts := startupHosts(opts.Host)
	urlFor := func(h string) string {
		return scheme + "://" + net.JoinHostPort(h, strconv.Itoa(port)) + "/"
	}

	s.BlankLine()
	s.Println(s.Bold(s.Cyan("⚔ KESTREL")) + s.Dim("  /  Autonomous Security Operations Platform"))
	s.Println(s.Dim(strings.Repeat("─", 64)))
	s.Println(s.Green("● ONLINE") + "   " + s.Bold(s.White(urlFor(hosts[0]))))
	for _, h := range hosts[1:] {
		s.Println(s.Dim("  Network  ") + s.Bold(s.White(urlFor(h))))
	}
	if opts.SelfSigned {
		s.Println(s.Dim("  TLS      ") + s.Yellow("self-signed") + s.Dim(" · accept browser certificate once"))
	}
	s.BlankLine()
}

// PrintBootstrapAdminCredentials prints the initial administrator password.
func PrintBootstrapAdminCredentials(password string) {
	password = strings.TrimSpace(password)
	if password == "" {
		return
	}

	s := New(os.Stdout)
	s.Println(s.Bold(s.Yellow("INITIAL CREDENTIALS")))
	s.Println(s.Dim(strings.Repeat("─", 64)))
	s.Println(s.Dim("  Username  ") + s.Bold(s.White("admin")))
	s.Println(s.Dim("  Password  ") + s.Bold(s.Yellow(password)))
	s.BlankLine()
	s.Println(s.Yellow("  ! ") + s.White("Record this password securely. It will not be shown again."))
	s.BlankLine()
}
