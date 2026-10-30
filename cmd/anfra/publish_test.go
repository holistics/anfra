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

	"github.com/holistics/anfra/internal/cliauth"
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
	printError(&diagnostics, err, "")
	for _, want := range []string{path, "move the file aside", "anfra auth login for each organization", "does not revoke server access"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("missing %q in %s", want, &diagnostics)
		}
	}
	if out.Len() != 0 || strings.Contains(diagnostics.String(), "must-not-print") {
		t.Fatalf("unsafe error output: stdout=%q stderr=%q", &out, &diagnostics)
	}
}

func TestLoginServerAndExplicitLogout(t *testing.T) {
	t.Setenv("ANFRA_CLOUD_URL", "https://runtime.example.com")
	origin, hint, err := loginServer("https://BI.example.com:443/o/acme")
	if err != nil || origin != "https://bi.example.com" || hint != "acme" {
		t.Fatalf("%s %s %v", origin, hint, err)
	}
	if origin, hint, err := loginServer(""); err != nil || origin != "https://runtime.example.com" || hint != "" {
		t.Fatalf("runtime cloud URL: %q %q %v", origin, hint, err)
	}
	dir := t.TempDir()
	if err := repoconfig.SetString(repoconfig.Path(dir), "org", "url", "https://bi.example.com/o/repo"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	terminal := newAuthTerminal(strings.NewReader(""), &bytes.Buffer{})
	if _, err := logoutOrg(context.Background(), nil, terminal); err == nil || !strings.Contains(err.Error(), "anfra auth logout https://<server>/o/<org>") {
		t.Fatalf("noninteractive logout used repo destination: %v", err)
	}
	org, err := logoutOrg(context.Background(), []string{"https://other.example.com/o/explicit"}, terminal)
	if err != nil || org.Slug != "explicit" {
		t.Fatalf("%v %v", org, err)
	}
}

func TestLogoutUsesGlobalAuthenticationOnly(t *testing.T) {
	for _, config := range []string{"org:\n  url: https://bi.example.com/o/repo\n", "org: ["} {
		t.Run(config, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".anfra"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(repoconfig.Path(dir), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			for _, tc := range []struct {
				name      string
				addresses []string
				keys      string
				want      string
				wantError string
			}{
				{name: "none", wantError: "no saved authentication to sign out of"},
				{name: "one still asks", addresses: []string{"https://bi.example.com/o/repo"}, keys: "\x03", wantError: context.Canceled.Error()},
				{name: "choose another", addresses: []string{"https://bi.example.com/o/repo", "https://bi.example.com/o/other"}, keys: "\r", want: "https://bi.example.com/o/other"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					seedLoginInventory(t, tc.addresses...)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					terminal := newAuthTerminal(strings.NewReader(tc.keys), io.Discard)
					terminal.interactive = true
					org, err := logoutOrg(ctx, nil, terminal)
					if tc.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), tc.wantError) {
							t.Fatalf("logout error = %v; want %q", err, tc.wantError)
						}
					} else if err != nil || org.String() != tc.want {
						t.Fatalf("logout org = %v, %v; want %s", org, err, tc.want)
					}
					got, err := os.ReadFile(repoconfig.Path(dir))
					if err != nil || string(got) != config {
						t.Fatalf("logout changed repo config: %q, %v", got, err)
					}
				})
			}
		})
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

