package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"iter"
	ds "local/gogit/data-structures"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Base struct{}

// Namespacing
var base Base

type NoOIDFoundError struct {
	message string
}

func (err *NoOIDFoundError) Error() string {
	return err.message
}

func (Base) isBranch(name string) bool {
	ref, err := data.GetRef(filepath.Join("refs/heads", name), true)
	return err == nil && ref.Value != ""
}

// Returns true if oid1 is an ancestor of oid2
func (Base) isAncestorOf(oid1, oid2 string) bool {
	for oid := range base.iterCommitsAndParents([]string{oid2}) {
		if oid == oid1 && oid != oid2 {
			return true
		}
	}
	return false
}

func (Base) iterBranches() iter.Seq2[string, *RefValue] {
	return func(yield func(string, *RefValue) bool) {
		for refName, ref := range data.iterRefs("heads", true) {
			if !yield(filepath.Base(refName), ref) {
				return
			}
		}
	}
}

func (Base) iterCommitsAndParents(oids []string) iter.Seq[string] {
	return func(yield func(string) bool) {
		visited := ds.NewSet([]string{})

		var oid string
		for len(oids) > 0 {
			// Pop head off slice
			oid, oids = oids[0], oids[1:]
			if oid == "" || visited.Includes(oid) {
				continue
			}
			visited.Add(oid)
			if !yield(oid) {
				return
			}

			c, err := base.GetCommit(oid)
			if err != nil {
				panic(err)
			}

			if len(c.ParentOids) > 0 {
				// Return first parent next
				oids = append(c.ParentOids[:1], oids...)
				// Return other parents later
				oids = append(oids, c.ParentOids[1:]...)
			}
		}
	}
}

func (Base) iterTreeEntries(oid string) iter.Seq2[int, TreeEntry] {
	return func(yield func(int, TreeEntry) bool) {
		if oid == "" {
			return
		}
		tree, err := data.GetObject(oid, TREE)
		if err != nil {
			fmt.Printf("error iterating tree entries: %s", err)
			return
		}
		for i, entry := range strings.Split(string(tree), "\n") {
			fields := strings.Split(entry, " ")
			if len(fields) < 3 {
				return
			}
			obj := TreeEntry{fields[0], fields[1], fields[2]}
			if !yield(i, obj) {
				return
			}
		}
	}
}

// MapObjectsInCommits takes a list of commit OIDs, a path, and a function
// and maps the function over all objects reachable from the commits specified
func (Base) MapObjectsInCommits(commitOIDs []string, mapFn func(oid string) error) error {
	visited := ds.NewSet([]string{})
	for commitOID := range base.iterCommitsAndParents(commitOIDs) {
		if visited.Includes(commitOID) {
			continue
		}

		// Map over commit OID if object does not exist
		if err := mapFn(commitOID); err != nil {
			return err
		}

		commit, err := base.GetCommit(commitOID)
		if err != nil {
			return err
		}

		if err = base.mapObjectsInTree(commit.TreeOid, &visited, mapFn); err != nil {
			return err
		}
	}
	return nil
}

func (Base) mapObjectsInTree(treeOID string, visited *ds.Set[string], mapFn func(oid string) error) error {
	// Map over tree OID if object does not exist
	if err := mapFn(treeOID); err != nil {
		return err
	}

	for _, node := range base.iterTreeEntries(treeOID) {
		if visited.Includes(node.Oid) {
			continue
		}

		if node.Type == TREE {
			err := base.mapObjectsInTree(node.Oid, visited, mapFn)
			if err != nil {
				return err
			}
		} else {
			visited.Add(node.Oid)
			err := mapFn(node.Oid)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// getStructuredIndex reads the Gogit index and returns a structured map mirroring the directory structure
func (Base) getStructuredIndex() (map[string]interface{}, error) {
	data, err := os.ReadFile(GOGIT_INDEX)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("nothing to commit")
		}
		return nil, err
	}

	index := map[string]string{}
	if err = json.Unmarshal(data, &index); len(data) > 0 && err != nil {
		return nil, err
	}

	structuredIndex := make(map[string]interface{})
	for path, oid := range index {
		dirs := strings.Split(path, "/")
		workingMap := structuredIndex
		for i, dir := range dirs {
			if _, ok := workingMap[dir]; !ok {
				if i == len(dirs)-1 {
					workingMap[dir] = oid
					continue
				}
				workingMap[dir] = make(map[string]interface{})
			}

			if assertedMap, ok := (workingMap[dir]).(map[string]interface{}); ok {
				workingMap = assertedMap
			}

		}
	}
	return structuredIndex, nil
}

func (Base) checkoutIndex(index map[string]string) error {
	if err := data.emptyCurrentDir(); err != nil {
		return err
	}

	for path, oid := range index {
		if err := os.MkdirAll(filepath.Dir(path), FP); err != nil && !os.IsExist(err) {
			return err
		}
		if oid == "" {
			return fmt.Errorf("empty oid: %v", index)
		}

		obj, err := data.GetObject(oid, BLOB)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, obj, FP); err != nil {
			return err
		}
	}
	return nil
}

