// Package task is the task commands: a coding agent marks, in anfra's trace,
// its work on one of its user's messages, or on a part of it, as a task. Its
// start gives the task an id, which the agent passes on every command it runs
// for the task (the CLI's --task, MCP's task argument), so a task's commands
// are found by the id, never by when they ran. A subagent starts its own task
// under its caller's, so its commands stay apart from its caller's even when
// they run at once. See docs/designs/telemetry_local.md, tasks.
//
// anfra keeps no task between commands. A task is the commands that carry its
// id, so it has no end and no outcome, and nothing is ever left open.
package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/telemetry"
	"go.opentelemetry.io/otel/trace"
)

// Flag is the name of the argument that names the task a command is run for:
// the CLI's --task on every command, and MCP's task argument on every tool.
const Flag = "task"

// FlagUsage is what Flag is, for the CLI's --help and MCP's tool schemas.
// No backquotes: the CLI's help would take the quoted text as the value's name.
const FlagUsage = "the id of the task you run this command for, which anfra experimental telemetry task start printed, " +
	"so the command is recorded in that task"

// maxText bounds a goal or a note, in bytes: a user's message or a short
// paragraph, not a transcript.
const maxText = 4 << 10

// ValidID reports whether id is a task's id, as newID makes it: t_ and 12
// lowercase hex digits. Only such an id is recorded, so a task's id holds
// nothing of the user's.
func ValidID(id string) bool { return telemetry.ValidTaskID(id) }

// InvalidIDMessage says what to pass in place of a malformed task id.
func InvalidIDMessage(id string) string {
	return fmt.Sprintf("%q isn't a task's id. Pass the id `anfra experimental telemetry task start` printed, such as t_7f3a91c0d2e4.", id)
}

// newID is a fresh task's id: t_ and 48 random bits in lowercase hex, enough
// to tell apart the tasks of one session.
func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	return "t_" + hex.EncodeToString(b[:])
}

// offMessage says that what is named wasn't recorded, because telemetry is
// off, so the agent stops marking tasks.
func offMessage(what string) string {
	return "Telemetry is off, so " + what + " wasn't recorded. Skip tasks, and --task, for the rest of your session. " +
		"`anfra experimental telemetry --help` says how to turn telemetry on."
}

// Start starts a task.
var Start = command.Define(command.Def[StartInput, StartResult]{
	Name:  "experimental.telemetry.task.start",
	Short: "Start a task for a message from your user that asks for data, and get the task's id to pass on every command",
	Long: "Start a task for a message from your user that asks for data, and get the task's id to pass on every command. " +
		"Pass the id as --task <id> on every anfra command you run for that message, so anfra records the commands in the task " +
		"and your work can be evaluated per message.\n\n" +
		"Start one task per message from your user, before your first anfra command for it, with the message as the goal. " +
		"A message with several questions is still one task. You can split it into subtasks with --parent.\n\n" +
		"If another agent gave you a task's id, you are its subagent. Start a task of your own with --parent <that id>, " +
		"and pass your own task's id on your commands, so your commands stay apart from the other agent's.\n\n" +
		"A task has no end. It is the commands that carry its id. " +
		"Its goal is your user's data, which isn't recorded when " + telemetry.DataVar + "=0. " +
		"A task is recorded only while telemetry is on, which `anfra status` shows.",
	NonDestructive: true,
	Normalize: func(in StartInput) StartInput {
		in.Goal = strings.TrimSpace(in.Goal)
		in.Parent = strings.TrimSpace(in.Parent)
		return in
	},
	Check: func(in StartInput) error {
		if in.Parent != "" && !ValidID(in.Parent) {
			return command.InvalidArg("parent", "invalid", InvalidIDMessage(in.Parent))
		}
		return checkText("goal", in.Goal)
	},
	Run: start,
})

// start gives a new task its id, and records its start on the command's span.
func start(ctx context.Context, _ command.CommandContext, in StartInput) (StartResult, error) {
	id := newID()
	if !trace.SpanFromContext(ctx).IsRecording() {
		return StartResult{Task: id, Message: offMessage("the task")}, nil
	}
	attrs := []telemetry.KeyValue{telemetry.TaskID.Is(id), telemetry.TaskGoal.Is(in.Goal)}
	if in.Parent != "" {
		attrs = append(attrs, telemetry.TaskParentID.Is(in.Parent))
	}
	apptracing.SetAttributes(ctx, attrs...)
	return StartResult{
		Task:     id,
		Recorded: true,
		Message:  fmt.Sprintf("Started task %s. Pass --task %s on every anfra command you run for it.", id, id),
	}, nil
}

