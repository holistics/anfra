package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestPublishNonInteractiveAndDryRun(t *testing.T) {
	t.Chdir(t.TempDir())
	seedLoginInventory(t, "https://bi.example.com/o/stored")
	t.Setenv("ANFRA_CLOUD_SESSION", "obsolete-hook-must-not-work")
	if err := os.WriteFile("orders.aml", []byte("Model orders {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"publish"}, {"publish", "--yes"},
		{"publish", "--set-org", "https://bi.example.com/o/acme", "--yes"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		var output bytes.Buffer
		cmd.SetIn(strings.NewReader("1\n"))
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("%v succeeded", args)
		}
		if !strings.Contains(err.Error(), "--set-org") && !strings.Contains(err.Error(), "anfra auth login https://bi.example.com/o/acme") {
			t.Fatalf("%v: %v", args, err)
		}
		if _, err := os.Stat(repoconfig.Path(".")); !os.IsNotExist(err) {
			t.Fatalf("failed command changed config: %v", err)
		}
		if strings.Contains(output.String(), "Choose") || strings.Contains(output.String(), "Opening") {
			t.Fatalf("noninteractive prompt: %s", output.String())
		}
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"publish", "--dry-run"})
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "orders.aml") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(repoconfig.Path(".")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote config")
	}
}

func TestAuthCommandsLeaveRepoConfigAlone(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ANFRA_HOME", t.TempDir())
	original := "# destination stays\norg:\n  url: https://bi.example.com/o/acme\nother: kept\n"
	if err := os.Mkdir(".anfra", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoconfig.Path("."), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"auth", "status"}, {"auth", "logout"},
		{"auth", "login", "not-an-address"},
		{"auth", "login", "--server", "https://bi.example.com"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		_ = cmd.Execute()
		got, err := os.ReadFile(repoconfig.Path("."))
		if err != nil || string(got) != original {
			t.Fatalf("%v changed config: %q %v", args, got, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(dir, ".anfra")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"auth", "status"}, {"auth", "logout"}, {"auth", "login", "invalid"}} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		_ = cmd.Execute()
		if _, err := os.Stat(repoconfig.Path(".")); !os.IsNotExist(err) {
			t.Fatalf("%v created config", args)
		}
	}
}

func TestAuthStatusExplainsDamagedCredentials(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("ANFRA_HOME", home)
	path := filepath.Join(home, "credentials.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"must-not-print"`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"auth", "status", "--json"})
	var out, diagnostics bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&diagnostics)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("damaged credentials accepted")
	}
	printError(&diagnostics, err)
	for _, want := range []string{path, "move the file aside", "anfra auth login for each organization", "does not revoke server access"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("missing %q in %s", want, &diagnostics)
		}
	}
	if out.Len() != 0 || strings.Contains(diagnostics.String(), "must-not-print") {
		t.Fatalf("unsafe error output: stdout=%q stderr=%q", &out, &diagnostics)
	}
}

func TestLoginServerAndLogoutDestination(t *testing.T) {
	t.Setenv("ANFRA_HOSTED_ORIGIN", "https://runtime-must-not-override.example.com")
	origin, hint, err := loginServer("https://BI.example.com:443/o/acme")
	if err != nil || origin != "https://bi.example.com" || hint != "acme" {
		t.Fatalf("%s %s %v", origin, hint, err)
	}
	if _, _, err := loginServer(""); err == nil || !strings.Contains(err.Error(), "Provide an org URL") {
		t.Fatalf("runtime default accepted: %v", err)
	}
	dir := t.TempDir()
	if err := repoconfig.SetString(repoconfig.Path(dir), "org", "url", "https://bi.example.com/o/repo"); err != nil {
		t.Fatal(err)
	}
	terminal := newAuthTerminal(strings.NewReader(""), &bytes.Buffer{})
	org, err := logoutOrg(context.Background(), dir, nil, terminal)
	if err != nil || org.Slug != "repo" {
		t.Fatalf("%v %v", org, err)
	}
	org, err = logoutOrg(context.Background(), dir, []string{"https://other.example.com/o/explicit"}, terminal)
	if err != nil || org.Slug != "explicit" {
		t.Fatalf("%v %v", org, err)
	}
	if _, err := logoutOrg(context.Background(), t.TempDir(), nil, terminal); err == nil || !strings.Contains(err.Error(), "anfra auth logout https://<server>/o/<org>") {
		t.Fatal(err)
	}
}