func (Base) printStructuredIndex(m map[string]interface{}, level int) {
	for k, v := range m {
		if vMap, ok := v.(map[string]interface{}); ok {
			fmt.Printf("%sdir: %s\n", strings.Repeat("-> ", level), k)
			base.printStructuredIndex(vMap, level+1)
		} else {
			fmt.Printf("%s%s %s\n", strings.Repeat("  ", level+1), k, v)
		}
	}
}

// GetCommit takes an OID and returns a pointer to a CommitObject
func (Base) GetCommit(oid string) (*CommitObject, error) {
	buf, err := data.GetObject(oid, COMMIT)
	if err != nil {
		return nil, err
	}
	var c CommitObject
	fields := strings.Split(string(buf), "\n")
	var parents []string
	for _, field := range fields {
		split := strings.SplitN(field, " ", 2)
		if len(split) != 2 {
			continue
		}
		key, value := split[0], split[1]
		switch key {
		case "tree":
			c.TreeOid = value
		case "parent":
			parents = append(parents, value)
		case "time":
			t, _ := time.Parse(time.Layout, value)
			c.Timestamp = t
		case "message":
			c.Message = value
		default:
			return nil, fmt.Errorf("unknown key %s", key)
		}
	}
	c.ParentOids = parents
	return &c, nil
}

func (Base) printCommit(oid string, commit CommitObject, refs []string) {
	oid, _ = base.GetOid(oid)
	var refsStr string
	if len(refs) > 0 {
		refsStr = fmt.Sprintf(" <- (%s)", strings.Join(refs, ", "))
	}
	fmt.Printf("commit: %s%s\n", oid, refsStr)
	fmt.Printf("message: \"%s\"\n\n", commit.Message)
}

func (Base) GetOid(name string) (string, error) {
	// Alias @ to HEAD
	if name == "@" {
		name = HEAD
	}

	for _, dir := range localRefDirs {
		fp := filepath.Join(dir, name)
		if ref, err := data.GetRef(fp, false); err == nil && ref != nil && len(ref.Value) > 0 {
			ref, err = data.GetRef(fp, true)
			if err != nil {
				return "", err
			}
			return ref.Value, nil
		}
	}
	if data.isValidSHA1(name) {
		return name, nil
	}
	return "", &NoOIDFoundError{fmt.Sprintf("no oid found with ref: %s", name)}
}

// Init initializes the gogit repository, and points HEAD toward a new branch "main"
func (Base) Init() error {
	if err := data.Init(); err != nil {
		return err
	}
	return data.UpdateRef(HEAD, &RefValue{true, BASE_BRANCH}, true)
}

func (Base) ReadTree(treeOid string, updateWorkingDir bool) error {
	return data.WithIndex(
		func(_ map[string]string) (map[string]string, error) {
			index := map[string]string{}
			for path, oid := range base.GetTree(treeOid, ".") {
				index[path] = oid
			}

			if !updateWorkingDir {
				return index, nil
			}

			return index, base.checkoutIndex(index)
		})
}

func (Base) WriteTree(dir string) (string, error) {
	index, err := base.getStructuredIndex()
	if err != nil {
		return "", err
	}

	// Write tree from index file
	var writeTreeRecursive func(index map[string]interface{}) (string, error)
	writeTreeRecursive = func(index map[string]interface{}) (string, error) {
		var entries []TreeEntry
		for name, value := range index {
			var oid, _type string
			if subdirIndex, ok := value.(map[string]interface{}); ok {
				_type = TREE
				oid, _ = writeTreeRecursive(subdirIndex)
			} else {
				_type = BLOB
				oid = value.(string)
			}
			entries = append(entries, TreeEntry{name, oid, _type})
		}

		var tree string
		for _, entry := range entries {
			tree += fmt.Sprintf("%s %s %s\n", entry.Name, entry.Oid, entry.Type)
		}
		return data.HashObject([]byte(tree), TREE)
	}
	return writeTreeRecursive(index)
}

func (Base) GetTree(oid, basePath string) Tree {
	result := make(Tree)
	for _, entry := range base.iterTreeEntries(oid) {
		path := filepath.Join(basePath, entry.Name)
		switch entry.Type {
		case BLOB:
			result[path] = entry.Oid
		case TREE:
			maps.Copy(result, base.GetTree(entry.Oid, fmt.Sprintf("%s/", path)))
		default:
			fmt.Printf("error: unknown tree entry %s", entry.Type)
			return nil
		}
	}
	return result
}

