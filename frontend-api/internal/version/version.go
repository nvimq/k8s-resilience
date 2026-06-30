package version

import "runtime"

var (
	Version   = "dev"
	GitCommit = "none"
	BuildTime = "unknown"
)

func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"git_commit": GitCommit,
		"build_time": BuildTime,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
}
