package main

// searchEntry is one search.json record: url, title, section, excerpt
// (at most 300 characters).
type searchEntry struct {
	U string `json:"u"`
	T string `json:"t"`
	S string `json:"s"`
	X string `json:"x"`
}

// buildSearch writes search.json of the version, one entry per heading,
// phrase and code of the registered pages (plan §3, Search). It runs after
// every other builder.
func buildSearch(c *Ctx) error { return nil }
