//go:build apps_e2e

package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Fake answers every command from canned responses, so the Shell's e2e tests run `anfra serve
// --apps` without sidecars or a database. It is configured by environment:
//
//	ANFRA_E2E_LOG        file to append one JSON line per event to (startup, each call)
//	ANFRA_E2E_RESPONSES  JSON file of answers, keyed "<command>" or, for search,
//	                      "search:<query>". An answer is the `data` to return, or
//	                      { "__error": "<message>" } to fail the call, or
//	                      { "__status": "invalid", "__data": <data> } for an invalid-status answer.
//	                      Any answer may carry "__delayMs" to answer late; a caller that hangs up
//	                      first is logged as an "aborted" event. A "status" answer with "__error"
//	                      makes the sidecars unhealthy.
func Fake() Caller {
	f := &fake{responses: os.Getenv("ANFRA_E2E_RESPONSES"), log: os.Getenv("ANFRA_E2E_LOG")}
	cwd, _ := os.Getwd()
	f.record(map[string]any{"event": "start", "pid": os.Getpid(), "cwd": cwd})
	return f
}

type fake struct {
	responses string
	log       string
	mu        sync.Mutex
}

func (f *fake) record(event map[string]any) {
	if f.log == "" {
		return
	}
	line, _ := json.Marshal(event)
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.OpenFile(f.log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.Write(append(line, '\n'))
}

// answer is re-read on every call, so a test can change what anfra answers mid-run.
func (f *fake) answer(key, command string) (json.RawMessage, bool) {
	all := map[string]json.RawMessage{}
	if f.responses != "" {
		raw, err := os.ReadFile(f.responses)
		if err == nil {
			_ = json.Unmarshal(raw, &all)
		}
	}
	if a, ok := all[key]; ok {
		return a, true
	}
	a, ok := all[command]
	return a, ok
}

func (f *fake) Call(ctx context.Context, command string, args map[string]any) (string, json.RawMessage, error) {
	f.record(map[string]any{"event": "call", "command": command, "args": args})
	key := command
	if command == "search" {
		key = fmt.Sprintf("search:%v", args["query"])
	}
	raw, ok := f.answer(key, command)
	if !ok {
		return "", nil, &CallError{Message: fmt.Sprintf("fake anfra: no canned response for %q", key)}
	}
	var special struct {
		Error   *string         `json:"__error"`
		Status  string          `json:"__status"`
		Data    json.RawMessage `json:"__data"`
		DelayMs int             `json:"__delayMs"`
	}
	isObject := strings.HasPrefix(strings.TrimSpace(string(raw)), "{")
	if isObject {
		_ = json.Unmarshal(raw, &special)
	}
	if special.DelayMs > 0 {
		select {
		case <-ctx.Done():
			f.record(map[string]any{"event": "aborted", "command": command})
			return "", nil, ctx.Err()
		case <-time.After(time.Duration(special.DelayMs) * time.Millisecond):
		}
	}
	switch {
	case special.Error != nil:
		return "", nil, &CallError{Message: *special.Error}
	case special.Status != "" || special.Data != nil:
		status := special.Status
		if status == "" {
			status = "ok"
		}
		return status, special.Data, nil
	}
	return "ok", raw, nil
}

func (f *fake) Healthy(ctx context.Context) bool {
	raw, ok := f.answer("status", "status")
	if !ok {
		return true
	}
	var special struct {
		Error *string `json:"__error"`
	}
	_ = json.Unmarshal(raw, &special)
	return special.Error == nil
}
