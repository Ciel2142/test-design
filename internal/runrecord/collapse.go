package runrecord

import "fmt"

// Collapse reduces one test family's member statuses to the family's
// status, worst-first: any Failed member makes the family Failed;
// otherwise any Skipped member makes it Skipped; otherwise Passed. An empty
// family is an error, since zero executed records can never be a pass.
func Collapse(members []Status) (Status, error) {
	if len(members) == 0 {
		return "", fmt.Errorf("runrecord: collapse: empty family")
	}

	sawFailed := false
	sawSkipped := false
	for _, m := range members {
		if !validStatus(m) {
			return "", fmt.Errorf("runrecord: collapse: invalid status %q", string(m))
		}
		switch m {
		case Failed:
			sawFailed = true
		case Skipped:
			sawSkipped = true
		}
	}

	switch {
	case sawFailed:
		return Failed, nil
	case sawSkipped:
		return Skipped, nil
	default:
		return Passed, nil
	}
}
