package discovery

import (
	"errors"
	"testing"
)

// Real `ss -ltn -p` output from a Debian host, including the header line, a
// dual-stack bind, a loopback-only service, a socket whose process could not be
// attributed (no root) and a multi-worker process column.
const ssOutput = `State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
LISTEN 0      4096       127.0.0.1:3000       0.0.0.0:*     users:(("grafana-server",pid=812,fd=8))
LISTEN 0      511          0.0.0.0:80          0.0.0.0:*     users:(("nginx",pid=999,fd=6),("nginx",pid=1000,fd=6))
LISTEN 0      511             [::]:80             [::]:*     users:(("nginx",pid=999,fd=7),("nginx",pid=1000,fd=7))
LISTEN 0      4096       127.0.0.1:5432       0.0.0.0:*
LISTEN 0      128          0.0.0.0:22          0.0.0.0:*     users:(("sshd",pid=1,fd=3))
LISTEN 0      128             [::]:22             [::]:*     users:(("sshd",pid=1,fd=4))
LISTEN 0      4096           [::1]:631             [::]:*     users:(("cupsd",pid=640,fd=7))
`

// Real `netstat -tlnp` output, including its two-line banner and the "-" that
// stands in for an unattributable process.
const netstatOutput = `Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 127.0.0.1:3000          0.0.0.0:*               LISTEN      812/grafana-server
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      1/sshd: /usr/sbin
tcp        0      0 127.0.0.1:5432          0.0.0.0:*               LISTEN      -
tcp6       0      0 :::80                   :::*                    LISTEN      999/nginx
`

func find(t *testing.T, listeners []Listener, port int) Listener {
	t.Helper()
	for _, l := range listeners {
		if l.Port == port {
			return l
		}
	}
	t.Fatalf("no listener reported for port %d (got %+v)", port, listeners)
	return Listener{}
}

// noEvidence stands for a host that told us nothing beyond its sockets — no
// container runtime, no readable process list.
func noEvidence() evidence { return evidence{} }

func TestParseSS(t *testing.T) {
	got := classify(parseSS(ssOutput), noEvidence())

	// 3000, 80, 5432, 22, 631 — the dual-stack pairs collapse into one row each.
	if len(got) != 5 {
		t.Fatalf("expected 5 distinct ports, got %d: %+v", len(got), got)
	}

	grafana := find(t, got, 3000)
	if grafana.Process != "grafana-server" || grafana.PID != 812 {
		t.Errorf("port 3000: got process %q pid %d, want grafana-server/812", grafana.Process, grafana.PID)
	}
	if !grafana.Loopback {
		t.Error("port 3000 binds 127.0.0.1 and should be reported as loopback-only")
	}
	// The process actually holding the socket outranks the port table's guess.
	if grafana.Service != "grafana-server" || grafana.Origin != OriginProcess {
		t.Errorf("port 3000: got %q from %q, want grafana-server from the process", grafana.Service, grafana.Origin)
	}
	if grafana.Scheme != "http" {
		t.Errorf("port 3000: got scheme %q, want http", grafana.Scheme)
	}

	// A process column listing several workers yields the first one, not a
	// mangled concatenation.
	if nginx := find(t, got, 80); nginx.Process != "nginx" || nginx.PID != 999 {
		t.Errorf("port 80: got process %q pid %d, want nginx/999", nginx.Process, nginx.PID)
	}

	// Unattributable sockets still count as listeners; only the process is lost.
	postgres := find(t, got, 5432)
	if postgres.Process != "" {
		t.Errorf("port 5432 has no process column; got %q", postgres.Process)
	}
	// Nothing was known about it, so the port number gets to guess — and says so.
	if postgres.Service != "PostgreSQL" || postgres.Origin != OriginPort {
		t.Errorf("port 5432: got %q from %q, want PostgreSQL from the port", postgres.Service, postgres.Origin)
	}
	if postgres.Scheme != "" {
		t.Errorf("port 5432 is not a web service; got scheme %q", postgres.Scheme)
	}

	if cups := find(t, got, 631); cups.Address != "::1" || !cups.Loopback {
		t.Errorf("port 631: got address %q loopback %v, want ::1 and loopback", cups.Address, cups.Loopback)
	}
}

