package main

// buildTutorial builds tutorial/NN-slug/ for the 15 chapters of
// <dsl>/tutorial and the tutorial/ index (plan §3, Tutorial): a
// `<!-- step K program=<folder> -->` directive becomes a program widget (via
// c.Layout.Program) whose changed lines come from diffLines against step K-1
// (step 1 against the previous chapter's last step); `<!-- new -->` becomes
// the coverage list linking Reference anchors; a `<!-- why <slug> -->`
// shows the captured `synod check --style rich` diagnostic (c.Synod.CheckRich)
// and links the Errors page and its decision; previous and next links.
// Steps with synod.sum go through c.Synod.Get in a temp copy to embed vendor/.
func buildTutorial(c *Ctx) error { return nil }
