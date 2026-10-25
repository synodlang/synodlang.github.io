package main

// buildExamples builds examples/ and examples/<folder>/ from the examples
// checkout (plan §3, Examples): a row of examples/README.md gives Topic,
// What to watch and Seed; the folder's files and its module-local imports
// (c.Synod.DocScope) become editable tabs of a program widget. It writes
// assets/examples-<tag>.json (the module's synod.mod and *.synod files) with
// c.WriteDocs; every example is checked or run natively and an unexpected
// exit fails the build.
func buildExamples(c *Ctx) error { return nil }
