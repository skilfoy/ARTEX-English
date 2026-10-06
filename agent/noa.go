package agent

import (
	"log"
	"path/filepath"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/noaadapter"
)

// noaWarn returns a diagnostics sink tagging non-fatal noa messages with the
// session, routed through the package logger (agents have no per-instance one).
func noaWarn(session string) func(string) {
	return func(msg string) { log.Printf("[noa] %s: %s", session, msg) }
}

// noa is the model-driven context compression introduced in norma v0.4.0. It is a
// platform experiment the user toggles in system settings. It is mutually exclusive
// with built-in compaction: noaadapter.Enable is the only entry point and, once
// called, installs the context takeover (Compactor), the Compress tool, and three
// persistent prompt notes. Not calling Enable leaves it off (built-in compaction
// works as usual). Each agent resolves the switch through its injected noaEnabledFn,
// read once per run, so a toggle only affects runs started afterwards. The agent
// does not need to be rebuilt.

// enableNoa wires noa into opts when the resolver reports it enabled. archiveRoot is
// the durable base directory for the compressed originals (the global workDir; every
// agent stores them under <workDir>/noa rather than scattering them across task or
// intent directories). sessionID names the archive subdirectory under that base
// (globally unique, so there is no clash within the same base directory).
//
// noa is experimental: a failed hookup must not abort the real task. On error, report
// via onWarn and fall back to built-in compression. On success, clear opts.Compaction
// so agentcore does not warn that two context managers are set at once.
func enableNoa(opts *agentcore.Options, enabled func() bool, archiveRoot, sessionID string, onWarn func(string)) {
	if enabled == nil || !enabled() {
		return
	}
	if opts.OnWarn == nil {
		opts.OnWarn = onWarn
	}
	if err := noaadapter.Enable(opts, noaadapter.Options{
		ArchiveBaseDir: filepath.Join(archiveRoot, "noa"),
		SessionID:      sessionID,
		OnWarn:         onWarn,
	}); err != nil {
		if onWarn != nil {
			onWarn("noa compression failed to enable; falling back to built-in compression:" + err.Error())
		}
		return
	}
	// Compactor overrides Compaction, but agentcore warns every time both are set; clear it explicitly.
	opts.Compaction = nil
}
