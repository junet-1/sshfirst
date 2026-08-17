// Package discovery inspects what a connected host is listening on, so the UI
// can offer a one-click tunnel — and, for anything that speaks HTTP, a panel
// tab — to a port that is otherwise unreachable from this machine.
//
// The interesting case is a service bound to loopback: a Grafana on
// 127.0.0.1:3000 is invisible from outside the box and normally takes an
// `ssh -L` incantation to reach. Those are exactly the ones this reports.
//
// Listening sockets are read with ss(8), falling back to netstat(8) on hosts
// that predate iproute2. Both output formats are parsed here as pure functions,
// because the awkward part — column layouts that differ per tool, per address
// family and per privilege level — is the part worth testing without needing an
// SSH server.
package discovery

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Runner executes a command on the remote host and returns its stdout. It
// reports an error for a non-zero exit status, which is how Scan decides that a
// tool is missing and the fallback should be tried.
type Runner interface {
	Run(command string) ([]byte, error)
}

// Listener is one TCP socket the host accepts connections on.
type Listener struct {
	// Address is the bind address as reported by the remote tool: a literal
	// IP, or "*" for a wildcard bind.
	Address string `json:"address"`
	Port    int    `json:"port"`
	// Process is the program holding the socket, empty when the remote tool
	// could not attribute it (which is the normal case for other users'
	// sockets without root).
	Process string `json:"process"`
	PID     int    `json:"pid,omitempty"`
	// Loopback marks a socket reachable only from the host itself — the case
	// where a forward is not a convenience but the only way in.
	Loopback bool `json:"loopback"`
	// Service is the best name we could establish for what is behind the port.
	Service string `json:"service"`
	// Detail is the evidence Service was drawn from — a container image or the
	// process command line — so a wrong guess is visibly a guess.
	Detail string `json:"detail"`
	// Container is the container publishing this port, when one does.
	Container string `json:"container"`
	// Origin records how Service was established: "container", "process" or
	// "port". Only the last one is guesswork.
	Origin string `json:"origin"`
	// Scheme is "http" or "https" when this looks like a web interface worth
	// opening as a panel, and empty otherwise.
	Scheme string `json:"scheme"`
}

// Origin values for Listener.Origin.
const (
	OriginContainer = "container"
	OriginProcess   = "process"
	OriginPort      = "port"
)

const (
	ssCommand      = "ss -ltn -p"
	netstatCommand = "netstat -tlnp"

	// evidenceCommand collects what a port actually belongs to. Everything in
	// it is optional — a host without Docker, or a user who may not talk to its
	// socket, simply contributes nothing and naming falls back to the port.
	//
	// The markers delimit sections because this is one exec channel rather than
	// three round trips.
	evidenceCommand = "echo " + containerMarker + "\n" +
		"docker ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}' 2>/dev/null\n" +
		"podman ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}' 2>/dev/null\n" +
		"echo " + processMarker + "\n" +
		"ps -eo pid=,args= 2>/dev/null\n" +
		"exit 0"

	containerMarker = "__SSHFIRST_CONTAINERS__"
	processMarker   = "__SSHFIRST_PROCESSES__"
)

// ErrNoTool is returned when neither ss nor netstat could be run.
var ErrNoTool = errors.New("neither ss nor netstat is available on the host")

// Scan lists the host's listening TCP sockets and works out what is behind
// them. An empty result from a tool that ran successfully is a valid answer (a
// host really may listen on nothing but SSH), so the fallback is only tried
// when the tool itself failed.
func Scan(r Runner) ([]Listener, error) {
	listeners, err := scanListeners(r)
	if err != nil {
		return nil, err
	}
	return classify(listeners, gatherEvidence(r)), nil
}

func scanListeners(r Runner) ([]Listener, error) {
	if out, err := r.Run(ssCommand); err == nil {
		return parseSS(string(out)), nil
	}
	out, err := r.Run(netstatCommand)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoTool, err)
	}
	return parseNetstat(string(out)), nil
}

// evidence is what the host could tell us about its own processes. Every field
// is optional; naming degrades a step at a time rather than failing.
type evidence struct {
	// containers maps a published host port to the container behind it.
	containers map[int]container
	// commands maps a PID to its full command line.
	commands map[int]string
}

