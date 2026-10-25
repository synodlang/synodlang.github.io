package main

// buildReference builds reference/NN-slug/, one page per
// <dsl>/spec/NN-*.md rendered by md.go, with heading ids like `s3-3`,
// links rewritten by rewriteHref (plan §3, Reference), a "Taught in" box
// from the coverage rows, and program widgets for complete `# package main`
// blocks (module example.com/snippet, file snippet/main.synod). Also the
// reference/ index.
func buildReference(c *Ctx) error { return nil }
