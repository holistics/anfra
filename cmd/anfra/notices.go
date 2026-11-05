package main

import (
	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/update"
)

// A notice tells a person about the environment anfra runs in, not about the
// command they ran: something that holds until they deal with it, and that a
// command deals with. It's for a person, so never in a program's output, and
// never a reason for a command to fail.
//
// Whether something is a notice: it would be worth saying after any command,
// the same whatever the command was, and running one command settles it. A
// problem with the command being run is that command's warning instead
// (printWarning); one with the console's next entry, the console's warning
// (consoleWarning); a state, as the console's server, isn't something to deal
// with at all.
//
// The notices are one list, which each place shows its own way, so a notice
// added to it shows everywhere at once: after a command, outside a console,
// each in full; in a console, the most important as a badge under the prompt,
// and each in full in \?.
type notice struct {
	// Its badge, as short as one: a mark ("↑" for behind, "!" for needing
	// attention) and a few words.
	mark, label string
	// The sentence said after a command, a command in it as `anfra <command>`,
	// as it's typed at a shell.
	text string
	// The command that settles it, as typed in a console.
	command string

	// How it's shown after a command, where a notice interrupts the output:
	skills bool // at most once a day, as the agents record it (agents.NoticeDue): true often enough that every command would nag
	update bool // anfra's own update, which opt-in auto-update acts on rather than says
}

// notices are what holds now, the most important first. They're asked for
// after every command, so each is worked out from local state only, never the
// network: what a background check last saw. None when the person turned them
// off (ANFRA_NO_UPDATE_NOTIFIER).
func notices() []notice {
	if updateNotifyDisabled() {
		return nil
	}
	var ns []notice
	if r := update.Cached(); r != nil && r.IsNewer() {
		ns = append(ns, notice{mark: "↑", label: "anfra " + r.Version, text: update.Notice(r), command: "update", update: true})
	}
	env, repoDir := agents.System(), repoDirIfAny()
	if text := agents.Attention(env, repoDir); text != "" {
		mark, label := agents.Badge(env, repoDir)
		ns = append(ns, notice{mark: mark, label: label, text: text, command: "agent setup", skills: true})
	}
	return ns
}
