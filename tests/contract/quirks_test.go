package contract

// invalidLogService is the error for an unknown system log stream. The stack
// has three containers: backend, nginx-proxy and certbot.
func invalidLogService() string {
	return "Invalid service name. Available: backend, proxy, certbot"
}

// logServices are the system log streams the server provides.
func logServices() []string {
	return []string{"backend", "proxy", "certbot"}
}
