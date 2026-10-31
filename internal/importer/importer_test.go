package importer_test

import (
	"path/filepath"
	"strings"
	"testing"

	"kestrel/internal/database"
	"kestrel/internal/importer"

	"go.uber.org/zap"
)

const sampleNmapXML = `<?xml version="1.0" encoding="UTF-8"?>
<nmaprun scanner="nmap" args="nmap -sV -oX - 192.168.1.10">
  <host>
    <status state="up"/>
    <address addr="192.168.1.10" addrtype="ipv4"/>
    <hostnames>
      <hostname name="target.corp" type="user"/>
    </hostnames>
    <ports>
      <port protocol="tcp" portid="80">
        <state state="open"/>
        <service name="http" product="nginx" version="1.18.0"/>
        <script id="http-title" output="Test Corporate Portal"/>
      </port>
      <port protocol="tcp" portid="445">
        <state state="open"/>
        <service name="microsoft-ds" product="Windows Server"/>
        <script id="smb-vuln-ms17-010" output="VULNERABLE: Remote Code Execution vulnerability in Microsoft SMBv1 servers (ms17-010)"/>
      </port>
    </ports>
  </host>
</nmaprun>`

const sampleNucleiJSONL = `{"template-id":"cve-2021-41773","info":{"name":"Apache 2.4.49 - Path Traversal","severity":"critical","description":"Path traversal flaw","remediation":"Upgrade Apache"},"type":"http","host":"http://192.168.1.10:80","matched-at":"http://192.168.1.10:80/icons/.%%32%65/etc/passwd","curl-command":"curl -X GET http://192.168.1.10/icons/.%%32%65/etc/passwd"}
{"template-id":"tech-detect","info":{"name":"Wappalyzer Tech Detection","severity":"info","description":"Tech detected"},"type":"http","host":"https://app.target.corp","matched-at":"https://app.target.corp","curl-command":"curl https://app.target.corp"}
`

const sampleBurpXML = `<?xml version="1.0"?>
<issues burpVersion="2023.12.1">
  <issue>
    <name>Cross-site scripting (reflected)</name>
    <host ip="192.168.1.25">https://app.target.corp</host>
    <path>/search</path>
    <location>/search?q=&lt;script&gt;alert(1)&lt;/script&gt;</location>
    <severity>High</severity>
    <confidence>Certain</confidence>
    <issueBackground>XSS occurs when...</issueBackground>
    <issueDetail>The value of parameter q was reflected in response body.</issueDetail>
    <remediationBackground>Use contextual output encoding.</remediationBackground>
  </issue>
</issues>`

func TestDetectFormat(t *testing.T) {
	if got := importer.DetectFormat([]byte(sampleNmapXML)); got != importer.FormatNmap {
		t.Fatalf("DetectFormat(nmap) = %q, want %q", got, importer.FormatNmap)
	}
	if got := importer.DetectFormat([]byte(sampleNucleiJSONL)); got != importer.FormatNuclei {
		t.Fatalf("DetectFormat(nuclei) = %q, want %q", got, importer.FormatNuclei)
	}
	if got := importer.DetectFormat([]byte(sampleBurpXML)); got != importer.FormatBurp {
		t.Fatalf("DetectFormat(burp) = %q, want %q", got, importer.FormatBurp)
	}
}

func TestImportData_Nmap(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "nmap_test.db")
	db, err := database.NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if _, err := db.CreateProject(&database.Project{
		ID:   "proj-1",
		Name: "Test Engagement",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	res, err := importer.ImportData(db, []byte(sampleNmapXML), "", "proj-1", "operator-1")
	if err != nil {
		t.Fatalf("ImportData failed: %v", err)
	}
	t.Logf("Result: %+v", res)

	if res.Format != importer.FormatNmap {
		t.Errorf("Format = %q, want %q", res.Format, importer.FormatNmap)
	}
	if res.AssetsCreated != 2 {
		t.Errorf("AssetsCreated = %d, want 2", res.AssetsCreated)
	}
	if res.VulnsCreated != 1 {
		t.Errorf("VulnsCreated = %d, want 1", res.VulnsCreated)
	}

	// Verify audit log appended
	logs, err := db.ListAuditLogs(database.ListAuditLogsFilter{Limit: 10})
	if err != nil || len(logs) == 0 {
		t.Fatalf("expected audit log entries, got err=%v logs=%d", err, len(logs))
	}
	if logs[0].Action != "import_scan" {
		t.Errorf("audit action = %q, want import_scan", logs[0].Action)
	}
}

func TestImportData_NucleiAndBurp(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "multi_import.db")
	db, err := database.NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if _, err := db.CreateProject(&database.Project{
		ID:   "proj-1",
		Name: "Test Engagement Multi",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 1. Import Nuclei
	nucleiRes, err := importer.ImportData(db, []byte(sampleNucleiJSONL), "", "proj-1", "operator-1")
	if err != nil {
		t.Fatalf("ImportData(nuclei) failed: %v", err)
	}
	if nucleiRes.VulnsCreated != 2 {
		t.Errorf("Nuclei VulnsCreated = %d, want 2", nucleiRes.VulnsCreated)
	}

	// 2. Import Burp
	burpRes, err := importer.ImportData(db, []byte(sampleBurpXML), "", "proj-1", "operator-1")
	if err != nil {
		t.Fatalf("ImportData(burp) failed: %v", err)
	}
	if burpRes.VulnsCreated != 1 {
		t.Errorf("Burp VulnsCreated = %d, want 1", burpRes.VulnsCreated)
	}

	// 3. Verify audit chain integrity survives multiple imports
	valid, err := db.VerifyAuditLogChain()
	if err != nil || !valid {
		t.Fatalf("VerifyAuditLogChain failed: valid=%v, err=%v", valid, err)
	}
}
