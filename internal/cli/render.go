package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
)

// render writes a checked summary in the requested format.
func render(w io.Writer, format string, summary intelligence.IntelligenceSummary) error {
	if format == "text" {
		return renderText(w, summary)
	}
	return renderJSON(w, summary)
}

func renderJSON(w io.Writer, summary intelligence.IntelligenceSummary) error {
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	_, err = w.Write(encoded)
	return err
}

// renderText writes the same summary for a person reading a terminal. It labels
// itself unverified again, because a text summary is the easiest thing to paste
// into a channel where the label would otherwise be lost.
func renderText(w io.Writer, summary intelligence.IntelligenceSummary) error {
	var out strings.Builder

	out.WriteString("UNVERIFIED INTELLIGENCE - HUMAN REVIEW REQUIRED\n\n")

	fmt.Fprintf(&out, "Claims (%d):\n", len(summary.Claims))
	if len(summary.Claims) == 0 {
		out.WriteString("  (none)\n")
	}
	for _, claim := range summary.Claims {
		fmt.Fprintf(&out, "  - %s\n", claim.Text)
		fmt.Fprintf(&out, "    Sources: %s\n", joinOrNone(claim.SourceIDs))
		fmt.Fprintf(&out, "    Confidence: %s\n", claim.Confidence)
		for _, evidence := range claim.Evidence {
			fmt.Fprintf(&out, "    Evidence: %s: %q\n", evidence.SourceID, evidence.Quote)
		}
	}
	out.WriteByte('\n')

	fmt.Fprintf(&out, "Themes (%d):\n", len(summary.Themes))
	if len(summary.Themes) == 0 {
		out.WriteString("  (none)\n")
	}
	for _, theme := range summary.Themes {
		fmt.Fprintf(&out, "  - %s\n    Sources: %s\n", theme.Text, joinOrNone(theme.SourceIDs))
	}
	out.WriteByte('\n')

	fmt.Fprintf(&out, "Conflicts (%d):\n", len(summary.Conflicts))
	if len(summary.Conflicts) == 0 {
		out.WriteString("  (none)\n")
	}
	for _, conflict := range summary.Conflicts {
		fmt.Fprintf(&out, "  - %s\n", conflict.Subject)
		fmt.Fprintf(&out, "    Positions: %s\n", joinOrNone(conflict.Positions))
		fmt.Fprintf(&out, "    Sources: %s\n", joinOrNone(conflict.SourceIDs))
	}
	out.WriteByte('\n')

	fmt.Fprintf(&out, "Gaps (%d):\n", len(summary.Gaps))
	if len(summary.Gaps) == 0 {
		out.WriteString("  (none)\n")
	}
	for _, gap := range summary.Gaps {
		fmt.Fprintf(&out, "  - %s\n", gap.Text)
		if len(gap.SourceIDs) > 0 {
			fmt.Fprintf(&out, "    Sources: %s\n", joinOrNone(gap.SourceIDs))
		}
	}
	out.WriteByte('\n')

	fmt.Fprintf(&out, "Citations (%d):\n", len(summary.Citations))
	if len(summary.Citations) == 0 {
		out.WriteString("  (none)\n")
	}
	for _, citation := range summary.Citations {
		fmt.Fprintf(&out, "  - %s %q (%s)", citation.SourceID, citation.Title, citation.PublishedAt)
		if len(citation.DuplicateOf) > 0 {
			fmt.Fprintf(&out, " duplicates: %s", joinOrNone(citation.DuplicateOf))
		}
		out.WriteByte('\n')
	}

	_, err := io.WriteString(w, out.String())
	return err
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
