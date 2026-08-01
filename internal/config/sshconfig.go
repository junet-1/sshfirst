// Package config reads the OpenSSH client configuration (~/.ssh/config) so
// its Host entries can be imported into the local host list. It only reads;
// it never writes back to the user's ssh config.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// ImportedHost is one concrete (non-wildcard) Host alias resolved from an
// OpenSSH config file, following Include directives.
type ImportedHost struct {
	Alias         string   `json:"alias"`
	Hostname      string   `json:"hostname"`
	Port          int      `json:"port"`
	User          string   `json:"user"`
	IdentityFiles []string `json:"identityFiles"`
	ProxyJump     string   `json:"proxyJump"`
	ForwardAgent  bool     `json:"forwardAgent"`
}

const maxIncludeDepth = 5

// DefaultPath returns the current user's ~/.ssh/config path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// Import parses the OpenSSH config file at path, follows any Include
// directives (recursively, matching OpenSSH glob semantics), and returns one
// entry per concrete Host alias. Wildcard patterns ("*", "192.168.*", "!x")
// are skipped since they don't name a single connectable host.
func Import(path string) ([]ImportedHost, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	cfg, decodeErr := ssh_config.Decode(f)
	f.Close()
	if decodeErr != nil {
		return nil, decodeErr
	}

	aliases, err := collectAliases(path, 0, map[string]bool{})
	if err != nil {
		return nil, err
	}

	hosts := make([]ImportedHost, 0, len(aliases))
	for _, alias := range aliases {
		host, err := resolveAlias(cfg, alias)
		if err != nil {
			continue // skip aliases we fail to resolve rather than aborting the whole import
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func resolveAlias(cfg *ssh_config.Config, alias string) (ImportedHost, error) {
	hostname, err := cfg.Get(alias, "HostName")
	if err != nil {
		return ImportedHost{}, err
	}
	if hostname == "" {
		hostname = alias
	}
	user, _ := cfg.Get(alias, "User")
	portStr, _ := cfg.Get(alias, "Port")
	port := 22
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}
	proxyJump, _ := cfg.Get(alias, "ProxyJump")
	forwardAgentStr, _ := cfg.Get(alias, "ForwardAgent")

	var identityFiles []string
	if idfs, err := cfg.GetAll(alias, "IdentityFile"); err == nil {
		for _, idf := range idfs {
			identityFiles = append(identityFiles, expandHome(idf))
		}
	}

	return ImportedHost{
		Alias:         alias,
		Hostname:      hostname,
		Port:          port,
		User:          user,
		IdentityFiles: identityFiles,
		ProxyJump:     proxyJump,
		ForwardAgent:  strings.EqualFold(forwardAgentStr, "yes"),
	}, nil
}

// collectAliases walks path and any files it Includes, extracting Host
// pattern tokens in declaration order (first occurrence wins, matching
// OpenSSH's "first obtained value" rule for later lookups via cfg.Get).
func collectAliases(path string, depth int, visited map[string]bool) ([]string, error) {
	if depth > maxIncludeDepth {
		return nil, nil
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		if visited[absPath] {
			return nil, nil
		}
		visited[absPath] = true
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var aliases []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		keyword, args, ok := splitDirective(scanner.Text())
		if !ok {
			continue
		}
		switch strings.ToLower(keyword) {
		case "host":
			for _, pattern := range args {
				if isWildcardPattern(pattern) || seen[pattern] {
					continue
				}
				seen[pattern] = true
				aliases = append(aliases, pattern)
			}
		case "include":
			for _, glob := range args {
				matches, err := resolveIncludeGlob(glob)
				if err != nil {
					continue
				}
				for _, m := range matches {
					nested, err := collectAliases(m, depth+1, visited)
					if err != nil {
						continue
					}
					for _, alias := range nested {
						if !seen[alias] {
							seen[alias] = true
							aliases = append(aliases, alias)
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return aliases, nil
}

func splitDirective(line string) (keyword string, args []string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, false
	}
	line = strings.ReplaceAll(line, "=", " ")
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", nil, false
	}
	return fields[0], fields[1:], true
}

func isWildcardPattern(pattern string) bool {
	if strings.HasPrefix(pattern, "!") {
		return true
	}
	return strings.ContainsAny(pattern, "*?")
}

func resolveIncludeGlob(directive string) ([]string, error) {
	path := expandHome(directive)
	if !filepath.IsAbs(path) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".ssh", path)
	}
	return filepath.Glob(path)
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
