// Package junitxml normalizes captured JUnit-shaped XML reports (Maven
// Surefire/Failsafe, Gradle Java/Kotlin) into the verifier's runrecord shape.
package junitxml

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"test-design/internal/runrecord"
)

type Case struct {
	Status    runrecord.Status
	ClassName string // testcase classname attribute, verbatim (Outer$Inner preserved)
	RawName   string // testcase name attribute, verbatim
}

type Document struct {
	Cases []Case
}

var suiteLocalNames = map[string]bool{
	"testsuite":  true,
	"testsuites": true,
}

var failStatusLocalNames = map[string]bool{
	"failure": true,
	"error":   true,
}

var skipStatusLocalNames = map[string]bool{
	"skipped":  true,
	"disabled": true,
	"ignored":  true,
	"pending":  true,
	"aborted":  true,
}

var ignorableLocalNames = map[string]bool{
	"system-out": true,
	"system-err": true,
	"properties": true,
	"property":   true,
}

// NormalizeFile parses one JUnit-shaped XML report whose root is testsuite
// or testsuites (recursively). Element and attribute matching is by local
// name, so an XML namespace changes nothing. Every named failure returns an
// error and no partial Document.
func NormalizeFile(r io.Reader) (*Document, error) {
	dec := xml.NewDecoder(r)

	doc := &Document{}
	foundRoot := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("junitxml: %w", err)
		}

		switch t := tok.(type) {
		case xml.Directive:
			if err := rejectDTDOrEntity(t); err != nil {
				return nil, err
			}
		case xml.StartElement:
			if foundRoot {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("junitxml: %w", err)
				}
				continue
			}
			if !suiteLocalNames[t.Name.Local] {
				return nil, fmt.Errorf("junitxml: root element must be testsuite or testsuites, got %q", t.Name.Local)
			}
			foundRoot = true
			if err := parseSuite(dec, &doc.Cases); err != nil {
				return nil, err
			}
		}
	}

	if !foundRoot {
		return nil, fmt.Errorf("junitxml: no root element found")
	}
	return doc, nil
}

func rejectDTDOrEntity(d xml.Directive) error {
	upper := strings.ToUpper(strings.TrimSpace(string(d)))
	if strings.HasPrefix(upper, "DOCTYPE") || strings.HasPrefix(upper, "ENTITY") {
		return fmt.Errorf("junitxml: rejected DTD/entity directive: %s", strings.TrimSpace(string(d)))
	}
	return nil
}

func parseSuite(dec *xml.Decoder, cases *[]Case) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("junitxml: %w", err)
		}

		switch t := tok.(type) {
		case xml.Directive:
			if err := rejectDTDOrEntity(t); err != nil {
				return err
			}
		case xml.StartElement:
			switch {
			case suiteLocalNames[t.Name.Local]:
				if err := parseSuite(dec, cases); err != nil {
					return err
				}
			case t.Name.Local == "testcase":
				c, err := parseTestCase(dec, t)
				if err != nil {
					return err
				}
				*cases = append(*cases, c)
			default:
				if err := dec.Skip(); err != nil {
					return fmt.Errorf("junitxml: %w", err)
				}
			}
		case xml.EndElement:
			return nil
		}
	}
}

func parseTestCase(dec *xml.Decoder, start xml.StartElement) (Case, error) {
	var className, rawName string
	var haveClassName, haveName bool
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "classname":
			className = a.Value
			haveClassName = true
		case "name":
			rawName = a.Value
			haveName = true
		}
	}
	if !haveClassName || !haveName {
		return Case{}, fmt.Errorf("junitxml: testcase missing classname or name attribute")
	}

	sawFailed := false
	sawSkipped := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return Case{}, fmt.Errorf("junitxml: %w", err)
		}

		switch t := tok.(type) {
		case xml.Directive:
			if err := rejectDTDOrEntity(t); err != nil {
				return Case{}, err
			}
		case xml.StartElement:
			switch {
			case failStatusLocalNames[t.Name.Local]:
				sawFailed = true
				if err := dec.Skip(); err != nil {
					return Case{}, fmt.Errorf("junitxml: %w", err)
				}
			case skipStatusLocalNames[t.Name.Local]:
				sawSkipped = true
				if err := dec.Skip(); err != nil {
					return Case{}, fmt.Errorf("junitxml: %w", err)
				}
			case ignorableLocalNames[t.Name.Local]:
				if err := dec.Skip(); err != nil {
					return Case{}, fmt.Errorf("junitxml: %w", err)
				}
			default:
				return Case{}, fmt.Errorf("junitxml: unrecognized status representation %q in testcase %q", t.Name.Local, rawName)
			}
		case xml.EndElement:
			status := runrecord.Passed
			switch {
			case sawFailed:
				status = runrecord.Failed
			case sawSkipped:
				status = runrecord.Skipped
			}
			return Case{Status: status, ClassName: className, RawName: rawName}, nil
		}
	}
}
