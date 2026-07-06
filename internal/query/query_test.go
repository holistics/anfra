package query

import "testing"

func TestExtractLimit(t *testing.T) {
	braceAQL := "explore {\n  dimensions {\n    products.name\n  }\n}"

	tests := []struct {
		name    string
		in      string
		wantAQL string
		wantLim int
		wantErr bool
	}{
		{
			name:    "no directive",
			in:      "products | select(products.name)",
			wantAQL: "products | select(products.name)",
			wantLim: NoLimit,
		},
		{
			name:    "brace-form with limit on its own line",
			in:      braceAQL + "\nlimit: 5\n",
			wantAQL: braceAQL + "\n", // brace kept, directive gone
			wantLim: 5,
		},
		{
			name:    "limit at end of input, no trailing newline",
			in:      braceAQL + "\nlimit: 100",
			wantAQL: braceAQL,
			wantLim: 100,
		},
		{
			name:    "extra spaces around value",
			in:      braceAQL + "  limit:   42  \n",
			wantAQL: braceAQL + "\n", // trailing newline after the directive is preserved
			wantLim: 42,
		},
		{
			name:    "limit zero is allowed",
			in:      braceAQL + " limit: 0",
			wantAQL: braceAQL,
			wantLim: 0,
		},
		{
			name:    "non-integer limit errors",
			in:      braceAQL + " limit: ten",
			wantErr: true,
		},
		{
			name:    "empty limit errors",
			in:      braceAQL + " limit:\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAQL, gotLim, err := ExtractLimit(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (aql=%q lim=%d)", gotAQL, gotLim)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotAQL != tt.wantAQL {
				t.Errorf("aql:\n got  %q\n want %q", gotAQL, tt.wantAQL)
			}
			if gotLim != tt.wantLim {
				t.Errorf("limit: got %d, want %d", gotLim, tt.wantLim)
			}
		})
	}
}