type container struct {
	name  string
	image string
	// port is the port inside the container. It is the conventional one — a
	// Grafana published as -p 8931:3000 is still 3000 inside — which is what
	// makes naming work at all when the host port was chosen at random.
	port int
}

// gatherEvidence is best effort by design: any failure leaves the maps empty
// and classification falls back to the port number.
func gatherEvidence(r Runner) evidence {
	out, _ := r.Run(evidenceCommand)
	return parseEvidence(string(out))
}

// ssProcess pulls the first program name and PID out of ss's process column,
// which looks like: users:(("nginx",pid=999,fd=6),("nginx",pid=1000,fd=6))
var ssProcess = regexp.MustCompile(`\("([^"]+)",pid=(\d+)`)

// parseSS reads `ss -ltn -p` output. Header lines are skipped by requiring the
// state column to be LISTEN, which holds across iproute2 versions regardless of
// whether a "Process" header column is printed.
func parseSS(out string) []Listener {
	var listeners []Listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "LISTEN" {
			continue
		}
		address, port, ok := splitHostPort(fields[3])
		if !ok {
			continue
		}
		l := Listener{Address: address, Port: port}
		if match := ssProcess.FindStringSubmatch(line); match != nil {
			l.Process = match[1]
			l.PID, _ = strconv.Atoi(match[2])
		}
		listeners = append(listeners, l)
	}
	return listeners
}

// parseNetstat reads `netstat -tlnp` output:
// tcp 0 0 127.0.0.1:6010 0.0.0.0:* LISTEN 1234/sshd: user
func parseNetstat(out string) []Listener {
	var listeners []Listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || !strings.HasPrefix(fields[0], "tcp") || fields[5] != "LISTEN" {
			continue
		}
		address, port, ok := splitHostPort(fields[3])
		if !ok {
			continue
		}
		l := Listener{Address: address, Port: port}
		if len(fields) > 6 && fields[6] != "-" {
			// "1234/sshd: user" — the PID, then a command that may carry its
			// own arguments after a colon.
			pid, name, found := strings.Cut(fields[6], "/")
			if found {
				l.PID, _ = strconv.Atoi(pid)
				l.Process = strings.TrimSpace(strings.SplitN(name, ":", 2)[0])
			}
		}
		listeners = append(listeners, l)
	}
	return listeners
}

// splitHostPort separates a listening address into its host and port halves.
// It handles every shape the two tools emit: "0.0.0.0:22", "*:80", "[::1]:631"
// and the scoped "[fe80::1%eth0]:53".
func splitHostPort(raw string) (string, int, bool) {
	colon := strings.LastIndex(raw, ":")
	if colon < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(raw[colon+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}

	host := strings.TrimSuffix(strings.TrimPrefix(raw[:colon], "["), "]")
	if scope := strings.Index(host, "%"); scope >= 0 {
		host = host[:scope]
	}
	if host == "" || host == "*" || host == "0.0.0.0" || host == "::" {
		return "*", port, true
	}
	return host, port, true
}

// publishedPort matches one entry of a container runtime's port column:
// "0.0.0.0:8931->3000/tcp", "127.0.0.1:9000->9000/tcp", "[::]:8931->3000/tcp".
// Entries without a "->" are merely exposed, not published, and are ignored —
// nothing on the host listens for them.
var publishedPort = regexp.MustCompile(`(?:\[[^\]]+\]|[\d.]+):(\d+)->(\d+)/tcp`)

// parseEvidence reads the marked sections of evidenceCommand's output.
func parseEvidence(out string) evidence {
	e := evidence{
		containers: make(map[int]container),
		commands:   make(map[int]string),
	}

	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch strings.TrimSpace(line) {
		case containerMarker:
			section = containerMarker
			continue
		case processMarker:
			section = processMarker
			continue
		}

		switch section {
		case containerMarker:
			addContainer(e.containers, line)
		case processMarker:
			pid, command, found := strings.Cut(strings.TrimSpace(line), " ")
			if !found {
				continue
			}
			if id, err := strconv.Atoi(pid); err == nil {
				e.commands[id] = strings.TrimSpace(command)
			}
		}
	}
	return e
}