func TestCloudURLDefaultAndOverride(t *testing.T) {
	previous := defaultCloudURL
	t.Cleanup(func() { defaultCloudURL = previous })
	for _, tc := range []struct {
		name, built, runtime, want string
		runtimeSet                 bool
	}{
		{name: "neither configured"},
		{name: "build default", built: "https://default.example.com", want: "https://default.example.com"},
		{name: "runtime only", runtimeSet: true, runtime: "https://runtime.example.com", want: "https://runtime.example.com"},
		{name: "runtime overrides build", built: "https://default.example.com", runtimeSet: true, runtime: "https://runtime.example.com", want: "https://runtime.example.com"},
		{name: "empty runtime disables build", built: "https://default.example.com", runtimeSet: true},
		{name: "runtime replaces invalid build", built: "invalid", runtimeSet: true, runtime: "https://runtime.example.com", want: "https://runtime.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defaultCloudURL = tc.built
			t.Setenv("ANFRA_CLOUD_URL", tc.runtime)
			if !tc.runtimeSet {
				if err := os.Unsetenv("ANFRA_CLOUD_URL"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := configuredCloudOrigin()
			if err != nil || got != tc.want {
				t.Fatalf("cloud URL = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestLoginServerRuntimeCloudURL(t *testing.T) {
	for _, tc := range []struct {
		name, value, want, wantError string
	}{
		{name: "missing", wantError: "provide an organization URL"},
		{name: "canonical", value: "HTTPS://Cloud.Example.COM:443/", want: "https://cloud.example.com"},
		{name: "local", value: "http://127.0.0.1:5173", want: "http://127.0.0.1:5173"},
		{name: "invalid", value: "https://example.com/path", wantError: "ANFRA_CLOUD_URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANFRA_CLOUD_URL", tc.value)
			origin, hint, err := loginServer("")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %q error, got %v", tc.wantError, err)
				}
				return
			}
			if err != nil || origin != tc.want || hint != "" {
				t.Fatalf("login server = (%q, %q, %v), want (%q, empty hint, nil)", origin, hint, err, tc.want)
			}
		})
	}
}

func TestLoginOffersOnlyCloudAndURLEntry(t *testing.T) {
	seedLoginInventory(t, "https://bi.example.com/o/stored")
	for _, tc := range []struct {
		name, cloudURL, keys, want string
	}{
		{name: "cloud", cloudURL: "https://cloud.example.com", keys: "\r"},
		{name: "organization URL", cloudURL: "https://cloud.example.com", keys: "\x1b[B\rhttps://other.example.com/o/another\r", want: "https://other.example.com/o/another"},
		{name: "no cloud", keys: "https://other.example.com/o/another\r", want: "https://other.example.com/o/another"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANFRA_CLOUD_URL", tc.cloudURL)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var output bytes.Buffer
			terminal := newAuthTerminal(strings.NewReader(tc.keys), &output)
			terminal.interactive = true
			address, err := terminal.loginAddress(ctx, "Authenticate with an organization")
			if err != nil || address != tc.want {
				t.Fatalf("login address = %q, %v; want %q", address, err, tc.want)
			}
			if strings.Contains(output.String(), "https://bi.example.com/o/stored") {
				t.Fatal("login displayed a saved authentication")
			}
		})
	}
}

func TestFirstPublishWithoutLoginsShowsAuthenticationImmediately(t *testing.T) {
	seedLoginInventory(t)
	t.Setenv("ANFRA_CLOUD_URL", "https://cloud.example.com")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var output bytes.Buffer
	terminal := newAuthTerminal(strings.NewReader("\x1b[8;24;100t"), &output)
	terminal.interactive = true
	if _, err := terminal.selectPublishOrg(ctx); err != context.DeadlineExceeded {
		t.Fatalf("cancel publish selection: %v", err)
	}
	for _, want := range []string{
		"This repo isn't linked to an organization yet.",
		"Authenticate to publish.",
		"https://cloud.example.com",
		"Enter your organization URL",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in first publish screen: %s", want, &output)
		}
	}
	if strings.Contains(output.String(), "Authenticate with another organization") {
		t.Fatalf("unnecessary organization selection: %s", &output)
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
			w.Header().Set("Content-Type", "application/json")
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

func TestPublishRecoveryPreservesCredentialErrors(t *testing.T) {
	terminal := newAuthTerminal(strings.NewReader(""), io.Discard)
	terminal.interactive = true
	linked, _ := cloudapi.ParseOrg("https://cloud.example.com/o/acme")
	cause := fmt.Errorf("cannot read credentials")
	if _, err := terminal.recoverPublishOrg(context.Background(), linked, cause); err != cause {
		t.Fatalf("credential error replaced: %v", err)
	}
}

func TestPublishRecoveryOffersLinkedAndOtherOrganizations(t *testing.T) {
	for _, addresses := range [][]string{nil, {"https://cloud.example.com/o/acme"}, {"https://cloud.example.com/o/beta"}, {"https://cloud.example.com/o/acme", "https://cloud.example.com/o/beta", "https://cloud.example.com/o/gamma"}} {
		t.Run(strings.Join(addresses, "+"), func(t *testing.T) {
			seedLoginInventory(t, addresses...)
			linked, _ := cloudapi.ParseOrg("https://cloud.example.com/o/acme")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var output bytes.Buffer
			terminal := newAuthTerminal(strings.NewReader("\x1b[8;24;120t"), &output)
			terminal.interactive = true
			_, cause := (cliauth.Manager{}).Authorize(ctx, linked)
			if cause == nil {
				t.Fatal("missing authentication accepted")
			}
			if _, err := terminal.recoverPublishOrg(ctx, linked, cause); err != context.DeadlineExceeded {
				t.Fatalf("recovery: %v", err)
			}
			for _, want := range []string{"This repo publishes to " + linked.String(), "You're not authenticated with this organization.", "> Authenticate with acme", "Switch to another organization"} {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("missing %q in recovery screen: %s", want, &output)
				}
			}
			if strings.Contains(output.String(), "Authenticate with a different organization") {
				t.Fatalf("new authentication displayed in top-level menu: %s", &output)
			}
			for _, address := range addresses {
				if address != linked.String() {
					if strings.Contains(output.String(), address) {
						t.Fatalf("saved org displayed in top-level menu: %s", &output)
					}
				}
			}
		})
	}
}

func TestPublishRecoverySwitchOrganization(t *testing.T) {
	for _, action := range []string{"cancel", "confirm", "authenticate", "authenticate without saved alternatives"} {
		t.Run(action, func(t *testing.T) {
			seedLoginInventory(t, "https://cloud.example.com/o/acme", "https://cloud.example.com/o/alpha", "https://cloud.example.com/o/beta")
			if action == "authenticate without saved alternatives" {
				seedLoginInventory(t, "https://cloud.example.com/o/acme")
			}
			t.Setenv("ANFRA_CLOUD_URL", "https://cloud.example.com")
			linked, _ := cloudapi.ParseOrg("https://cloud.example.com/o/acme")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			input, keys, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			defer keys.Close()
			frames, screen := io.Pipe()
			defer frames.Close()
			defer screen.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = io.WriteString(keys, "\x1b[8;24;160t")
				var rendered strings.Builder
				stage := 0
				buf := make([]byte, 4096)
				for {
					n, err := frames.Read(buf)
					if err != nil {
						return
					}
					rendered.Write(buf[:n])
					if stage > 0 && strings.Contains(string(buf[:n]), "\x1b[?25l") {
						_, _ = io.WriteString(keys, "\x1b[8;24;160t")
					}
					switch {
					case stage == 0 && strings.Contains(rendered.String(), "Switch to another organization"):
						stage++
						_, _ = io.WriteString(keys, "\x1b[B\r")
					case stage == 1 && strings.Contains(rendered.String(), "Choose an organization to publish to"):
						stage++
						answer := "\x1b[B\r"
						if action == "authenticate" {
							answer = "\x1b[B\x1b[B\r"
						}
						_, _ = io.WriteString(keys, answer)
					case (stage == 1 || stage == 2) && strings.Contains(rendered.String(), "Authenticate with an organization") && strings.Contains(rendered.String(), "Enter your organization URL"):
						stage = 3
						_, _ = io.WriteString(keys, "\x03")
					case stage == 2 && strings.Contains(rendered.String(), "This updates org.url in .anfra/config.yml."):
						stage++
						answer := "\r"
						if action == "confirm" {
							answer = "\x1b[B\r"
						}
						_, _ = io.WriteString(keys, answer)
					}
				}
			}()
			var output bytes.Buffer
			terminal := newAuthTerminal(input, io.MultiWriter(&output, screen))
			terminal.interactive = true
			selected, err := terminal.recoverPublishOrg(ctx, linked, cliauth.ErrAuthenticationRequired)
			_ = screen.Close()
			_ = keys.Close()
			<-done
			if action == "confirm" {
				if err != nil || selected.String() != "https://cloud.example.com/o/beta" {
					t.Fatalf("confirmed destination = %v, %v; screen: %s", selected, err, &output)
				}
			} else if err != context.Canceled {
				t.Fatalf("default did not cancel: %v; screen: %s", err, &output)
			}
		})
	}
}

