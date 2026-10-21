package motherduck

import "context"

// Account is a person's MotherDuck account, through the CLI and the REST API.
// Find finds the CLI, before anything else.
type Account struct {
	CLI CLI
	API API
}

// Find finds the CLI, and answers its version.
func (a *Account) Find(ctx context.Context) (version string, ok bool) {
	path, ok := Find()
	if !ok {
		return "", false
	}
	a.CLI.Path = path
	version, err := a.CLI.Version(ctx)
	if err != nil {
		return "", false
	}
	return version, true
}

func (a *Account) Status(ctx context.Context) (Status, error) { return a.CLI.Status(ctx) }

func (a *Account) Login(ctx context.Context) error { return a.CLI.Login(ctx) }

func (a *Account) Session(ctx context.Context) (token string, databases []string, err error) {
	return a.CLI.Session(ctx)
}

func (a *Account) DatabaseExists(ctx context.Context, name string) (bool, error) {
	return a.CLI.DatabaseExists(ctx, name)
}

func (a *Account) CreateDatabaseFrom(ctx context.Context, name, path string) error {
	return a.CLI.CreateDatabaseFrom(ctx, name, path)
}

func (a *Account) CreateReadToken(ctx context.Context, session, username, name, description string) (Token, error) {
	return a.API.CreateReadToken(ctx, session, username, name, description)
}

func (a *Account) RevokeToken(ctx context.Context, session, username, id string) error {
	return a.API.RevokeToken(ctx, session, username, id)
}