func addContainer(into map[int]container, line string) {
	fields := strings.Split(line, "\t")
	if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" {
		return
	}
	name, image, ports := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), fields[2]
	for _, match := range publishedPort.FindAllStringSubmatch(ports, -1) {
		hostPort, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		containerPort, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		// A container published on both address families lists the same host
		// port twice; the first mapping wins and the second is identical.
		if _, seen := into[hostPort]; !seen {
			into[hostPort] = container{name: name, image: image, port: containerPort}
		}
	}
}

// classify deduplicates, annotates and orders the raw parser output.
//
// Dual-stack services appear twice (once per address family) and services bound
// to several interfaces once per interface; the UI wants one row per port. The
// widest binding wins, so a port reachable from the network is never mislabelled
// as loopback-only just because it also has a 127.0.0.1 socket.
func classify(raw []Listener, e evidence) []Listener {
	byPort := make(map[int]Listener, len(raw))
	for _, l := range raw {
		l.Loopback = isLoopback(l.Address)
		existing, seen := byPort[l.Port]
		if !seen {
			byPort[l.Port] = l
			continue
		}
		if existing.Process == "" && l.Process != "" {
			existing.Process, existing.PID = l.Process, l.PID
		}
		// Any non-loopback socket makes the port externally reachable.
		if !l.Loopback && existing.Loopback {
			existing.Address, existing.Loopback = l.Address, false
		}
		byPort[l.Port] = existing
	}

	listeners := make([]Listener, 0, len(byPort))
	for _, l := range byPort {
		listeners = append(listeners, describe(l, e))
	}

	// Strictly by port. Every row offers the same action, so grouping by a
	// property the list does not show would only make the order look arbitrary;
	// ascending ports let you find a port by scanning down the column.
	sort.Slice(listeners, func(i, j int) bool {
		return listeners[i].Port < listeners[j].Port
	})
	return listeners
}

func isLoopback(address string) bool {
	return address == "::1" || strings.HasPrefix(address, "127.")
}

// DialAddress is the destination a local forward should dial from the server's
// side to reach this listener. A wildcard bind is reached over loopback; a
// socket bound to one specific interface has to be dialled on that address.
func DialAddress(bindAddress string) string {
	if bindAddress == "" || bindAddress == "*" {
		return "127.0.0.1"
	}
	return bindAddress
}

type service struct {
	name   string
	scheme string
}

// wellKnown maps ports to what is conventionally behind them. It leans towards
// the self-hosted and homelab software this app is used with, because those are
// the services people actually need a tunnel for — nobody forwards port 25.
var wellKnown = map[int]service{
	22:    {"SSH", ""},
	25:    {"SMTP", ""},
	53:    {"DNS", ""},
	80:    {"HTTP", "http"},
	81:    {"Nginx Proxy Manager", "http"},
	139:   {"SMB", ""},
	443:   {"HTTPS", "https"},
	445:   {"SMB", ""},
	587:   {"SMTP (submission)", ""},
	631:   {"CUPS", "http"},
	993:   {"IMAPS", ""},
	1883:  {"MQTT", ""},
	2375:  {"Docker API", ""},
	2376:  {"Docker API (TLS)", ""},
	3000:  {"Grafana", "http"},
	3001:  {"Uptime Kuma", "http"},
	3306:  {"MySQL / MariaDB", ""},
	3389:  {"RDP", ""},
	5000:  {"HTTP (alt)", "http"},
	5055:  {"Overseerr", "http"},
	5432:  {"PostgreSQL", ""},
	5601:  {"Kibana", "http"},
	5900:  {"VNC", ""},
	6379:  {"Redis", ""},
	7878:  {"Radarr", "http"},
	8000:  {"HTTP (alt)", "http"},
	8006:  {"Proxmox VE", "https"},
	8008:  {"HTTP (alt)", "http"},
	8080:  {"HTTP (alt)", "http"},
	8081:  {"HTTP (alt)", "http"},
	8096:  {"Jellyfin", "http"},
	8112:  {"Deluge", "http"},
	8123:  {"Home Assistant", "http"},
	8443:  {"HTTPS (alt)", "https"},
	8888:  {"Jupyter", "http"},
	8989:  {"Sonarr", "http"},
	9000:  {"Portainer", "http"},
	9090:  {"Cockpit / Prometheus", "http"},
	9091:  {"Transmission", "http"},
	9443:  {"Portainer (HTTPS)", "https"},
	10000: {"Webmin", "https"},
	19999: {"Netdata", "http"},
	27017: {"MongoDB", ""},
	32400: {"Plex", "http"},
}

