package version

// Variables set via ldflags at build time
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// Info returns version information
func Info() string {
	return Version
}

// Full returns full version information including commit and build date
func Full() string {
	return Version + " (" + GitCommit + ") built " + BuildDate
}
