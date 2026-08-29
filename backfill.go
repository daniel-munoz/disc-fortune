package main

import "slices"

// backfillResult reports what one backfill pass did.
type backfillResult struct {
	// Updated counts the entries that gained a release ID.
	Updated int
	// Ambiguous holds the legacy keys that matched more than one release,
	// each listed once. Only favorites report these: history is a log, there
	// is no action to take on a past pick, and a long history would produce
	// dozens of lines nobody can act on.
	Ambiguous []string
}

// indexByLegacyKey maps each "Artist - Title" in the collection to the
// distinct release IDs that answer to it. A key with more than one ID is
// exactly the case the old dedup used to hide.
func indexByLegacyKey(collection []Album) map[string][]int {
	idx := make(map[string][]int, len(collection))
	for _, a := range collection {
		if a.ReleaseID == 0 {
			continue
		}
		key := a.Key()
		if slices.Contains(idx[key], a.ReleaseID) {
			continue
		}
		idx[key] = append(idx[key], a.ReleaseID)
	}
	return idx
}

// backfillAlbums stamps release IDs onto entries that predate them, matching
// their legacy key against the freshly synced collection.
//
// Three outcomes, and only one of them writes anything. Exactly one match
// stamps the ID. No match means the record is no longer in the collection --
// sold, or dropped from the synced folders -- and is left alone silently.
// More than one match is unknowable: nothing in the file says which pressing
// the user meant, so guessing would write an assertion they never made and
// could not later tell apart from a real choice. Those stay on the name
// fallback, where they still display and still match, and are reported.
func backfillAlbums(entries, collection []Album) ([]Album, backfillResult) {
	idx := indexByLegacyKey(collection)
	out := make([]Album, len(entries))
	copy(out, entries)

	var res backfillResult
	reported := make(map[string]bool)

	for i := range out {
		if out[i].ReleaseID != 0 {
			continue
		}
		key := out[i].Key()
		ids := idx[key]
		switch {
		case len(ids) == 1:
			out[i].ReleaseID = ids[0]
			res.Updated++
		case len(ids) > 1 && !reported[key]:
			reported[key] = true
			res.Ambiguous = append(res.Ambiguous, key)
		}
	}

	return out, res
}

// backfillHistory is backfillAlbums over the album inside each history entry,
// leaving timestamps untouched.
func backfillHistory(entries []HistoryEntry, collection []Album) ([]HistoryEntry, backfillResult) {
	albums := make([]Album, len(entries))
	for i, e := range entries {
		albums[i] = e.Album
	}

	filled, res := backfillAlbums(albums, collection)

	out := make([]HistoryEntry, len(entries))
	copy(out, entries)
	for i := range out {
		out[i].Album = filled[i]
	}
	return out, res
}