// webServers are programs that serve HTTP whatever port they were put on, so
// an unrecognised port still gets an Open action when one of them holds it.
var webServers = map[string]bool{
	"apache2":  true,
	"caddy":    true,
	"gunicorn": true,
	"httpd":    true,
	"lighttpd": true,
	"nginx":    true,
	"node":     true,
	"traefik":  true,
	"uvicorn":  true,
}

// opaque programs never name a service, whatever their arguments say. The worst
// offender is docker-proxy, which holds *every* published container port on a
// Docker host — naming from it would label a whole homelab "docker-proxy", and
// its arguments describe the forwarding, not the service behind it.
var opaque = map[string]bool{
	"docker-proxy":            true,
	"docker":                  true,
	"containerd-shim":         true,
	"containerd-shim-runc-v2": true,
	"conmon":                  true,
	"rootlesskit":             true,
	"slirp4netns":             true,
}

// runtimes only mean something together with what they were asked to run:
// "python3" is noise, "python3 -m http.server" is an answer.
var runtimes = map[string]bool{
	"bash":    true,
	"bun":     true,
	"deno":    true,
	"java":    true,
	"node":    true,
	"perl":    true,
	"php":     true,
	"python":  true,
	"python3": true,
	"ruby":    true,
	"sh":      true,
	"socat":   true,
	"systemd": true,
	"xinetd":  true,
}

// describe works out what is behind a port, preferring evidence over guesswork.
//
// The order matters and is the whole point of this function. A container knows
// its own name and, more usefully, the port the service uses *inside* it: a
// Grafana published as -p 8931:3000 is unrecognisable from 8931 but obvious
// from 3000. Only when nothing on the host could tell us anything does the
// port number get to guess.
func describe(l Listener, e evidence) Listener {
	if c, ok := e.containers[l.Port]; ok {
		l.Container = c.name
		l.Service = c.name
		l.Detail = c.image
		l.Origin = OriginContainer
		// Name and scheme come from different places on purpose: the container
		// is authoritative about the name, the inner port about the protocol.
		_, l.Scheme = identify(c.port, "")
		if l.Scheme == "" {
			_, l.Scheme = identify(l.Port, "")
		}
		return l
	}

	command := e.commands[l.PID]
	if command != "" {
		l.Detail = command
	}
	if name := programName(l.Process, command); name != "" {
		l.Service = name
		l.Origin = OriginProcess
		if _, scheme := identify(l.Port, name); scheme != "" {
			l.Scheme = scheme
		}
		return l
	}

	l.Service, l.Scheme = identify(l.Port, "")
	if l.Service != "" {
		l.Origin = OriginPort
	}
	return l
}

// programName returns a name worth showing, or "" when the process tells us
// nothing. A generic runtime is rescued by its command line: "python3" alone is
// noise, but "python3 -m http.server" identifies the service.
func programName(process, command string) string {
	if lower := strings.ToLower(process); process != "" {
		if opaque[lower] {
			return ""
		}
		if !runtimes[lower] {
			return process
		}
	}
	if command == "" {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	binary := path.Base(fields[0])
	switch lower := strings.ToLower(binary); {
	case opaque[lower]:
		return ""
	case !runtimes[lower]:
		return binary
	}

	// A runtime plus what it is running, e.g. "python3 -m http.server" or
	// "node /app/server.js" — enough to recognise the service without pasting a
	// 300-character command line into a table cell.
	//
	// The target is required to look like a module or path, because the token
	// after a flag is usually that flag's value: "-proto tcp" would otherwise
	// name the service "tcp".
	for _, arg := range fields[1:] {
		if strings.HasPrefix(arg, "-") || !strings.ContainsAny(arg, "./") {
			continue
		}
		return binary + " " + path.Base(arg)
	}
	return ""
}

// identify names a port from convention, and tells whether it is likely to
// speak HTTP. This is the last resort: everything it returns is a guess based
// on nothing but the number.
func identify(port int, program string) (name, scheme string) {
	if known, ok := wellKnown[port]; ok {
		return known.name, known.scheme
	}
	if program != "" && webServers[strings.ToLower(program)] {
		return "", "http"
	}
	return "", ""
}
