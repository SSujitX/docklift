package contract

// targetIsGo reports whether the suite is running against the Go backend.
// Use it for the few behaviours the Go backend fixes on purpose; every other
// expectation must hold for both backends.
func targetIsGo() bool { return cfg.target == "go" }

// invalidLogService is the error for an unknown system log stream. The Go
// stack has three containers; 2.x also had the frontend and gateway nginx.
func invalidLogService() string {
	if targetIsGo() {
		return "Invalid service name. Available: backend, proxy, certbot"
	}
	return "Invalid service name. Available: backend, frontend, proxy, nginx, certbot"
}

// logServices are the system log streams the target serves.
func logServices() []string {
	if targetIsGo() {
		return []string{"backend", "proxy", "certbot"}
	}
	return []string{"backend", "frontend", "proxy", "nginx", "certbot"}
}
