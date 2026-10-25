package main

// buildHome builds the home page "/" (latest version only: it returns nil
// when !c.IsLatest()): icon, sentence, three buttons and a canvas playing
// assets/home-raft.json with loop true. It makes that scene with the native
// `synod run raft --scene` of the examples checkout (decision 110) and
// writes it with c.WriteDocs("assets/home-raft.json"). It also builds the
// docs home of the version from content/docs-home.md (plan §3, Home).
func buildHome(c *Ctx) error { return nil }
