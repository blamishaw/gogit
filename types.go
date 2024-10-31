package main

import (
	"fmt"
	"path/filepath"
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
