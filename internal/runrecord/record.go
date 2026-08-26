package runrecord

// Record is a normalized test outcome. Scope is the package import path for
// Go or the testcase classname for JUnit XML; Name is the runner's
// test/subtest path for Go or the testcase name for JUnit XML.
type Record struct {
	Status Status
	Scope  string
	Name   string
}