// A port bound on both loopback and a wildcard is reachable from the network,
// and must not be shown as loopback-only regardless of which socket came first.
func TestClassifyPrefersTheWidestBinding(t *testing.T) {
	for _, order := range [][]Listener{
		{{Address: "127.0.0.1", Port: 8080}, {Address: "*", Port: 8080, Process: "caddy"}},
		{{Address: "*", Port: 8080, Process: "caddy"}, {Address: "127.0.0.1", Port: 8080}},
	} {
		got := classify(order, noEvidence())
		if len(got) != 1 {
			t.Fatalf("expected the two sockets to collapse into one port, got %+v", got)
		}
		if got[0].Loopback {
			t.Errorf("port 8080 has a wildcard socket and is not loopback-only: %+v", got[0])
		}
		if got[0].Process != "caddy" {
			t.Errorf("the known process should survive deduplication, got %q", got[0].Process)
		}
	}
}

func TestParseNetstat(t *testing.T) {
	got := classify(parseNetstat(netstatOutput), noEvidence())

	if len(got) != 4 {
		t.Fatalf("expected 4 ports, got %d: %+v", len(got), got)
	}
	if grafana := find(t, got, 3000); grafana.Process != "grafana-server" || !grafana.Loopback {
		t.Errorf("port 3000: got process %q loopback %v", grafana.Process, grafana.Loopback)
	}
	// "1/sshd: /usr/sbin" — the command keeps its own arguments after a colon,
	// which must not end up in the process name.
	if sshd := find(t, got, 22); sshd.Process != "sshd" || sshd.PID != 1 {
		t.Errorf("port 22: got process %q pid %d, want sshd/1", sshd.Process, sshd.PID)
	}
	if postgres := find(t, got, 5432); postgres.Process != "" {
		t.Errorf(`port 5432 reports "-" as its process; got %q`, postgres.Process)
	}
	if nginx := find(t, got, 80); nginx.Address != "*" || nginx.Loopback {
		t.Errorf("port 80 binds :::* and is not loopback-only; got %+v", nginx)
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort int
		wantOK   bool
	}{
		{"0.0.0.0:22", "*", 22, true},
		{"*:80", "*", 80, true},
		{"[::]:443", "*", 443, true},
		{"127.0.0.1:3000", "127.0.0.1", 3000, true},
		{"[::1]:631", "::1", 631, true},
		{"[fe80::1%eth0]:53", "fe80::1", 53, true},
		{"127.0.0.1:*", "", 0, false},
		{"nonsense", "", 0, false},
		{"127.0.0.1:70000", "", 0, false},
	}
	for _, c := range cases {
		host, port, ok := splitHostPort(c.in)
		if ok != c.wantOK || host != c.wantHost || port != c.wantPort {
			t.Errorf("splitHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
				c.in, host, port, ok, c.wantHost, c.wantPort, c.wantOK)
		}
	}
}

// An unrecognised port still gets an Open action when a known web server holds
// it, because that is the case where the port number tells us nothing.
func TestIdentifyFallsBackToTheProgram(t *testing.T) {
	if name, scheme := identify(4711, "caddy"); name != "" || scheme != "http" {
		t.Errorf("identify(4711, caddy) = (%q, %q), want an http scheme and no name", name, scheme)
	}
	if name, scheme := identify(4711, "postgres"); name != "" || scheme != "" {
		t.Errorf("identify(4711, postgres) = (%q, %q), want nothing recognised", name, scheme)
	}
}

func TestDialAddress(t *testing.T) {
	if got := DialAddress("*"); got != "127.0.0.1" {
		t.Errorf("a wildcard bind is dialled over loopback, got %q", got)
	}
	if got := DialAddress("10.0.0.5"); got != "10.0.0.5" {
		t.Errorf("a specific bind must be dialled on its own address, got %q", got)
	}
}

type stubRunner struct {
	responses map[string][]byte
	calls     []string
}

