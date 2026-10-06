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

// noa Yes norma v0.4.0 Introduction[Model driven context compression]Mechanisms,As a platform experiment feature, user-driven
// system settings. It's with the interior. compaction Crust.:noaadapter.Enable The only way in.,Hang up once.
// Context receiver(Compactor),Compress Tools & Three Permanent Notes,Do Not Call Enable Close
// (Built-in compaction Work as usual.).Switches by each agent Injecting. noaEnabledFn Analysis,each run Read
// Once.,So switch only to start after run,No need to rebuild. agent.

// enableNoa When the parser report opens noa Access opts.archiveRoot It's a condensed sustainable base directory.
// (Take Global workDir,Each agent Unanimous <workDir>/noa Down,Do not follow the mission/Intentional directory dispersed),sessionID
// Naming its lower archive subdirectories(One in the world.,So there's no conflict in the same base directory.).
//
// noa It's an experimental function.:Access failure must not interrupt the real task. When an error occurs onWarn Report and reverse internal compression.
// Clear when enabled opts.Compaction,Avoid agentcore Because[Both context manager settings]Police!.
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
			onWarn("noa Compression enabled failed,Revert internal compression:" + err.Error())
		}
		return
	}
	// Compactor override Compaction,But when they're together, agentcore Every time, they call the police.;Clear..
	opts.Compaction = nil
}
