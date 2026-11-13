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
	eventServeQueryCacheUnavailable = logkit.DefineEvent("serve.query_cache_unavailable",
		"anfra serve couldn't open the query cache's store at startup, so queries run without the cache.",
		"store", "error", "hint")

	eventQueryCacheOrphanedFiles = logkit.DefineEvent("query_cache.orphaned_files",
		"A new query cache store's folder already holds files from an earlier cache, which no entry names (orphans).",
		"dir", "hint")
	eventQueryCacheSwept = logkit.DefineEvent("query_cache.swept",
		"anfra serve's sweep removed the query cache's expired entries, and their files.",
		"entries", "bytes", "took")
	eventQueryCacheSweepFailed = logkit.DefineEvent("query_cache.sweep_failed",
		"anfra serve's sweep of the query cache failed; the next one tries again.",
		"error", "hint")
	eventQueryCacheUnavailable = logkit.DefineEvent("query_cache.unavailable",
		"A query ran without the query cache, because the cache failed: its store, its canal-lake, or a write.",
		"error", "hint")
	eventQueryCacheSettingsUnknown = logkit.DefineEvent("query_cache.settings_unknown",
		"The repo's settings file couldn't be read or parsed, so the query cache's settings are unknown, and it keeps its defaults.",
		"path", "error", "hint")

	eventPprofListening = logkit.DefineEvent("pprof.listening",
		"The profiler (ANFRA_PPROF_ADDR) is listening.",
		"addr")
	eventPprofExposed = logkit.DefineEvent("pprof.exposed",
		"The profiler listens beyond loopback, where anyone who reaches it can read anfra's memory.",
		"addr", "hint")
)