func TestAppWarnings(t *testing.T) {
	var out strings.Builder
	appWarnings([]byte(`{"valid":true,"reports":[
		{"validator":"data_app","filePath":"apps/a.html","severity":"warning","message":"line 3: \"x.png\" is a file beside the app"},
		{"validator":"unused_field","filePath":"models/m.aml","severity":"warning","message":"unused"}
	]}`), &out)
	if got, want := out.String(), "! apps/a.html, line 3: \"x.png\" is a file beside the app\n"; got != want {
		t.Errorf("appWarnings wrote %q, want %q", got, want)
	}
}

// A question answers what is typed, in order, read ahead or not; Ctrl-C, which
// cancels the context, ends one unanswered with its default (the safe answer),
// though nothing is typed, and every one after it too.
func TestStdinPromptAnswersAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in, typed := io.Pipe()
	defer func() { _ = typed.Close() }()
	var out bytes.Buffer
	ask, input := stdinQuestions(ctx, in, &out)
	go func() { _, _ = io.WriteString(typed, "y\nn\n  my token  \n") }()
	if choice, ok := ask("First?", "yes", "no"); choice != 0 || !ok {
		t.Errorf("first: %d, %v", choice, ok)
	}
	if choice, ok := ask("Second?", "yes", "no"); choice != 1 || !ok {
		t.Errorf("second: %d, %v", choice, ok)
	}
	if answer, ok := input("Name:"); answer != "my token" || !ok {
		t.Errorf("input: %q, %v", answer, ok)
	}

	answered := make(chan int)
	go func() { choice, _ := ask("Third?", "continue", "skip"); answered <- choice }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case choice := <-answered:
		if choice != 1 {
			t.Errorf("interrupted: %d, want the default", choice)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl-C did not end the question")
	}
	if choice, _ := ask("Fourth?", "continue", "skip"); choice != 1 || strings.Contains(out.String(), "Fourth?") {
		t.Errorf("after Ctrl-C: %d, asked %q", choice, out.String())
	}
}