// StartInput is a task's goal, and the task it is part of.
type StartInput struct {
	Goal   string `json:"goal" cli:"positional" doc:"your user's message, in their words, or for a subtask the part of it you work on. At most 4 KiB."`
	Parent string `json:"parent,omitempty" doc:"the id of the task this one is part of: the task another agent gave you when you are its subagent, or your own when you split a message into subtasks"`
}

func (StartInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[StartInput](s)
}

// StartResult is the new task's id.
type StartResult struct {
	Task     string `json:"task" doc:"the task's id. Pass it as --task on every anfra command you run for the task."`
	Recorded bool   `json:"recorded" doc:"whether the task's start is in the trace. False while telemetry is off, when nothing about the task is recorded."`
	Message  string `json:"message" doc:"the outcome, in a sentence"`
}

// Note records, in a task, what the agent found, a change of approach, or its
// answer.
var Note = command.Define(command.Def[NoteInput, NoteResult]{
	Name:  "experimental.telemetry.task.note",
	Short: "Record in a task what you found, a change of approach, or your answer, for your user to read later",
	Long: "Record in a task what you found, a change of approach, or your answer, for your user to read later. " +
		"Pass the task's id as --task, as on every command you run for the task.\n\n" +
		"--kind says what the note is:\n" +
		"  - finding: something you learned, such as a field that doesn't mean what its name says\n" +
		"  - pivot: a change of approach, such as why you are about to write your own SQL\n" +
		"  - progress: where you are, in a long task\n" +
		"  - summary: your answer to the task's goal, and what you are unsure of\n\n" +
		"Notes are for people to read. Evaluating your work never depends on them. " +
		"A note is your user's data, which isn't recorded when " + telemetry.DataVar + "=0.",
	NonDestructive: true,
	Normalize: func(in NoteInput) NoteInput {
		in.Text = strings.TrimSpace(in.Text)
		return in
	},
	Check: func(in NoteInput) error { return checkText("text", in.Text) },
	Run:   note,
})

// note records a note on the command's span, which carries the command's task.
func note(ctx context.Context, _ command.CommandContext, in NoteInput) (NoteResult, error) {
	if !trace.SpanFromContext(ctx).IsRecording() {
		return NoteResult{Message: offMessage("the note")}, nil
	}
	apptracing.SetAttributes(ctx, telemetry.TaskNoteKind.Is(in.Kind), telemetry.TaskNoteText.Is(in.Text))
	return NoteResult{Recorded: true, Message: fmt.Sprintf("Recorded the %s note.", in.Kind)}, nil
}

// NoteInput is a note's kind and text. Its task is the command's (Flag).
type NoteInput struct {
	Kind string `json:"kind" enum:"finding,pivot,progress,summary" doc:"what the note is: finding, pivot, progress, or summary"`
	Text string `json:"text" cli:"positional" doc:"the note, in a sentence or a short paragraph. At most 4 KiB."`
}

func (NoteInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[NoteInput](s)
}

// NoteResult says whether the note was recorded.
type NoteResult struct {
	Recorded bool   `json:"recorded" doc:"whether the note is in the trace. False while telemetry is off, when the note is lost."`
	Message  string `json:"message" doc:"the outcome, in a sentence"`
}

func checkText(field, text string) error {
	if text == "" {
		return command.InvalidArg(field, "required", fmt.Sprintf("The %s is empty. Give its text.", field))
	}
	if len(text) > maxText {
		return command.InvalidArg(field, "too_long", fmt.Sprintf("The %s is %d bytes, over the %d it can hold. Shorten it to a message or a short paragraph.", field, len(text), maxText))
	}
	if !utf8.ValidString(text) {
		return command.InvalidArg(field, "invalid", fmt.Sprintf("The %s isn't valid UTF-8.", field))
	}
	return nil
}
