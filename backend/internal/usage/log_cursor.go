package usage

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// log_cursor.go parses usage-list continuation positions for ID and creation-time
// ordering. Timestamp text retains subsecond precision through the browser round
// trip, and the row ID disambiguates records sharing the same creation instant.
// API handlers reject malformed boundaries before passing authorized filters to
// the repository; a cursor changes the starting position within those filters.
//
// Creation-time cursors pair the original RFC3339 timestamp with a positive ID.
// Keeping both values allows the repository to use the same tuple for ordering
// and seeking, including when several records share one timestamp. ID ordering
// needs only the positive decimal identifier. Other sort columns have no cursor
// representation and continue to use offset pagination when no cursor is supplied.

// LogCursor identifies the last returned record within an authorized usage selection.
type LogCursor struct {
	ID int64 // exclusive boundary record identifier
	// CreatedAt participates only in creation-time ordering. ID ordering ignores
	// this value and uses the positive record identifier alone as its boundary.
	CreatedAt time.Time
}

// ParseLogCursor accepts a decimal ID or an RFC3339 timestamp paired with a positive
// decimal ID. Empty input selects offset pagination; unsupported sorts and malformed
// positions return an error before a database query can start.
// Timestamp parsing preserves its fractional seconds rather than reducing it to
// browser clock precision. The paired ID supplies a deterministic boundary for
// rows that share the same instant, so callers must retain both components when
// continuing creation-time ordering. The returned cursor carries no authorization
// scope; handlers still provide the original visibility and selection filters.
func ParseLogCursor(raw, sortBy string) (*LogCursor, error) {
	if raw == "" {
		return nil, nil
	}
	invalid := errors.New("invalid usage cursor for selected sort")
	var cursor LogCursor
	switch sortBy {
	case "created_at":
		stamp, id, found := strings.Cut(raw, "|")
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if !found || err != nil || parsed.IsZero() {
			return nil, invalid
		}
		cursor.CreatedAt = parsed
		raw = id
	case "id":
	default:
		return nil, invalid
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, invalid
	}
	cursor.ID = id
	return &cursor, nil
}
