package mongo

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// maxFindLimit bounds a result. A find with no limit over a large collection
	// would otherwise be sent to a browser in full.
	maxFindLimit = 200
	// defaultFindLimit is what a caller gets without asking.
	defaultFindLimit = 50
	// findTimeout bounds how long a find may take. Applied as a context deadline:
	// driver v2 removed SetMaxTime and derives the server-side timeout from the
	// context instead, so MongoDB still kills the operation itself rather than the
	// client abandoning one that keeps running.
	findTimeout = 15 * time.Second
)

// Find returns documents from one collection.
//
// A find and nothing else. The driver sends a typed command with the filter as a
// BSON document, so there is no query string to parse and nothing to inject into
// — but a find is also the only shape offered: aggregate can write with $out and
// $merge, and runCommand is the whole server. Neither belongs behind a read-only
// button, and adding them later is a deliberate decision rather than a widening
// nobody noticed.
func (r *Repository) Find(ctx context.Context, database string, req FindRequest) (FindResult, error) {
	// Clamped, not discarded. A caller asking for more than the cap is asking for
	// as much as it can have, and dropping to the default would both return fewer
	// documents than are available and report truncation at the wrong number.
	limit := req.Limit
	switch {
	case limit <= 0:
		limit = defaultFindLimit
	case limit > maxFindLimit:
		limit = maxFindLimit
	}

	ctx, cancel := context.WithTimeout(ctx, findTimeout)
	defer cancel()

	opts := options.Find().
		// One more than asked for, so a full page can be told from a page that
		// happened to end exactly at the limit.
		SetLimit(limit + 1)
	if len(req.Projection) > 0 {
		opts = opts.SetProjection(bson.M(req.Projection))
	}
	if len(req.Sort) > 0 {
		opts = opts.SetSort(orderedFrom(req.Sort))
	}

	filter := bson.M{}
	if len(req.Filter) > 0 {
		filter = bson.M(req.Filter)
	}

	started := time.Now()
	cursor, err := r.client.Database(database).Collection(req.Collection).Find(ctx, filter, opts)
	if err != nil {
		// The server's own message, which names the offending operator or the
		// index it could not use — the useful part of a failed query.
		return FindResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}
	defer func() { _ = cursor.Close(ctx) }()

	result := FindResult{Documents: []string{}}
	for cursor.Next(ctx) {
		if int64(len(result.Documents)) == limit {
			result.Truncated = true
			break
		}
		rendered, err := bson.MarshalExtJSON(cursor.Current, false, false)
		if err != nil {
			return FindResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
		}
		result.Documents = append(result.Documents, string(rendered))
	}
	if err := cursor.Err(); err != nil && !result.Truncated {
		return FindResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}

	result.ElapsedMs = time.Since(started).Milliseconds()
	return result, nil
}

// orderedFrom turns a sort map into a document.
//
// JSON objects have no order and Go maps have none either, so a multi-key sort
// arriving as a map cannot preserve what the caller wrote. Rather than pretend
// otherwise, the keys are sorted by name — deterministic, and the single-key case
// that is almost every sort is unaffected. A caller needing a specific compound
// order will need an ordered representation, which is a change to the request
// type rather than something to fake here.
func orderedFrom(sort map[string]any) bson.D {
	keys := make([]string, 0, len(sort))
	for key := range sort {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	document := make(bson.D, 0, len(keys))
	for _, key := range keys {
		document = append(document, bson.E{Key: key, Value: sort[key]})
	}
	return document
}
