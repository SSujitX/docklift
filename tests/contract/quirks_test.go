package contract

// targetIsGo reports whether the suite is running against the Go backend.
// Use it for the few behaviours the Go backend fixes on purpose; every other
// expectation must hold for both backends.
func targetIsGo() bool { return cfg.target == "go" }