func (s *stubRunner) Run(command string) ([]byte, error) {
	s.calls = append(s.calls, command)
	out, ok := s.responses[command]
	if !ok {
		return nil, errors.New("command not found")
	}
	return out, nil
}

func (s *stubRunner) called(command string) bool {
	for _, c := range s.calls {
		if c == command {
			return true
		}
	}
	return false
}

func TestScanFallsBackToNetstat(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{netstatCommand: []byte(netstatOutput)}}
	got, err := Scan(runner)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected the netstat output to be used, got %+v", got)
	}
	if runner.calls[0] != ssCommand {
		t.Errorf("expected ss to be tried first, calls were %v", runner.calls)
	}
}

// A host that genuinely listens on nothing is a valid answer, not a reason to
// run the fallback and then report a missing tool.
func TestScanTreatsEmptyOutputAsAnAnswer(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{ssCommand: []byte("")}}
	got, err := Scan(runner)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no listeners, got %+v", got)
	}
	if runner.called(netstatCommand) {
		t.Errorf("the fallback must not run after a successful scan, calls were %v", runner.calls)
	}
}

// A host with no container runtime and an unreadable process list still gets a
// usable answer — the evidence pass is allowed to fail entirely.
func TestScanSurvivesMissingEvidence(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{ssCommand: []byte(ssOutput)}}
	got, err := Scan(runner)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 ports, got %+v", got)
	}
	if !runner.called(evidenceCommand) {
		t.Errorf("the evidence pass should have been attempted, calls were %v", runner.calls)
	}
}

func TestScanWithoutEitherTool(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{}}
	if _, err := Scan(runner); !errors.Is(err, ErrNoTool) {
		t.Fatalf("expected ErrNoTool, got %v", err)
	}
}

// A Docker host is where naming from the port number falls apart: every
// published port is held by docker-proxy, and the host port was chosen freely.
const dockerHostSS = `State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
LISTEN 0      4096       127.0.0.1:8931       0.0.0.0:*     users:(("docker-proxy",pid=4102,fd=4))
LISTEN 0      4096         0.0.0.0:9443       0.0.0.0:*     users:(("docker-proxy",pid=4210,fd=4))
LISTEN 0      4096       127.0.0.1:7654       0.0.0.0:*     users:(("docker-proxy",pid=4320,fd=4))
LISTEN 0      4096       127.0.0.1:8000       0.0.0.0:*     users:(("python3",pid=5001,fd=3))
LISTEN 0      128          0.0.0.0:22          0.0.0.0:*     users:(("sshd",pid=1,fd=3))
`

const dockerEvidence = "__SSHFIRST_CONTAINERS__\n" +
	"grafana\tgrafana/grafana:11.1.0\t127.0.0.1:8931->3000/tcp\n" +
	"portainer\tportainer/portainer-ce:2.21.0\t0.0.0.0:9443->9443/tcp, [::]:9443->9443/tcp\n" +
	"paperless\tghcr.io/paperless-ngx/paperless-ngx:2.11\t127.0.0.1:7654->8000/tcp\n" +
	"redis\tredis:7-alpine\t6379/tcp\n" +
	"__SSHFIRST_PROCESSES__\n" +
	"    1 /usr/sbin/sshd -D\n" +
	" 4102 /usr/bin/docker-proxy -proto tcp -host-ip 127.0.0.1 -host-port 8931\n" +
	" 5001 python3 -m http.server 8000\n"

