package main

// rewriteHref rewrites one link of a Markdown source page for the site
// (plan §3, Reference): spec/03 §3.3 and ../spec/NN-*.md go to
// reference/NN-slug/#s3-3, implementation/*.md go to GitHub at the pinned
// dsl tag, "decision N" to design/decisions/#dN, and a code span equal to a
// diagnostic code to the Errors page. page is the site URL of the page the
// link is on. ok is false when href is left as it is.
func rewriteHref(c *Ctx, page, href string) (rewritten string, ok bool) { return href, false }
