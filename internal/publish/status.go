package publish

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/publish/syncstate"
)

// StatusResult is `publish status`'s result: how this folder stands against
// what its org has published.
type StatusResult struct {
	Message     string       `json:"message" doc:"the outcome, in a sentence"`
	Org         *OrgRef      `json:"org,omitempty" doc:"the repo's org. Absent when none is configured."`
	Revision    string       `json:"revision,omitempty" doc:"the org's published revision"`
	PublishedBy string       `json:"published_by,omitempty" doc:"who published that revision. Absent for the org's starting files."`
	PublishedAt *time.Time   `json:"published_at,omitempty" doc:"when that revision was published" pretty:",moment"`
	Base        string       `json:"base,omitempty" doc:"the org's revision this folder last pulled or published. Absent when anfra has no record of one."`
	Incoming    []FileChange `json:"incoming,omitempty" doc:"what the org changed since the base: what a pull would change in this folder"`
	Unpublished []FileChange `json:"unpublished,omitempty" doc:"what this folder changed since the base, that the org doesn't have: what a publish would send"`
	// PullInterrupted is a pull stopped partway through writing files: the
	// folder is half pulled, so it isn't compared.
	PullInterrupted bool                      `json:"pull_interrupted,omitzero" doc:"true when the last pull stopped partway through writing files. The folder isn't compared until it's undone."`
	Conflicts       map[string][]PullConflict `json:"conflicts,omitempty" doc:"the last pull's conflicts not resolved yet, by kind"`
	HowToResolve    map[string]string         `json:"how_to_resolve,omitempty" doc:"how to resolve each kind of conflict left, by kind"`
	Hints           []string                  `json:"hints,omitempty" doc:"what to do next, if any"`
}

// Status says how the folder at dir stands against its org's published
// revision: what a pull would bring in, what a publish would send, and the
// last pull's conflicts. It changes nothing, here or on the org.
func Status(ctx context.Context, dir string, env Env) (StatusResult, error) {
	org, configured, err := readOrg(dir)
	if err != nil {
		return StatusResult{}, err
	}
	if !configured {
		return StatusResult{
			Message: "This repo has no org, so there's nothing published to compare with.",
			Hints:   []string{"Set the org with `anfra publish --set-org https://<server>/o/<org>`, or run `anfra pull` on a terminal to choose one."},
		}, nil
	}
	state, err := syncstate.For(dir).Load()
	if err != nil {
		return StatusResult{}, err
	}
	s := state[org.String()]
	ref := orgRef(env, org)
	res := StatusResult{Org: &ref}
	if s != nil && s.Pending != nil && s.Pending.Stage == syncstate.Writing {
		res.PullInterrupted = true
		res.Message = "The last pull stopped partway through writing files."
		res.Hints = []string{"Run `anfra pull --abort` to put them back as they were, then pull again."}
		return res, nil
	}
	local, err := readFolder(dir, verbStatus)
	if err != nil {
		return StatusResult{}, err
	}
	// No org selection or saving here: the configured org, its login checked.
	_, client, err := connect(ctx, dir, destination{org: org, configured: true}, env, cliui.NewPrinter(env.ErrOut), verbStatus)
	if err != nil {
		return StatusResult{}, err
	}
	status, err := client.Status(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	res.Revision = status.Revision
	if status.PublishedBy != nil {
		res.PublishedBy = status.PublishedBy.Email
	}
	if !status.PublishedAt.IsZero() {
		at := status.PublishedAt
		res.PublishedAt = &at
	}
	// A pull with conflicts brought its revision in: compare from it.
	base := ""
	if s != nil {
		base = s.Base
		if s.Pending != nil {
			base = s.Pending.Revision
		}
	}
	c, err := compare(ctx, client, status, base, local)
	if err != nil {
		return StatusResult{}, err
	}
	res.Base = c.base
	res.Incoming, res.Unpublished = c.incoming(), c.outgoing()
	open := openConflicts(dir, s)
	if len(open) > 0 {
		res.Conflicts, res.HowToResolve = describe(open, s.Base == "")
	}
	res.Message, res.Hints = statusText(res, ref, open)
	return res, nil
}

// statusText is a status's message and hints.
func statusText(res StatusResult, ref OrgRef, open []syncstate.Conflict) (string, []string) {
	var msg []string
	var hints []string
	in, out := len(res.Incoming), len(res.Unpublished)
	switch {
	case in == 0 && out == 0:
		msg = append(msg, fmt.Sprintf("Everything here is published to %s, and this folder has everything published there.", ref.shown()))
	case in > 0 && res.Base != "" && res.PublishedBy != "":
		msg = append(msg, fmt.Sprintf("%s published to %s on %s, after this folder last pulled or published.", res.PublishedBy, ref.shown(), res.PublishedAt.Local().Format("2006-01-02 15:04")))
	case in > 0:
		msg = append(msg, fmt.Sprintf("anfra has no record of this folder pulling from or publishing to %s, and the org has files this folder doesn't have, or has differently.", ref.shown()))
	default:
		msg = append(msg, fmt.Sprintf("This folder has everything published to %s.", ref.shown()))
	}
	if out > 0 {
		msg = append(msg, fmt.Sprintf("%d %s here %s published yet.", out, pick(out, "file", "files"), pick(out, "isn't", "aren't")))
	}
	if len(open) > 0 {
		msg = append(msg, fmt.Sprintf("%s from the last pull %s left to resolve.", countConflicts(open), isAre(len(open))))
	}

	if in > 0 {
		if out > 0 {
			hints = append(hints, "Run `anfra publish --pull` to merge the org's changes and publish yours, or `anfra pull` to review theirs first.")
		} else {
			hints = append(hints, "Run `anfra pull` to bring the org's changes into this folder.")
		}
		var both []string
		for _, c := range res.Incoming {
			if c.Change == "merged" || c.Change == "conflict" {
				both = append(both, c.Path)
			}
		}
		if len(both) > 0 {
			hints = append(hints, fmt.Sprintf("%s changed both here and on the org, so a pull merges %s, and may leave conflicts to resolve.", joinPaths(both), itThem(len(both))))
		}
	}
	if len(open) > 0 {
		hints = append(hints, conflictHints("the last pull")...)
	} else if out > 0 && in == 0 {
		hints = append(hints, "Run `anfra publish` to publish them.")
	}
	return strings.Join(msg, " "), hints
}
