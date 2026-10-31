package handler

import (
	"testing"
)

func TestValidateWebshellTargetURL_SSRFProtection(t *testing.T) {
	blockedURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.1.1/secret",
		"http://127.0.0.1:8080/admin",
		"http://127.0.0.2/",
		"http://10.0.0.1/internal",
		"http://172.16.0.1/",
		"http://192.168.1.1/router",
		"http://localhost:8080",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://instance-data/latest/",
		"http://metadata.azure.com/metadata/instance",
		"http://internal.service.internal/",
		"http://cluster.local/",
		"ftp://example.com/shell.php",
		"file:///etc/passwd",
		"http://8.8.8.8:99999/", // invalid port
	}

	for _, u := range blockedURLs {
		if err := validateWebshellTargetURL(u); err == nil {
			t.Errorf("expected URL %q to be blocked by SSRF validation, but got nil", u)
		}
	}
}

func TestValidateWebshellTargetURL_ValidPublicURLs(t *testing.T) {
	validURLs := []string{
		"http://8.8.8.8/shell.php",
		"https://1.1.1.1:8443/webshell",
	}

	for _, u := range validURLs {
		if err := validateWebshellTargetURL(u); err != nil {
			t.Errorf("expected valid public URL %q to pass, got err: %v", u, err)
		}
	}
}
