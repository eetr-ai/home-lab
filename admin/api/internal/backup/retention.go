package backup

import "sort"

// toTrash picks the backups past the count a schedule keeps.
//
// Per database, newest first by the time Drive recorded, so "keep 7" means seven
// of each database rather than seven files in all. Ties on time fall back to the
// name, which carries the run's timestamp, so the answer never depends on the
// order Drive listed things in.
//
// Only databases in succeeded are considered. That is the rule that keeps a
// broken run from eating history: if tonight's dump of a database failed, its
// oldest good backup is the last thing that should go, and nothing of it is
// pruned until a run has replaced it.
func toTrash(files []File, keep int, succeeded map[string]bool) []File {
	byDatabase := map[string][]File{}
	for _, file := range files {
		if succeeded[file.Database] {
			byDatabase[file.Database] = append(byDatabase[file.Database], file)
		}
	}

	var ret []File
	for _, group := range byDatabase {
		sort.Slice(group, func(a, b int) bool {
			if !group[a].CreatedAt.Equal(group[b].CreatedAt) {
				return group[a].CreatedAt.After(group[b].CreatedAt)
			}
			return group[a].Name > group[b].Name
		})
		if len(group) > keep {
			ret = append(ret, group[keep:]...)
		}
	}

	// Sorted so the log reads in a stable order run after run.
	sort.Slice(ret, func(a, b int) bool { return ret[a].Name < ret[b].Name })
	return ret
}