func TestDockerContainersNameTheirPorts(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{
		ssCommand:       []byte(dockerHostSS),
		evidenceCommand: []byte(dockerEvidence),
	}}
	got, err := Scan(runner)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// The published host port is arbitrary, but the container knows its name
	// and the port the service uses inside it — which is the conventional one.
	grafana := find(t, got, 8931)
	if grafana.Service != "grafana" || grafana.Container != "grafana" {
		t.Errorf("port 8931: got service %q container %q, want grafana", grafana.Service, grafana.Container)
	}
	if grafana.Detail != "grafana/grafana:11.1.0" {
		t.Errorf("port 8931: got detail %q, want the image", grafana.Detail)
	}
	if grafana.Origin != OriginContainer {
		t.Errorf("port 8931: got origin %q, want %q", grafana.Origin, OriginContainer)
	}
	// 8931 means nothing; 3000 inside the container means Grafana, hence http.
	if grafana.Scheme != "http" {
		t.Errorf("port 8931: got scheme %q, want http from the container port 3000", grafana.Scheme)
	}

	// Paperless publishes an unrecognisable 7654 onto an inner 8000. Neither the
	// container name nor the host port says "web", but the inner port does —
	// which is the only reason this row gets an Open action at all.
	paperless := find(t, got, 7654)
	if paperless.Service != "paperless" || paperless.Scheme != "http" {
		t.Errorf("port 7654: got %q/%q, want paperless/http via the inner port 8000",
			paperless.Service, paperless.Scheme)
	}

	portainer := find(t, got, 9443)
	if portainer.Service != "portainer" || portainer.Scheme != "https" {
		t.Errorf("port 9443: got %q/%q, want portainer/https", portainer.Service, portainer.Scheme)
	}

	// Only published ports listen on the host; an exposed-only one must not
	// invent a row or attach itself to an unrelated port.
	for _, l := range got {
		if l.Port == 6379 {
			t.Errorf("port 6379 is exposed but not published and must not appear: %+v", l)
		}
	}

	// A generic runtime is rescued by its command line rather than being shown
	// as the meaningless "python3".
	if server := find(t, got, 8000); server.Service != "python3 http.server" {
		t.Errorf("port 8000: got service %q, want the command line to name it", server.Service)
	}
}

// Without access to the Docker socket every published port looks like
// docker-proxy. That name must never reach the UI — the port table, guesswork
// though it is, is still more useful than labelling a whole host "docker-proxy".
func TestDockerProxyIsNeverTheServiceName(t *testing.T) {
	runner := &stubRunner{responses: map[string][]byte{ssCommand: []byte(dockerHostSS)}}
	got, err := Scan(runner)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, l := range got {
		if l.Service == "docker-proxy" {
			t.Errorf("port %d was named after docker-proxy: %+v", l.Port, l)
		}
	}
	// 9443 is in the port table, so it still resolves without Docker's help.
	if portainer := find(t, got, 9443); portainer.Service != "Portainer (HTTPS)" || portainer.Origin != OriginPort {
		t.Errorf("port 9443: got %q from %q, want the port table to fill in", portainer.Service, portainer.Origin)
	}
}

func TestParsePublishedPorts(t *testing.T) {
	e := parseEvidence(dockerEvidence)

	if len(e.containers) != 3 {
		t.Fatalf("expected 3 published ports, got %+v", e.containers)
	}
	if c := e.containers[8931]; c.name != "grafana" || c.port != 3000 {
		t.Errorf("host port 8931: got %+v, want grafana on inner port 3000", c)
	}
	// Published on both address families; one entry, not two conflicting ones.
	if c := e.containers[9443]; c.name != "portainer" || c.port != 9443 {
		t.Errorf("host port 9443: got %+v", c)
	}
	if _, ok := e.containers[6379]; ok {
		t.Error("an exposed-only port must not be recorded as published")
	}
	if e.commands[5001] != "python3 -m http.server 8000" {
		t.Errorf("pid 5001: got command %q", e.commands[5001])
	}
}

func TestProgramName(t *testing.T) {
	cases := []struct {
		process, command, want string
	}{
		{"nginx", "", "nginx"},
		{"grafana-server", "/usr/share/grafana/bin/grafana server", "grafana-server"},
		// Uninformative processes fall through to the command line...
		{"python3", "python3 -m http.server 8000", "python3 http.server"},
		{"node", "node /opt/app/server.js", "node server.js"},
		{"docker-proxy", "/usr/bin/docker-proxy -proto tcp -host-port 8931", ""},
		// ...and when there is no command line either, nothing is claimed.
		{"docker-proxy", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := programName(c.process, c.command); got != c.want {
			t.Errorf("programName(%q, %q) = %q, want %q", c.process, c.command, got, c.want)
		}
	}
}
