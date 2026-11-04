package main

import "github.com/holistics/anfra/shared/logkit"

// The CLI's and anfra serve's own log events (docs/designs/logs.md).
var (
	eventCommandFailed = logkit.DefineEvent("command.failed",
		"A command run here failed: its code, its scope and its whole chain, at its scope's level.",
		"command", "code", "scope", "internal_code", "error")
	eventTelemetryFailed = logkit.DefineEvent("telemetry.failed",
		"OpenTelemetry failed, such as an export to a collector that is down.",
		"error")

	eventServeListening = logkit.DefineEvent("serve.listening",
		"anfra serve is listening.",
		"url")
	eventServeExposed = logkit.DefineEvent("serve.exposed",
		"anfra serve listens beyond loopback, with no authentication.",
		"addr", "hint")
	eventServeNoAppPages = logkit.DefineEvent("serve.no_app_pages",
		"This anfra was built without its Data App pages.",
		"hint")
	eventServeUpdateAvailable = logkit.DefineEvent("serve.update_available",
		"A newer anfra release is out.",
		"latest", "current", "notice")

	eventPprofListening = logkit.DefineEvent("pprof.listening",
		"The profiler (ANFRA_PPROF_ADDR) is listening.",
		"addr")
	eventPprofExposed = logkit.DefineEvent("pprof.exposed",
		"The profiler listens beyond loopback, where anyone who reaches it can read anfra's memory.",
		"addr", "hint")
)
