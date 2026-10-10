package command

import (
	"context"
	"io"
	"time"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// ExportStore is where exports go: a file written once, then a link to it that
// expires. A server provides it, as it provides its clients; a command knows no
// storage and no URL. See docs/designs/exports.md.
type ExportStore interface {
	// Create starts a file named filename (what a download saves it as), whose
	// content is of contentType.
	Create(ctx context.Context, filename, contentType string) (Export, error)
}

// Export is one file being written. Done finishes it and answers its link, which
// works until expiresAt; Abort discards it. One of them is called, once.
type Export interface {
	io.Writer
	Done(ctx context.Context) (url string, expiresAt time.Time, err error)
	Abort(ctx context.Context) error
}

// ExportLink is an export's answer: where to download its file, until when.
type ExportLink struct {
	URL       string    `json:"url" doc:"where to download the file: a plain GET, with no credentials, until expires_at"`
	Filename  string    `json:"filename" doc:"the file's name, as a download saves it"`
	Format    string    `json:"format" doc:"the file's format"`
	RowCount  int       `json:"row_count" doc:"the rows the file holds"`
	ExpiresAt time.Time `json:"expires_at" doc:"when the link stops working"`
}

// RequireExports refuses an export on a server that stores none, before the
// query runs.
func RequireExports(cc CommandContext) error {
	if cc.Exports == nil {
		return apperr.New(errcode.ExportsUnavailable, "")
	}
	return nil
}
