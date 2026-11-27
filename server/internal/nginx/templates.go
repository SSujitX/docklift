// Package nginx is the port of backend/src/services/nginx.ts, nginxSsl.ts and
// lib/domainOwnership.ts: it writes the nginx-proxy vhosts for service
// domains and panel domains (ACME challenge path plus optional HTTPS from
// Let's Encrypt), reloads nginx-proxy through `docker exec`, keeps the conf
// directory in sync with the database and enforces that a hostname belongs
// to one mapping.
//
// Generated files are byte-identical to the TypeScript output (existing
// installs keep their files, and markers such as "# docklift-ssl:" and
// "# docklift-panel-domain" are parsed back out of them), with one deliberate
// change: a panel domain for the dashboard port proxies to the Go backend
// (PanelUpstreamHost:<PORT>) instead of the removed docklift-nginx gateway.
// MigratePanelUpstreams rewrites existing panel confs once at boot.
package nginx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"docklift/internal/certs"
)

// PanelConfMarker is the first line of every panel-domain conf.
const PanelConfMarker = "# docklift-panel-domain"

// DashboardPort is the panel port that maps to the dashboard container
// rather than host.docker.internal.
const DashboardPort = 8080

// BackendContainer is the container name of the Go backend.
const BackendContainer = "docklift-backend"

// BackendNetwork is the control-plane network the backend and nginx-proxy
// share (docker-compose.yml networks.docklift_network.name).
const BackendNetwork = "docklift_network"

// PanelUpstreamHost is the name the panel conf resolves the backend by. It
// is network-qualified: nginx-proxy also joins every project network, where
// a project's service keys are DNS aliases, and Docker's embedded DNS
// resolves "<name>.<network>" by looking up <name> only among endpoints of
// <network> (Sandbox.ResolveName in moby's libnetwork; a whole-name match in
// any network is tried first, which is why deploys reject service names
// starting with "docklift").
const PanelUpstreamHost = BackendContainer + "." + BackendNetwork

const acmeInclude = "include /etc/nginx/snippets/acme.conf;"

var (
	serviceIDPattern     = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`)
)

// InvalidInputError reports a value that cannot be placed in an nginx conf.
type InvalidInputError struct {
	Field string
	Value string
}

func (e *InvalidInputError) Error() string {
	return fmt.Sprintf("invalid %s for nginx config: %q", e.Field, e.Value)
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

// BuildServiceProxyLocation is the `location /` block proxying to a deployed
// service container through Docker DNS (a variable, so nginx starts even when
// the container is down). The id, container name and port are validated so
// nothing but the expected tokens reaches the conf.
func BuildServiceProxyLocation(serviceID, containerName string, internalPort int) (string, error) {
	if !serviceIDPattern.MatchString(serviceID) {
		return "", &InvalidInputError{"service id", serviceID}
	}
	if !containerNamePattern.MatchString(containerName) {
		return "", &InvalidInputError{"container name", containerName}
	}
	if !validPort(internalPort) {
		return "", &InvalidInputError{"internal port", strconv.Itoa(internalPort)}
	}
	v := "target_" + strings.ReplaceAll(serviceID, "-", "_")
	return `
    location / {
        set $` + v + ` ` + containerName + `;
        proxy_pass http://$` + v + `:` + strconv.Itoa(internalPort) + `;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }`, nil
}

// dashboardLocation is the panel upstream for DashboardPort. Only the two
// upstream lines differ from the TypeScript template (which pointed at
// docklift-nginx:80); MigratePanelUpstreams relies on that.
func dashboardLocation(backendPort int) string {
	return `
    location / {
        set $dashboard ` + PanelUpstreamHost + `;
        proxy_pass http://$dashboard:` + strconv.Itoa(backendPort) + `;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
        proxy_buffering off;
    }`
}

// BuildPanelProxyLocation is the `location /` block of a panel domain: the Go
// backend (PanelUpstreamHost:backendPort) for the dashboard port 8080, else
// host.docker.internal:<port>.
func BuildPanelProxyLocation(port, backendPort int) (string, error) {
	if !validPort(port) {
		return "", &InvalidInputError{"panel port", strconv.Itoa(port)}
	}
	if port == DashboardPort {
		if !validPort(backendPort) {
			return "", &InvalidInputError{"backend port", strconv.Itoa(backendPort)}
		}
		return dashboardLocation(backendPort), nil
	}
	return `
    location / {
        proxy_pass http://host.docker.internal:` + strconv.Itoa(port) + `;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }`, nil
}

// CertIndex answers which Let's Encrypt lineage serves a host set;
// *certs.Service satisfies it.
type CertIndex interface {
	FindCoveringCertName(hosts []string) string
	FindBestExpandableCertName(hosts []string) string
	ListCertDNSNames(certName string) []string
}

// ServerBlocks describes the vhost of one mapping.
type ServerBlocks struct {
	// Comment is the first comment line (newlines are replaced by spaces).
	Comment string
	// ServerNames is the space-separated server_name list.
	ServerNames string
	// PrimaryDomain names the lineage when no certificate exists yet.
	PrimaryDomain string
	// ProxyLocation is a block from BuildServiceProxyLocation or
	// BuildPanelProxyLocation.
	ProxyLocation string
	// EnableHTTPS allows the :443 server when a usable lineage exists.
	EnableHTTPS bool
}

// sanitizeComment keeps the comment on its line: nginx comments end at a
// newline, so a line break in a service name would otherwise inject
// directives.
func sanitizeComment(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return ' '
		}
		return r
	}, s)
}

