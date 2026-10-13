package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFileIDs(t *testing.T) {
	assert.Equal(t, []string{"F0123ABCDEF", "F0999ZZZZZZ"},
		parseFileIDs("F0123ABCDEF (Q3 report.pdf), F0999ZZZZZZ (notes.pdf)"))
	assert.Equal(t, []string{"F0123ABCDEF"}, parseFileIDs("F0123ABCDEF, F0123ABCDEF"))
	assert.Equal(t, []string{"F01AAAAAAA", "F01BBBBBBB"}, parseFileIDs("F01AAAAAAA F01BBBBBBB"))
	assert.Empty(t, parseFileIDs("Final report.pdf"))
	assert.Empty(t, parseFileIDs(""))
}

func TestDownloadFilename(t *testing.T) {
	assert.Equal(t, "Q3 report_123abcde.pdf", downloadFilename("Q3 report.pdf", "", "F0123ABCDE"))
	assert.Equal(t, "passwd_123abcde", downloadFilename("../../etc/passwd", "", "F0123ABCDE"))
	assert.Equal(t, "evil_123abcde.pdf", downloadFilename("..\\..\\evil.pdf", "", "F0123ABCDE"))
	assert.Equal(t, "Scan_123abcde.png", downloadFilename("", "Scan.png", "F0123ABCDE"))
	assert.Equal(t, "slack_file_123abcde", downloadFilename("", "", "F0123ABCDE"))
	assert.Equal(t, "a_b_c_123abcde.pdf", downloadFilename("a:b*c.pdf", "", "F0123ABCDE"))
}
