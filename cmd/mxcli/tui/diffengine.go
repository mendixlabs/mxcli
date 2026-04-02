package tui

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffLineType represents the type of a diff line.
type DiffLineType int

const (
	DiffEqual DiffLineType = iota
	DiffInsert
	DiffDelete
)

// DiffSegment represents a word-level segment within a changed line.
type DiffSegment struct {
	Text    string
	Changed bool
}

// DiffLine represents a single line in the diff output.
type DiffLine struct {
	Type      DiffLineType
	OldLineNo int // 0 for Insert lines
	NewLineNo int // 0 for Delete lines
	Content   string
	Segments  []DiffSegment // word-level breakdown (Insert/Delete only)
}

// DiffStats holds summary statistics for a diff.
type DiffStats struct {
	Additions int
	Deletions int
	Equal     int
}

// DiffResult holds the complete diff output.
type DiffResult struct {
	Lines []DiffLine
	Stats DiffStats
}

// ComputeDiff computes a line-level diff with word-level segments for changed lines.
// Uses go-difflib SequenceMatcher for high-quality line pairing (Python difflib algorithm),
// and sergi/go-diff for word-level segments within paired lines.
func ComputeDiff(oldText, newText string) *DiffResult {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	matcher := difflib.NewMatcherWithJunk(oldLines, newLines, false, nil)
	opcodes := matcher.GetOpCodes()

	result := &DiffResult{}
	oldLineNo := 0
	newLineNo := 0

	for _, op := range opcodes {
		switch op.Tag {
		case 'e': // equal
			for i := op.I1; i < op.I2; i++ {
				oldLineNo++
				newLineNo++
				result.Stats.Equal++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffEqual,
					OldLineNo: oldLineNo,
					NewLineNo: newLineNo,
					Content:   oldLines[i],
				})
			}

		case 'r': // replace — pair old[i1:i2] with new[j1:j2]
			delCount := op.I2 - op.I1
			insCount := op.J2 - op.J1
			paired := min(delCount, insCount)

			// Paired lines get word-level segments
			for k := range paired {
				oldSegs, newSegs := computeWordSegments(oldLines[op.I1+k], newLines[op.J1+k])

				oldLineNo++
				result.Stats.Deletions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffDelete,
					OldLineNo: oldLineNo,
					Content:   oldLines[op.I1+k],
					Segments:  oldSegs,
				})

				newLineNo++
				result.Stats.Additions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffInsert,
					NewLineNo: newLineNo,
					Content:   newLines[op.J1+k],
					Segments:  newSegs,
				})
			}

			// Excess deletes (more old lines than new)
			for k := paired; k < delCount; k++ {
				oldLineNo++
				result.Stats.Deletions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffDelete,
					OldLineNo: oldLineNo,
					Content:   oldLines[op.I1+k],
					Segments:  []DiffSegment{{Text: oldLines[op.I1+k], Changed: true}},
				})
			}

			// Excess inserts (more new lines than old)
			for k := paired; k < insCount; k++ {
				newLineNo++
				result.Stats.Additions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffInsert,
					NewLineNo: newLineNo,
					Content:   newLines[op.J1+k],
					Segments:  []DiffSegment{{Text: newLines[op.J1+k], Changed: true}},
				})
			}

		case 'd': // delete
			for i := op.I1; i < op.I2; i++ {
				oldLineNo++
				result.Stats.Deletions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffDelete,
					OldLineNo: oldLineNo,
					Content:   oldLines[i],
					Segments:  []DiffSegment{{Text: oldLines[i], Changed: true}},
				})
			}

		case 'i': // insert
			for j := op.J1; j < op.J2; j++ {
				newLineNo++
				result.Stats.Additions++
				result.Lines = append(result.Lines, DiffLine{
					Type:      DiffInsert,
					NewLineNo: newLineNo,
					Content:   newLines[j],
					Segments:  []DiffSegment{{Text: newLines[j], Changed: true}},
				})
			}
		}
	}

	return result
}

// splitLines splits text into lines, stripping trailing newline.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Remove trailing empty element from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeWordSegments runs character-level diff on two lines and maps
// the results to old/new DiffSegment slices with Changed flags.
func computeWordSegments(oldLine, newLine string) ([]DiffSegment, []DiffSegment) {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldLine, newLine, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var oldSegs, newSegs []DiffSegment
	for _, d := range diffs {
		if d.Text == "" {
			continue
		}
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			oldSegs = append(oldSegs, DiffSegment{Text: d.Text, Changed: false})
			newSegs = append(newSegs, DiffSegment{Text: d.Text, Changed: false})
		case diffmatchpatch.DiffDelete:
			oldSegs = append(oldSegs, DiffSegment{Text: d.Text, Changed: true})
		case diffmatchpatch.DiffInsert:
			newSegs = append(newSegs, DiffSegment{Text: d.Text, Changed: true})
		}
	}
	return oldSegs, newSegs
}