func seedLoginInventory(t *testing.T, addresses ...string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ANFRA_HOME", home)
	entries := map[string]any{}
	for _, address := range addresses {
		org, err := cloudapi.ParseOrg(address)
		if err != nil {
			t.Fatal(err)
		}
		entries[address] = map[string]any{"org": org, "access_token": "must-not-print-access", "refresh_token": "must-not-print-refresh", "pending_refresh": true}
	}
	raw, err := jsonkit.Marshal(map[string]any{"version": 1, "entries": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "credentials.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoginOffersOnlyCloudAndURLEntry(t *testing.T) {
	seedLoginInventory(t, "https://bi.example.com/o/stored")
	for _, tc := range []struct {
		name, keys, want string
	}{
		{name: "cloud", keys: "\r"},
		{name: "organization URL", keys: "\x1b[B\rhttps://other.example.com/o/another\r", want: "https://other.example.com/o/another"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var output bytes.Buffer
			terminal := newAuthTerminal(strings.NewReader(tc.keys), &output)
			terminal.interactive = true
			address, err := terminal.loginAddress(ctx)
			if err != nil || address != tc.want {
				t.Fatalf("login address = %q, %v; want %q", address, err, tc.want)
			}
			if strings.Contains(output.String(), "https://bi.example.com/o/stored") {
				t.Fatal("login displayed a saved authentication")
			}
		})
	}
}

func TestBareAuthNeverSelectsWithoutTerminal(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, addresses := range [][]string{nil, {"https://bi.example.com/o/one"}, {"https://bi.example.com/o/one", "https://other.example.com/o/two"}} {
		t.Run(strings.Join(addresses, "+"), func(t *testing.T) {
			seedLoginInventory(t, addresses...)
			for _, action := range []string{"login", "logout"} {
				for _, flags := range [][]string{nil, {"--json"}, {"--no-interactive"}} {
					cmd := newRootCmd()
					cmd.SetArgs(append([]string{"auth", action}, flags...))
					var out, diagnostics bytes.Buffer
					cmd.SetIn(strings.NewReader("1\n"))
					cmd.SetOut(&out)
					cmd.SetErr(&diagnostics)
					if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "anfra auth "+action) {
						t.Fatalf("%s %v: %v", action, flags, err)
					}
					if out.Len() != 0 || diagnostics.Len() != 0 {
						t.Fatalf("unexpected output: %q %q", out.String(), diagnostics.String())
					}
				}
			}
		})
	}
}

func TestAuthLoginBrowserFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir()) // No browser launcher is installed.
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/api":
			_ = jsonkit.MarshalWrite(w, map[string]any{"resource": origin + "/api", "authorization_servers": []string{origin}, "scopes_supported": []string{"all"}, "bearer_methods_supported": []string{"header"}})
		case "/.well-known/oauth-authorization-server":
			_ = jsonkit.MarshalWrite(w, map[string]any{
				"issuer": origin, "authorization_endpoint": origin + "/oauth/authorize", "token_endpoint": origin + "/oauth/token", "revocation_endpoint": origin + "/oauth/revoke",
				"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"},
				"revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"all"}, "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true,
			})
		case "/oauth/token":
			_ = jsonkit.MarshalWrite(w, map[string]any{
				"access_token": "fallback-access-token", "refresh_token": "fallback-refresh-token", "token_type": "Bearer", "expires_in": 3600, "scope": "all", "resource": origin + "/api",
				"anfra_grant": map[string]any{"id": "11111111-1111-4111-8111-111111111111", "expires_at": time.Now().UTC().Add(time.Hour), "org": map[string]string{"id": "22222222-2222-4222-8222-222222222222", "slug": "selected", "url": origin + "/o/selected"}},
			})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	origin = server.URL
	for _, mode := range []string{"human", "json"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ANFRA_HOME", t.TempDir())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			observed := make(chan error, 1)
			go func() {
				defer reader.Close()
				var address string
				scanner := bufio.NewScanner(reader)
				for scanner.Scan() {
					line := scanner.Text()
					if raw, ok := strings.CutPrefix(line, "If the browser didn't open, visit: "); ok {
						address = raw
					}
					if !strings.HasPrefix(line, "Could not open your browser.") {
						continue
					}
					authorization, err := url.Parse(address)
					if err != nil || authorization.Path != "/oauth/authorize" || authorization.Query().Get("code_challenge_method") != "S256" {
						observed <- fmt.Errorf("invalid fallback authorization URL")
						cancel()
						return
					}
					query := authorization.Query()
					callback := query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "iss": {origin}, "code": {"manual-callback-code"}}.Encode()
					request, err := http.NewRequestWithContext(ctx, http.MethodGet, callback, nil)
					if err == nil {
						var response *http.Response
						response, err = http.DefaultClient.Do(request)
						if err == nil {
							response.Body.Close()
							if response.StatusCode != http.StatusOK {
								err = fmt.Errorf("manual callback status: %d", response.StatusCode)
							}
						}
					}
					observed <- err
				}
			}()
			cmd := newRootCmd()
			args := []string{"auth", "login", origin + "/o/hint", "--no-interactive"}
			if mode == "json" {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.MultiWriter(&diagnostics, writer))
			err := cmd.ExecuteContext(ctx)
			if mode == "json" {
				if err == nil || !strings.Contains(err.Error(), "could not open your browser") || out.Len() != 0 || diagnostics.Len() != 0 {
					t.Fatalf("JSON launcher failure: err=%v stdout=%q stderr=%q", err, out.String(), diagnostics.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("manual browser login failed: %v", err)
			}
			if callbackErr := <-observed; callbackErr != nil {
				t.Fatal(callbackErr)
			}
			if out.String() != "Authenticated with "+origin+"/o/selected.\n" || strings.Contains(diagnostics.String(), "fallback-access-token") || strings.Contains(diagnostics.String(), "fallback-refresh-token") || strings.Contains(diagnostics.String(), "manual-callback-code") {
				t.Fatalf("incorrect login output: stdout=%q stderr=%q", out.String(), diagnostics.String())
			}
		})
	}
}