// checkNames validates server names and the primary domain against
// DOMAIN_REGEX; names must be separated by single spaces.
func checkNames(serverNames, primary string) error {
	fields := strings.Fields(serverNames)
	if len(fields) == 0 || strings.Join(fields, " ") != serverNames {
		return &InvalidInputError{"server_name", serverNames}
	}
	for _, f := range fields {
		if !certs.IsValidHostname(f) {
			return &InvalidInputError{"server_name", f}
		}
	}
	if !certs.IsValidHostname(primary) {
		return &InvalidInputError{"primary domain", primary}
	}
	return nil
}

// BuildHTTPHTTPSServers renders the vhost. With a lineage covering every
// name: :80 serves ACME and redirects to HTTPS, :443 serves all names
// ("# docklift-ssl: active"). With a lineage covering only some names (e.g.
// while a SAN is being added): :80 keeps proxying so HSTS hosts are not
// downgraded, :443 serves only the names on the certificate
// ("# docklift-ssl: partial"). Without one: :80 only ("pending|http-only").
func BuildHTTPHTTPSServers(idx CertIndex, b ServerBlocks) (string, error) {
	if err := checkNames(b.ServerNames, b.PrimaryDomain); err != nil {
		return "", err
	}
	comment := sanitizeComment(b.Comment)
	hosts := make([]string, 0)
	for _, f := range strings.Fields(b.ServerNames) {
		if h := strings.ToLower(f); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		hosts = []string{strings.ToLower(b.PrimaryDomain)}
	}

	fullCover, certName := "", ""
	if b.EnableHTTPS {
		fullCover = idx.FindCoveringCertName(hosts)
		certName = fullCover
		if certName == "" {
			certName = idx.FindBestExpandableCertName(hosts)
		}
	}
	if certName != "" && !certs.IsValidCertName(certName) {
		return "", &InvalidInputError{"certificate name", certName}
	}

	if certName == "" {
		return `
# ` + comment + `
# docklift-ssl: pending|http-only primary=` + b.PrimaryDomain + `
server {
    listen 80;
    server_name ` + b.ServerNames + `;

    resolver 127.0.0.11 valid=30s ipv6=off;

    ` + acmeInclude + `
` + b.ProxyLocation + `
}
`, nil
	}

	fullchain, privkey := certs.NginxCertPaths(certName)
	if fullCover == "" {
		// :443 only for names actually on the chosen cert — never present the
		// wrong PEMs to HSTS clients.
		onCert := idx.ListCertDNSNames(certName)
		var https []string
		for _, h := range hosts {
			for _, n := range onCert {
				if n == h {
					https = append(https, h)
					break
				}
			}
		}
		httpsNames := strings.Join(https, " ")
		if httpsNames == "" {
			httpsNames = b.PrimaryDomain
		}
		return `
# ` + comment + `
# docklift-ssl: partial primary=` + b.PrimaryDomain + ` cert=` + certName + `
server {
    listen 80;
    server_name ` + b.ServerNames + `;

    resolver 127.0.0.11 valid=30s ipv6=off;

    ` + acmeInclude + `
` + b.ProxyLocation + `
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ` + httpsNames + `;

    ssl_certificate ` + fullchain + `;
    ssl_certificate_key ` + privkey + `;

    resolver 127.0.0.11 valid=30s ipv6=off;
` + b.ProxyLocation + `
}
`, nil
	}

	return `
# ` + comment + `
# docklift-ssl: active primary=` + b.PrimaryDomain + ` cert=` + certName + `
server {
    listen 80;
    server_name ` + b.ServerNames + `;

    ` + acmeInclude + `

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ` + b.ServerNames + `;

    ssl_certificate ` + fullchain + `;
    ssl_certificate_key ` + privkey + `;

    resolver 127.0.0.11 valid=30s ipv6=off;
` + b.ProxyLocation + `
}
`, nil
}

// BuildPanelConf renders a complete panel-domain conf: the marker, the port
// line and the vhost.
func BuildPanelConf(idx CertIndex, domain string, port, backendPort int, enableHTTPS bool) (string, error) {
	loc, err := BuildPanelProxyLocation(port, backendPort)
	if err != nil {
		return "", err
	}
	body, err := BuildHTTPHTTPSServers(idx, ServerBlocks{
		Comment:       "Panel domain " + domain,
		ServerNames:   domain,
		PrimaryDomain: domain,
		ProxyLocation: loc,
		EnableHTTPS:   enableHTTPS,
	})
	if err != nil {
		return "", err
	}
	return PanelConfMarker + "\n# docklift-panel-port: " + strconv.Itoa(port) + "\n" + body, nil
}
