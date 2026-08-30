package mutation

import (
	"errors"
	"fmt"
	"strings"

	"test-design/internal/gotestjson"
	"test-design/internal/jvmrun"
)

var (
	ErrZeroSelection       = errors.New("selection verification: zero records executed")
	ErrForeignIdentity     = errors.New("selection verification: an outcome exists outside the targeted identity")
	ErrUnsupportedIdentity = errors.New("selection verification: targeted identity's family could not be normalized")
)

// VerifyGoSelection proves rep is the targeted family run alone: at least
// one record executed, every record belongs to id.Scope and is id.Name or
// one of its subtests, and rep.Failures names no package but id.Scope (a
// mutated build/test failure in id's own package is a legitimate outcome).
func VerifyGoSelection(rep *gotestjson.Report, id Identity) error {
	if len(rep.Records) == 0 {
		return fmt.Errorf("%w: no record for %s.%s", ErrZeroSelection, id.Scope, id.Name)
	}
	for _, r := range rep.Records {
		if r.Scope != id.Scope || !goFamilyMember(r.Name, id.Name) {
			return fmt.Errorf("%w: record %s.%s ran outside %s.%s", ErrForeignIdentity, r.Scope, r.Name, id.Scope, id.Name)
		}
	}
	for _, f := range rep.Failures {
		if f.Package != id.Scope {
			return fmt.Errorf("%w: failure in package %s is outside %s", ErrForeignIdentity, f.Package, id.Scope)
		}
	}
	return nil
}

func goFamilyMember(name, want string) bool {
	return name == want || strings.HasPrefix(name, want+"/")
}

// VerifyJVMSelection proves res is the targeted family run alone.
// res.NormalizationError means the report cannot be trusted at all, so no
// verdict is possible; it is checked before anything else. An Unsupported
// family within id.Scope is checked next: the target command already
// confined execution to id.Scope's class, so an unrecognized decoration
// found there can only be id's own family, not a foreign one — it maps to
// the distinct ErrUnsupportedIdentity instead of ErrForeignIdentity.
func VerifyJVMSelection(res *jvmrun.Result, id Identity) error {
	if res.NormalizationError != "" {
		return fmt.Errorf("selection verification: report is not trustworthy: %s", res.NormalizationError)
	}
	if len(res.Records) == 0 {
		return fmt.Errorf("%w: no record for %s.%s", ErrZeroSelection, id.Scope, id.Name)
	}
	for _, fam := range res.Unsupported {
		if jvmScopeMember(fam.Identity.ClassName, id.Scope) {
			return fmt.Errorf("%w: %s could not be normalized to a declared method (%s)", ErrUnsupportedIdentity, fam.Identity.ClassName, fam.Identity.Unsupported)
		}
	}
	for _, r := range res.Records {
		if !jvmScopeMember(r.Scope, id.Scope) || !jvmFamilyMember(r.Name, id.Name) {
			return fmt.Errorf("%w: record %s.%s ran outside %s.%s", ErrForeignIdentity, r.Scope, r.Name, id.Scope, id.Name)
		}
	}
	return nil
}

func jvmScopeMember(scope, want string) bool {
	return scope == want || strings.HasPrefix(scope, want+"$")
}

func jvmFamilyMember(name, want string) bool {
	return name == want || strings.HasPrefix(name, want+"[") || strings.HasPrefix(name, want+"(")
}