func TestAuthStatusIsLocalInventoryWithoutTokens(t *testing.T) {
	t.Chdir(t.TempDir())
	seedLoginInventory(t, "https://bi.example.com/o/one", "https://other.example.com/o/two")
	cmd := newRootCmd()
	cmd.SetArgs([]string{"auth", "status"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.HasPrefix(text, "Authenticated organizations\n\n") || strings.Contains(text, "Access has not been checked") || !strings.Contains(text, "/o/one") || !strings.Contains(text, "/o/two") || strings.Contains(text, "must-not-print") || strings.Contains(text, "\x1b") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "  1. one\n     https://bi.example.com/o/one\n     Authenticate again: anfra auth login https://bi.example.com/o/one\n\n  2. two\n     https://other.example.com/o/two\n") {
		t.Fatalf("authenticated organizations are not numbered in order: %s", text)
	}
	if _, err := os.Stat(repoconfig.Path(".")); !os.IsNotExist(err) {
		t.Fatal("status created repo config")
	}
}

func TestAuthStatusRepoBoundariesAndJSON(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	seedLoginInventory(t, "https://bi.example.com/o/one", "https://other.example.com/o/two")
	writeConfig := func(dir, raw string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(repoconfig.Path(dir), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(root, "org:\n  url: https://outside.example.com/o/no\n")
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(repo, "org:\n  url: https://BI.example.com:443/o/one/\n")
	for _, dir := range []string{repo, nested} {
		t.Chdir(dir)
		cmd := newRootCmd()
		cmd.SetArgs([]string{"auth", "status", "--json"})
		var out, stderr bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Orgs          []authStatusOrg `json:"orgs"`
			RepoOrgURL    *string         `json:"repo_org_url"`
			RemoteChecked bool            `json:"remote_checked"`
		}
		if err := jsonkit.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Orgs) != 2 || result.RepoOrgURL == nil || *result.RepoOrgURL != "https://bi.example.com/o/one" || !result.Orgs[0].RepoActive || result.Orgs[1].RepoActive || result.RemoteChecked || stderr.Len() != 0 || strings.Contains(out.String(), "must-not-print") || strings.Contains(out.String(), "\x1b") {
			t.Fatalf("unsafe or incorrect result: %s stderr=%s", &out, &stderr)
		}
		var plain bytes.Buffer
		if err := printAuthStatus(context.Background(), dir, &plain, false, true); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plain.String(), "* 1. one  (current org of this repo)\n     https://bi.example.com/o/one\n     Authenticate again: anfra auth login https://bi.example.com/o/one\n\n  2. two\n     https://other.example.com/o/two\n") || strings.Contains(plain.String(), "\x1b") || strings.Contains(plain.String(), "anfra publish") || strings.Contains(plain.String(), "Set by") {
			t.Fatalf("repo publishing organization is not clearly identified: %s", &plain)
		}
	}
	writeConfig(repo, "org:\n  url: https://bi.example.com/o/missing\n")
	var plain bytes.Buffer
	if err := printAuthStatus(context.Background(), nested, &plain, false, false); err != nil || !strings.Contains(plain.String(), "This repo is configured to publish to https://bi.example.com/o/missing  Not authenticated\nRun anfra auth login https://bi.example.com/o/missing to authenticate\n") || strings.Contains(plain.String(), "(current org of this repo)") {
		t.Fatalf("%s %v", &plain, err)
	}
	for _, raw := range []string{"org: [", "org:\n  url: https://user:secret@bi.example.com/o/one\n"} {
		writeConfig(repo, raw)
		if err := printAuthStatus(context.Background(), nested, &bytes.Buffer{}, true, false); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("config error: %v", err)
		}
	}
	if err := os.Remove(repoconfig.Path(repo)); err != nil {
		t.Fatal(err)
	}
	for _, worktree := range []bool{false, true} {
		git := filepath.Join(repo, ".git")
		if worktree {
			if err := os.Remove(git); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(git, []byte("gitdir: elsewhere"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Mkdir(git, 0o700); err != nil {
			t.Fatal(err)
		}
		if url, err := statusRepoOrg(nested); err != nil || url != nil {
			t.Fatalf("crossed Git boundary: %v %v", url, err)
		}
	}
	writeConfig(filepath.Join(repo, "a"), "other: true\n")
	if url, err := statusRepoOrg(nested); err != nil || url != nil {
		t.Fatalf("skipped nearer config: %v %v", url, err)
	}
	if err := os.Remove(repoconfig.Path(root)); err != nil {
		t.Fatal(err)
	}
	if url, err := statusRepoOrg(root); err != nil || url != nil {
		t.Fatalf("crossed home boundary: %v %v", url, err)
	}
	seedLoginInventory(t)
	var empty bytes.Buffer
	if err := printAuthStatus(context.Background(), root, &empty, true, false); err != nil || empty.String() != "{\"orgs\":[],\"repo_org_url\":null,\"remote_checked\":false}" {
		t.Fatalf("%s %v", &empty, err)
	}
}
