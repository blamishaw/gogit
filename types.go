package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const GOGIT_DIR = ".gogit"

var GOGIT_ROOT = filepath.Join(".", GOGIT_DIR)
var GOGIT_INDEX = filepath.Join(GOGIT_ROOT, "index")

// Object types
const (
	BLOB   = "blob"
	TREE   = "tree"
	COMMIT = "commit"
)

// Refs
const (
	HEAD       = "HEAD"
	MERGE_HEAD = "MERGE_HEAD"
	BASE       = "BASE"
)

const BASE_BRANCH = "refs/heads/main"

// Directories in which refs can be found
var localRefDirs = []string{"", "refs", "refs/tags", "refs/heads"}

const remoteRefDir = "refs/remote"

type CommitObject struct {
	TreeOid    string
	ParentOids []string
	Timestamp  time.Time
	Message    string
}

func (c CommitObject) String() string {
	s := fmt.Sprintf("tree %s\n", c.TreeOid)
	s += fmt.Sprintf("time %s\n", c.Timestamp)
	for _, parentOid := range c.ParentOids {
		s += fmt.Sprintf("parent %s\n", parentOid)
	}
	s += fmt.Sprintf("message %s", c.Message)
	return s
}

type TreeEntry struct {
	Name string
	Oid  string
	Type string
}

type RefValue struct {
	Symbolic bool
	Value    string
}

/* Error Types */
type GogitError struct {
	message string
}

func (err GogitError) Error() string {
	return fmt.Sprintf("error: %s", err.message)
}

type ObjectTypeError struct {
	received string
	expected string
}

func (err ObjectTypeError) Error() string {
	return fmt.Sprintf(
		"Object is of type: %s\nShould be type: %s",
		strings.ToUpper(err.received),
		strings.ToUpper(err.expected),
	)
}

type RefNotFoundError struct {
	ref string
}

func (err RefNotFoundError) Error() string {
	return fmt.Sprintf("no ref found with name \"%s\"", err.ref)
}
