package main

// buildLibrary builds library/ and library/std-<name>/ for every std package
// listed by c.Synod.StdPackages (std/look/basic becomes std-look-basic)
// from `synod doc --json` (c.Synod.DocJSON) and the dsl/std/README.md
// section: intro prose, per phrase pattern, gives, contexts, kind, native,
// doc, receiver, source link at the std tag, runnable examples from
// dsl/std/examples/README.md as widgets, records, Node fields, and the theme
// table (plan §3, Library).
func buildLibrary(c *Ctx) error { return nil }
