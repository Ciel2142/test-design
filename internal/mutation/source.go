package mutation

import "strings"

func validKind(kind string) bool {
	switch kind {
	case "go", "maven", "gradle":
		return true
	}
	return false
}

func isProductionSource(path, kind string) bool {
	if commonlyForeign(path) {
		return false
	}
	switch kind {
	case "go":
		return isGoProductionSource(path)
	case "maven", "gradle":
		return isJVMProductionSource(path)
	}
	return false
}

func commonlyForeign(path string) bool {
	if pathHasSegment(path, ".test-design") || pathHasSegment(path, "docs") {
		return true
	}
	return !strings.Contains(path, "/") && strings.HasPrefix(path, ".")
}

func isGoProductionSource(path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	switch baseName(path) {
	case "go.mod", "go.sum":
		return false
	}
	return !pathHasSegment(path, "testdata")
}

var jvmBuildFileNames = map[string]bool{
	"pom.xml":             true,
	"gradle.properties":   true,
	"settings.gradle":     true,
	"settings.gradle.kts": true,
	"gradlew":             true,
	"gradlew.bat":         true,
	"mvnw":                true,
	"mvnw.cmd":            true,
}

func isJVMProductionSource(path string) bool {
	base := baseName(path)
	if jvmBuildFileNames[base] {
		return false
	}
	if strings.HasSuffix(base, ".gradle") || strings.HasSuffix(base, ".gradle.kts") {
		return false
	}
	if pathHasConsecutiveSegments(path, "gradle", "wrapper") {
		return false
	}
	if pathHasSegment(path, ".mvn") {
		return false
	}
	if pathHasConsecutiveSegments(path, "src", "test") {
		return false
	}
	if !pathHasConsecutiveSegments(path, "src", "main") {
		return false
	}
	return strings.HasSuffix(path, ".java") || strings.HasSuffix(path, ".kt")
}

func baseName(path string) string {
	if idx := strings.LastIndexByte(path, '/'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func pathHasSegment(path, segment string) bool {
	for _, s := range strings.Split(path, "/") {
		if s == segment {
			return true
		}
	}
	return false
}

func pathHasConsecutiveSegments(path, a, b string) bool {
	segs := strings.Split(path, "/")
	for i := 0; i+1 < len(segs); i++ {
		if segs[i] == a && segs[i+1] == b {
			return true
		}
	}
	return false
}
