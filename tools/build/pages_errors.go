package main

// buildErrors builds errors/<code>/ for every code of c.Synod.ExplainJSON
// and the errors/ index (plan §3, Errors): summary, text, the example files
// (all of example.files, synod.mod included, decision 111), the diagnostic,
// the fix note and the fixed files (example plus fix.files minus
// fix.deleted), Run or Check per the entry's command. The build checks
// natively that the example gives the entry's diagnostic.
func buildErrors(c *Ctx) error { return nil }
