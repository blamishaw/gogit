package main

import (
	"fmt"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	ds "local/gogit/data-structures"
)

type Diff struct{}

// Namespacing
var diff Diff

// Key: path, Value: oid
type Tree map[string]string

const (
	DIFF = iota
	MERGE
)

const COLORFLUSH = "\033[0m"
const RED = "\033[31m"
const GREEN = "\033[32m"

func (Diff) getDiffCmdAndArgs(action int, path string, files []string) (string, []string) {
	switch action {
	case DIFF:
		cmd := "diff"
		args := []string{"--unified", "--show-c-function"}
		for i, file := range files {
			args = append(args, "--label", fmt.Sprintf("%s/%s", string(rune(97+i)), path), file)
		}
		return cmd, args
	case MERGE:
		cmd := "diff3"
		args := []string{"-m"}
		commitSHA := filepath.Base(files[2])
		labels := [3]string{HEAD, BASE, commitSHA[:10]}
		for i, file := range files {
			args = append(args, "-L", labels[i], file)
		}
		return cmd, args
	default:
		return "", []string{}
	}
}

// compareTrees is an iterator that takes a variadic number of Tree objects and returns a filename
// and the associated oids for that file in the provided Trees
func (Diff) compareTrees(trees ...Tree) iter.Seq2[string, []string] {
	return func(yield func(string, []string) bool) {
		entries := make(map[string][]string)
		paths := ds.NewSet([]string{})

		for i, tree := range trees {
			for path, oid := range tree {
				paths.Add(path)
				if len(entries[path]) == 0 {
					entries[path] = make([]string, len(trees))
				}
				entries[path][i] = oid
			}
		}

		arr := paths.ToArray()
		// Sort paths to deterministically yield files in alphabetical order
		slices.SortFunc(arr, func(a, b string) int {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		})

		for _, path := range arr {
			if !yield(path, entries[path]) {
				return
			}
		}

	}
}

// iterChangedFiles returns an iterator of [filepath, action]
func (Diff) iterChangedFiles(fromTree, toTree Tree) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for path, oids := range diff.compareTrees(fromTree, toTree) {
			fromOid, toOid := oids[0], oids[1]
			if oids[0] != oids[1] {
				var action string
				if len(fromOid) == 0 && len(toOid) > 0 {
					action = "new file"
				} else if len(fromOid) > 0 && len(toOid) == 0 {
					action = "deleted"
				} else {
					action = "modified"
				}

				if !yield(path, action) {
					return
				}
			}
		}
	}
}

// DiffTrees takes two Tree objects and returns a string output of the files that have been changed.
// This works because the oid is a hash over the content of the file and therefore different oids imply
// different content. The action parameter either returns the tree diff or merged tree output
func (Diff) DiffTrees(treeFrom, treeTo Tree) ([]byte, error) {
	var output []byte
	for path, oids := range diff.compareTrees(treeFrom, treeTo) {
		if len(oids) != 2 {
			return []byte{}, fmt.Errorf("expected 2 oids, received %d", len(oids))
		}
		if oids[0] == oids[1] {
			continue
		}
		difference, err := diff.DiffBlobs(path, []string{oids[0], oids[1]})
		if err != nil {
			return []byte{}, err
		}
		output = append(output, difference...)

	}
	return output, nil
}

// MergeTrees takes two Trees and returns a map of path to blob
func (Diff) MergeTrees(baseTree, headTree, mergeTree Tree) (map[string]string, error) {
	res := make(map[string]string)
	for path, oids := range diff.compareTrees(baseTree, headTree, mergeTree) {
		baseBlob, headBlob, mergeBlob := oids[0], oids[1], oids[2]
		// If the oids are the same, use the toBlob data
		if headBlob == mergeBlob {
			res[path] = headBlob
		} else {
			out, err := diff.MergeBlobs(path, []string{headBlob, baseBlob, mergeBlob})
			if err != nil {
				return nil, err
			}
			oid, err := data.HashObject(out, BLOB)
			if err != nil {
				return nil, err
			}
			res[path] = oid
		}

	}
	return res, nil
}

// execBlobDiff blobs executes the "diff" shell command on the two specified BLOBs. If the action type is DIFF, the output is
// the diff of the two blobs. If the action type is MERGE, the output is the merged output of the two blobs
func (Diff) execBlobDiff(path string, blobs []string, action int) ([]byte, error) {
	var tempFiles []string

	// Cleanup tmp files
	defer func() {
		for _, file := range tempFiles {
			err := os.Remove(file)
			if err != nil {
				fmt.Printf("error cleaning up tmp files: %s\n", err)
			}
		}
	}()

	for i, blob := range blobs {
		err := data.writeTempBlob(blob, &tempFiles)
		if err != nil || len(tempFiles) < i+1 {
			return []byte{}, err
		}
	}

	cmd, args := diff.getDiffCmdAndArgs(action, path, tempFiles)
	execCmd := exec.Command(cmd, args...)
	out, err := execCmd.Output()
	if err != nil {
		// diff command returns 0 if no differences, 1 if differences
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() > 1 {
			return []byte{}, err
		}
	}
	return out, nil
}

func (Diff) DiffBlobs(path string, blobs []string) ([]byte, error) {
	return diff.execBlobDiff(path, blobs, DIFF)
}

func (Diff) MergeBlobs(path string, blobs []string) ([]byte, error) {
	return diff.execBlobDiff(path, blobs, MERGE)
}

func (Diff) PrettyPrint(line string) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 1 {
		return
	}

	var color string
	if trimmed[0] == '+' || strings.HasPrefix(trimmed, "new file") {
		color = GREEN
	} else if trimmed[0] == '-' || strings.HasPrefix(trimmed, "deleted") || strings.HasPrefix(trimmed, "modified") {
		color = RED
	} else {
		color = COLORFLUSH
	}
	fmt.Printf("%s%s\n%s", color, line, COLORFLUSH)
}
