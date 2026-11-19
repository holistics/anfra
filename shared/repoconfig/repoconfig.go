// Package repoconfig is a repo's settings, as .anfra/config.yml holds them:
// what each setting is, how a file is read, and how the layers that set them
// (the file, a member's .anfra/config.local.yml, a platform's overrides) are
// laid over each other. anfra core owns it, as it owns the repo format;
// anfra-cloud imports it, so the CLI and every host read settings alike.
//
// A setting is a field of Config that isn't a mapping: a scalar, or a list,
// which is one value. Its Key is the path to it, spelled as the file spells it.
// What a layer sets is a list of Settings, each one setting's key and its
// value as JSON, which Layer applies over anfra's defaults.
package repoconfig

import (
	"fmt"
	"reflect"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/repoformat"
)

// FilePath is the settings file, relative to the repo's root: committed, and
// published with the repo (repoformat.Publishable).
const FilePath = repoformat.ConfigFile

// LocalFilePath is a member's own settings, relative to the repo's root,
// laid over FilePath's on their machine only: never committed or published.
const LocalFilePath = ".anfra/config.local.yml"

// MaxOrigins is the most origins apps.allow_origins may list: each is one more
// server whose code runs beside readers' query results.
const MaxOrigins = 20

// Config is a repo's settings. Its fields are the settings anfra knows: a key
// in a file that has no field here is refused, never ignored. The zero value is
// anfra's defaults.
type Config struct {
	Org struct {
		// URL is the org `anfra publish` publishes the repo to, by its address on
		// its server. Never a credential. A platform ignores it (Key.Local).
		URL string `yaml:"url"`
	} `yaml:"org"`
	Apps struct {
		// AllowOrigins are the origins the repo's Data Apps may load from beyond
		// the SDK's own CDNs, each passing repoformat.CheckOrigin, at most
		// MaxOrigins.
		AllowOrigins []string `yaml:"allow_origins"`
		// Plugins are the SDK plugins the repo's Data Apps get, one mapping per
		// plugin a repo may turn off: core, always installed, has none. A
		// plugin's own settings, if one ever has any, go beside its Enabled.
		Plugins struct {
			Inspect            Plugin `yaml:"inspect"`
			Lineage            Plugin `yaml:"lineage"`
			ViewUnderlyingData Plugin `yaml:"view-underlying-data"`
			Render             Plugin `yaml:"render"`
			Export             Plugin `yaml:"export"`
		} `yaml:"plugins"`
	} `yaml:"apps"`
	// Query is how the repo's queries run (docs/designs/query-settings.md).
	Query struct {
		// Default is what a query gets when it sets none: each key is the path
		// of the query's own field, in snake_case. Unset, anfra's own default.
		Default struct {
			Input struct {
				// Timezone is input.timezone's default, an IANA zone.
				Timezone string `yaml:"timezone"`
				// WeekStartDay is input.weekStartDay's default: monday to sunday.
				WeekStartDay string `yaml:"weekStartDay"`
			} `yaml:"input"`
			Cache struct {
				// Duration is cache.duration's default: how old a cached result
				// a query accepts, and how long the result it caches is kept.
				// Unset, a query that sets none doesn't use the cache.
				Duration string `yaml:"duration"`
			} `yaml:"cache"`
		} `yaml:"default"`
		// LockedDefaults are the paths of the defaults a query can't change, such
		// as input.weekStartDay. Each names a value Default sets.
		LockedDefaults []string `yaml:"locked_defaults"`
		Cache          struct {
			// Enabled turns the query cache off for the repo when false: no
			// store, no canal-lake. Unset is on.
			Enabled *bool `yaml:"enabled"`
			// MaxDuration is the longest a query's result may be cached, at most
			// querycache.MaxDuration, which it is when unset.
			MaxDuration string `yaml:"max_duration"`
		} `yaml:"cache"`
	} `yaml:"query"`
	// AQL is how the repo's AQL compiles.
	AQL struct {
		Compiler struct {
			// Version is the AQL compiler the repo's AQL compiles and type-checks
			// with: "v1" (also when unset) or "v2".
			Version string `yaml:"version"`
		} `yaml:"compiler"`
	} `yaml:"aql"`
}

// Plugin is one SDK plugin's settings, under apps.plugins.<name>.
type Plugin struct {
	// Enabled installs the plugin in the repo's Data Apps. Unset is on.
	Enabled *bool `yaml:"enabled"`
}

// Plugins are the names of the SDK plugins a repo's settings turn on or off,
// in Config's order: every built-in plugin but core. No name holds a ".".
var Plugins = yamlNames(reflect.TypeOf(Config{}.Apps.Plugins))

// PluginKey is the setting that turns the plugin name on or off:
// apps.plugins.<name>.enabled.
func PluginKey(name string) Key { return Key("apps.plugins").child(name).child("enabled") }

// EnabledPlugins are the names of the SDK plugins the repo's Data Apps get
// beyond core, in Plugins' order: each one its setting leaves on.
func (c Config) EnabledPlugins() []string {
	v := reflect.ValueOf(c)
	on := []string{}
	for _, name := range Plugins {
		f, _ := field(v, PluginKey(name))
		if f.IsNil() || f.Elem().Bool() {
			on = append(on, name)
		}
	}
	return on
}

// QueryCacheEnabled is query.cache.enabled: on unless the repo sets false.
func (c Config) QueryCacheEnabled() bool {
	return c.Query.Cache.Enabled == nil || *c.Query.Cache.Enabled
}

// QueryCacheSettings are query.cache's rules the engine takes: its cap, zero
// when unset. Whether the cache is off is the host's to decide, since the
// environment can override the file. Check checked them.
func (c Config) QueryCacheSettings() command.QueryCacheSettings {
	d, _ := time.ParseDuration(c.Query.Cache.MaxDuration)
	return command.QueryCacheSettings{MaxDuration: d}
}

// QueryDefaults are query.default and query.locked_defaults, as the engine
// takes them: the one place settings become a query's defaults. Check checked
// them.
func (c Config) QueryDefaults() command.QueryDefaults {
	d := c.Query.Default
	duration, _ := time.ParseDuration(d.Cache.Duration)
	return command.QueryDefaults{
		Timezone: d.Input.Timezone, WeekStartDay: d.Input.WeekStartDay, CacheDuration: duration,
		Locked: c.Query.LockedDefaults,
	}
}

// UseV2Compiler is whether the repo's AQL compiles and type-checks with the
// AQL v2 compiler: aql.compiler.version is v2.
func (c Config) UseV2Compiler() bool { return c.AQL.Compiler.Version == "v2" }

// AllowedOrigins is apps.allow_origins, each an origin a Data App's content
// policy can take: Check checked them.
func (c Config) AllowedOrigins() []string { return c.Apps.AllowOrigins }

// checkOrigins refuses an origin a content policy can't take, or too many.
func checkOrigins(origins []string) error {
	if len(origins) > MaxOrigins {
		return fmt.Errorf("apps.allow_origins lists %d origins. List at most %d", len(origins), MaxOrigins)
	}
	for _, o := range origins {
		if err := repoformat.CheckOrigin(o); err != nil {
			return fmt.Errorf("apps.allow_origins: %w", err)
		}
	}
	return nil
}
