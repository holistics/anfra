package appserve

import (
	"os"
	"os/user"
	"strings"
)

// Reader is who reads the Data Apps, as the Anfra SDK provisions a user: on anfra serve, the
// local user, who may see everything.
type Reader struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Email       string      `json:"email"`
	Role        string      `json:"role"`
	Timezone    string      `json:"timezone"`
	Permissions Permissions `json:"permissions"`
}

type Permissions struct {
	CanViewGeneratedSQL bool `json:"canViewGeneratedSql"`
	CanExportData       bool `json:"canExportData"`
}

// LocalReader is the local user, with the machine's time zone.
func LocalReader() Reader {
	name := "Local user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	return Reader{
		ID: 1, Name: name, Role: "admin", Timezone: localTimezone(),
		Permissions: Permissions{CanViewGeneratedSQL: true, CanExportData: true},
	}
}

// localTimezone is the machine's IANA zone: $TZ, else the system's
// (systemTimezone), else Etc/UTC.
func localTimezone() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		return tz
	}
	if tz := systemTimezone(); tz != "" {
		return tz
	}
	return "Etc/UTC"
}