func (Base) GetIndexTree() (Tree, error) {
	var indexTree Tree
	err := data.WithIndex(
		func(index map[string]string) (map[string]string, error) {
			indexTree = index
			return indexTree, nil
		})
	return indexTree, err
}

func (Base) GetWorkingTree() (Tree, error) {
	res := make(Tree)
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, e error) error {
		if data.isIgnored(path) || d.IsDir() {
			return nil
		}
		buf, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		oid, err := data.HashObject(buf, BLOB)
		if err != nil {
			return err
		}
		res[path] = oid
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (Base) ReadTreeMerged(
	baseTreeOid string,
	headTreeOid string,
	mergeTreeOid string,
	updateWorkingDir bool,
) error {
	return data.WithIndex(
		func(index map[string]string) (map[string]string, error) {
			mergedTree, err := diff.MergeTrees(base.GetTree(baseTreeOid, ""), base.GetTree(headTreeOid, ""), base.GetTree(mergeTreeOid, ""))
			if err != nil {
				return nil, err
			}

			if !updateWorkingDir {
				return mergedTree, nil
			}
			return mergedTree, base.checkoutIndex(mergedTree)
		})
}

func (Base) Commit(message string, timestamp time.Time) (string, error) {
	tree, err := base.WriteTree(".")
	if err != nil {
		return "", err
	}

	var parents []string
	headRef, _ := data.GetRef(HEAD, true)
	if headRef.Value != "" {
		parents = append(parents, headRef.Value)
	}
	mergeHeadRef, _ := data.GetRef(MERGE_HEAD, true)
	if mergeHeadRef.Value != "" {
		parents = append(parents, mergeHeadRef.Value)
		err = data.DeleteRef(MERGE_HEAD, false)
		if err != nil {
			return "", err
		}
	}
	c := CommitObject{tree, parents, timestamp, message}
	oid, err := data.HashObject([]byte(c.String()), COMMIT)
	if err != nil {
		return "", err
	}
	err = data.UpdateRef(HEAD, &RefValue{false, oid}, true)
	return oid, err
}

func (Base) Log(oid string) error {
	refs := make(map[string][]string)
	for refName, ref := range data.iterRefs("", true) {
		refs[ref.Value] = append(refs[ref.Value], refName)
	}

	oid, _ = base.GetOid(oid)
	for oidItr := range base.iterCommitsAndParents([]string{oid}) {
		c, err := base.GetCommit(oidItr)
		if err != nil {
			return err
		}
		base.printCommit(oidItr, *c, refs[oidItr])
	}
	return nil
}

func (Base) Checkout(name string) error {
	oid, err := base.GetOid(name)

	if err != nil {
		if _, ok := err.(*NoOIDFoundError); !ok {
			return err
		}
	}

	if oid != "" {
		c, err := base.GetCommit(oid)
		if err != nil {
			return err
		}

		if err := base.ReadTree(c.TreeOid, true); err != nil {
			return err
		}
	}

	var headRef *RefValue
	if base.isBranch(name) || oid == "" {
		headRef = &RefValue{true, fmt.Sprintf("refs/heads/%s", name)}
	} else {
		headRef = &RefValue{false, oid}
	}
	return data.UpdateRef(HEAD, headRef, false)
}

// Reset moves HEAD to the oid passed as a parameter
func (Base) Reset(oid string) error {
	oid, err := base.GetOid(oid)
	if err != nil {
		return err
	}
	return data.UpdateRef(HEAD, &RefValue{false, oid}, true)
}

func (Base) CreateTag(name, oid string) error {
	if oid == "" {
		oid = "@"
	}
	return data.UpdateRef(fmt.Sprintf("refs/tags/%s", name), &RefValue{false, oid}, true)
}

func (Base) CreateBranch(name, baseName string) error {
	oid, err := base.GetOid(baseName)
	if err != nil {
		return err
	}
	return data.UpdateRef(filepath.Join("refs/heads", name), &RefValue{false, oid}, true)
}

func (Base) GetBranch() (string, error) {
	headRef, err := data.GetRef(HEAD, false)
	if err != nil || !headRef.Symbolic {
		return "", err
	}
	return filepath.Base(headRef.Value), nil
}

// Performs 3-way merge
func (Base) Merge(oid string) error {
	headRef, err := data.GetRef(HEAD, true)
	if err != nil {
		return err
	}

	oid, err = base.GetOid(oid)
	if err != nil {
		return err
	}
	commit, err := base.GetCommit(oid)
	if err != nil {
		return err
	}

	mergeBaseOID, err := base.getMergeBase(oid, headRef.Value)
	if err != nil {
		return err
	}

	// Fast-forward merge
	if mergeBaseOID == headRef.Value {
		fmt.Println("fast-forward merge")
		err := base.ReadTree(commit.TreeOid, true)
		if err != nil {
			return err
		}
		return data.UpdateRef(HEAD, &RefValue{false, oid}, true)
	}

	headCommit, err := base.GetCommit(headRef.Value)
	if err != nil {
		return err
	}
	mergeBaseCommit, err := base.GetCommit(mergeBaseOID)
	if err != nil {
		return err
	}
	err = data.UpdateRef(MERGE_HEAD, &RefValue{false, oid}, true)
	if err != nil {
		return err
	}
	return base.ReadTreeMerged(mergeBaseCommit.TreeOid, headCommit.TreeOid, commit.TreeOid, true)
}

func (Base) getMergeBase(oid1, oid2 string) (string, error) {
	oid1Parents := ds.NewSet([]string{})
	for parent := range base.iterCommitsAndParents([]string{oid1}) {
		oid1Parents.Add(parent)
	}

	for parent := range base.iterCommitsAndParents([]string{oid2}) {
		if oid1Parents.Includes(parent) {
			return parent, nil
		}
	}
	return "", fmt.Errorf("no common ancestor found for oids %s and %s", oid1, oid2)
}

func (Base) Rebase(oid string) error {
	headRef, err := data.GetRef(HEAD, true)
	if err != nil {
		return err
	}

	oid, err = base.GetOid(oid)
	if err != nil {
		return err
	}

	mergeBaseOID, err := base.getMergeBase(oid, headRef.Value)
	if err != nil {
		return err
	}

	// commits in current branch, not in the history of OID
	commitOIDs := base.getRebaseCommits(oid, headRef.Value)
	err = data.UpdateRef(HEAD, &RefValue{false, oid}, true)
	if err != nil {
		return err
	}

	headCommit, err := base.GetCommit(headRef.Value)
	if err != nil {
		return err
	}
	mergeBaseCommit, err := base.GetCommit(mergeBaseOID)
	if err != nil {
		return err
	}

	for _, commitOID := range commitOIDs {
		commit, err := base.GetCommit(commitOID)
		if err != nil {
			return err
		}
		if err = base.ReadTreeMerged(mergeBaseCommit.TreeOid, headCommit.TreeOid, commit.TreeOid, true); err != nil {
			return err
		}

		if _, err = base.Commit(commit.Message, commit.Timestamp); err != nil {
			return err
		}
	}
	return nil
}

func (Base) Add(filenames ...string) error {
	addFile := func(filename string, index map[string]string) error {
		if data.isIgnored(filename) {
			return nil
		}

		f, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		oid, err := data.HashObject(f, BLOB)
		if err != nil {
			return err
		}
		index[filename] = oid
		return nil
	}

	addDir := func(filename string, index map[string]string) error {
		return filepath.WalkDir(filename, func(path string, d fs.DirEntry, e error) error {
			if d.IsDir() {
				return nil
			}
			return addFile(path, index)
		})
	}

	return data.WithIndex(
		func(index map[string]string) (map[string]string, error) {
			for _, filename := range filenames {
				if info, err := os.Stat(filename); err == nil {
					if info.IsDir() {
						if err = addDir(filename, index); err != nil {
							return nil, err
						}
					} else {
						if err = addFile(filename, index); err != nil {
							return nil, err
						}
					}
				} else {
					return nil, err
				}
			}
			return index, nil
		})
}

func (Base) getRebaseCommits(oid1, oid2 string) []string {
	oid1Parents := ds.NewSet([]string{})
	for parent := range base.iterCommitsAndParents([]string{oid1}) {
		oid1Parents.Add(parent)
	}

	commits := []string{}
	for parent := range base.iterCommitsAndParents([]string{oid2}) {
		if !oid1Parents.Includes(parent) {
			commits = append(commits, parent)
		}
	}
	return commits
}

func (Base) K() error {
	dot := "digraph commits {\n"
	oids := ds.NewSet([]string{})

	for refName, ref := range data.iterRefs("", false) {
		dot += fmt.Sprintf("\"%s\" [shape=note]\n", refName)
		dot += fmt.Sprintf("\"%s\" -> \"%s\"\n", refName, ref.Value)
		if !ref.Symbolic {
			oids.Add(ref.Value)
		}
	}

	for oid := range base.iterCommitsAndParents(oids.ToArray()) {
		c, _ := base.GetCommit(oid)
		dot += fmt.Sprintf("\"%s\" [shape=box style=filled label=\"%s\"]\n", oid, oid[:10])
		for _, parent := range c.ParentOids {
			dot += fmt.Sprintf("\"%s\" -> \"%s\"\n", oid, parent)
		}
	}

	dot += "}"
	fmt.Println(dot)
	return nil
}
