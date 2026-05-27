package skills

// RestoreLoadedSkills marks the given skill names as loaded. This is used to
// restore the tracker state after compaction. Only skills in the active set
// can be marked as loaded.
func (t *Tracker) RestoreLoadedSkills(names []string) {
	if t == nil || len(names) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, name := range names {
		if t.activeNames[name] {
			t.loaded[name] = true
		}
	}
}
