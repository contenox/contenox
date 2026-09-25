package vfs

import (
	"fmt"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
)

// SortField selects the column a listing is ordered and paginated by.
type SortField string

const (
	// SortByCreatedAt orders by creation time. It is the default.
	SortByCreatedAt SortField = "created_at"
	// SortByName orders lexicographically by name.
	SortByName SortField = "name"
)

// DefaultListLimit is the page size used when Page.Limit is not set.
const DefaultListLimit = 100

// Cursor is the keyset position after the last row of the previous page.
// Callers build it from the final row they received; only the field matching
// the chosen SortField is consulted, with ID as the tiebreaker.
type Cursor struct {
	CreatedAt time.Time
	Name      string
	ID        string
}

// Page describes a keyset-pagination request for a listing.
type Page struct {
	SortBy SortField // "" defaults to SortByCreatedAt
	Desc   bool      // descending order when true
	After  *Cursor   // nil requests the first page
	Limit  int       // <=0 uses DefaultListLimit; greater than runtimetypes.MAXLIMIT is rejected
}

// SortOrDefault returns the effective sort field.
func (p Page) SortOrDefault() SortField {
	if p.SortBy == "" {
		return SortByCreatedAt
	}
	return p.SortBy
}

// EffectiveLimit returns the page size that will be applied.
func (p Page) EffectiveLimit() int {
	if p.Limit <= 0 {
		return DefaultListLimit
	}
	return p.Limit
}

// Validate checks the sort field is supported and the limit is within bounds.
func (p Page) Validate() error {
	switch p.SortOrDefault() {
	case SortByCreatedAt, SortByName:
	default:
		return fmt.Errorf("vfsstore: unsupported sort field %q", p.SortBy)
	}
	if p.Limit > runtimetypes.MAXLIMIT {
		return runtimetypes.ErrLimitParamExceeded
	}
	return nil
}

// resolve validates the page and returns the effective sort field and limit.
func (p Page) resolve() (SortField, int, error) {
	if err := p.Validate(); err != nil {
		return "", 0, err
	}
	return p.SortOrDefault(), p.EffectiveLimit(), nil
}

// keyset builds the cursor predicate and ORDER BY clause for a keyset scan.
// colExpr is the qualified sort column (e.g. "ft.created_at"), idExpr the
// qualified tiebreaker column (e.g. "ft.id"), and startIdx the next positional
// placeholder ($N). It returns the " AND (...)" predicate (empty for the first
// page), the " ORDER BY ..." clause, and the cursor args in placeholder order.
func keyset(sortBy SortField, desc bool, after *Cursor, colExpr, idExpr string, startIdx int) (where, order string, args []any) {
	dir, cmp := "ASC", ">"
	if desc {
		dir, cmp = "DESC", "<"
	}
	order = fmt.Sprintf(" ORDER BY %s %s, %s %s", colExpr, dir, idExpr, dir)
	if after == nil {
		return "", order, nil
	}
	var sortVal any = after.CreatedAt
	if sortBy == SortByName {
		sortVal = after.Name
	}
	where = fmt.Sprintf(" AND (%s, %s) %s ($%d, $%d)", colExpr, idExpr, cmp, startIdx, startIdx+1)
	return where, order, []any{sortVal, after.ID}
}
