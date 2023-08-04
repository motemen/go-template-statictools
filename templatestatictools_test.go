package templatestatictools

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestChecker_Parse(t *testing.T) {
	var checker Checker

	err := checker.ParseFile("testdata/in.tmpl")
	assert.NilError(t, err)
}
