package tags

import (
	"reflect"
	"testing"
)

func TestExtractBareID(t *testing.T) {
	got := Extract("covers: SC-01")
	want := []string{"SC-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractAreaPrefixedIDSurvives(t *testing.T) {
	// The trap this rule exists to prevent: substring-matching "covers: SC-"
	// silently drops area-prefixed forms like orders/SC-01.
	got := Extract("covers: orders/SC-01")
	want := []string{"orders/SC-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractCommaAndWhitespaceSeparatedList(t *testing.T) {
	got := Extract("covers: orders/SC-01, orders/SC-02")
	want := []string{"orders/SC-01", "orders/SC-02"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractCommentWithNoMarkerYieldsNoTokens(t *testing.T) {
	got := Extract("just a regular comment about something")
	if len(got) != 0 {
		t.Errorf("Extract = %v, want no tokens", got)
	}
}

func TestExtractMultiLineCommentBlockOnlyMarkerLine(t *testing.T) {
	comment := "this test asserts the widget name is required\ncovers: SC-01\nadditional trailing prose"
	got := Extract(comment)
	want := []string{"SC-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractMalformedTokenReturnedVerbatim(t *testing.T) {
	got := Extract("covers: not-an-id")
	want := []string{"not-an-id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractMultipleMarkerLinesEachContribute(t *testing.T) {
	comment := "covers: SC-01\nsome prose\ncovers: orders/SC-02"
	got := Extract(comment)
	want := []string{"SC-01", "orders/SC-02"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}

func TestExtractMarkerInsideLargerWordDoesNotMatch(t *testing.T) {
	got := Extract("this recovers: SC-01")
	if len(got) != 0 {
		t.Errorf("Extract = %v, want no tokens", got)
	}
}

func TestExtractMarkerAfterNonWordCharStillExtracts(t *testing.T) {
	got := Extract("// covers: SC-01")
	want := []string{"SC-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract = %v, want %v", got, want)
	}
}
