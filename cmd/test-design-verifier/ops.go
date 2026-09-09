package main

import (
	"flag"
	"io"
)

type opHandler func(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError

type opSpec struct {
	newFlags func() *flag.FlagSet
	handler  opHandler
}

var ops = map[string]opSpec{
	"next-id": {
		newFlags: newNextIDFlags,
		handler:  nextIDHandler,
	},
	"preflight": {
		newFlags: newPreflightFlags,
		handler:  preflightHandler,
	},
	"baseline": {
		newFlags: newBaselineFlags,
		handler:  baselineHandler,
	},
	"trace": {
		newFlags: newTraceFlags,
		handler:  traceHandler,
	},
	"prepare-mutation": {
		newFlags: newPrepareMutationFlags,
		handler:  prepareMutationHandler,
	},
	"prepare-live": {
		newFlags: newPrepareLiveFlags,
		handler:  prepareLiveHandler,
	},
	"evaluate-mutation": {
		newFlags: newEvaluateMutationFlags,
		handler:  evaluateMutationHandler,
	},
	"execute-live": {
		newFlags: newExecuteLiveFlags,
		handler:  executeLiveHandler,
	},
	"finalize": {
		newFlags: newFinalizeFlags,
		handler:  finalizeHandler,
	},
	"record-attempt": {
		newFlags: newRecordAttemptFlags,
		handler:  recordAttemptHandler,
	},
	"record-review": {
		newFlags: newRecordReviewFlags,
		handler:  recordReviewHandler,
	},
	"record-mutation": {
		newFlags: newRecordMutationFlags,
		handler:  recordMutationHandler,
	},
	"resume": {
		newFlags: newResumeFlags,
		handler:  resumeHandler,
	},
}

func newNextIDFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("next-id", flag.ContinueOnError)
	fs.String("area", "", "coverage area for the generated ID")
	fs.String("repo", ".", "repository root")
	addRequestFlag(fs)
	return fs
}

func newPreflightFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.String("source", "", "coverage source markdown path")
	fs.String("repo", ".", "repository root")
	addRequestFlag(fs)
	return fs
}

func newBaselineFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func newTraceFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func newPrepareMutationFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("prepare-mutation", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func newEvaluateMutationFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("evaluate-mutation", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func newPrepareLiveFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("prepare-live", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func newExecuteLiveFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("execute-live", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func addRequestFlag(fs *flag.FlagSet) {
	fs.String("request", "", "path to a JSON request document, or - for stdin")
}
