// nolint
package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// If this changes, you must change the "tests" Make target as well
const TEST_DIR = "/tmp/gogit_test"

type ctxKey int

const TestName ctxKey = iota

// ** Test Helpers **
func expectEquals[T comparable](t *testing.T, ctx context.Context, received, target T) {
	if target != received {
		cleanup(t, fmt.Errorf("%s:\nexpected: \"%v\"\nreceived: \"%v\"", ctx.Value(TestName), target, received))
	}
}

func expectNotEquals[T comparable](t *testing.T, ctx context.Context, received, target T) {
	if target == received {
		cleanup(t, fmt.Errorf("%s:\nreceived: \"%v\"\nshould not equal: \"%v\"", ctx.Value(TestName), received, target))
	}
}

func expectExists(t *testing.T, ctx context.Context, path string, shouldExist bool) {
	fp := filepath.Join(TEST_DIR, path)
	_, err := os.Stat(fp)
	if err != nil && shouldExist {
		cleanup(t, fmt.Errorf("%s:\n%s does not exist", ctx.Value(TestName), fp))
	}

	if err == nil && !shouldExist {
		cleanup(t, fmt.Errorf("%s:\n%s exists", ctx.Value(TestName), fp))
	}
}

func expectDirLength(t *testing.T, ctx context.Context, dirPath string, length int) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		cleanup(t, fmt.Errorf("%s:\n%s", ctx.Value(TestName), err))
	}
	if len(files) != length {
		cleanup(t, fmt.Errorf("%s:\nexpected %d file(s)\nreceived %d file(s)", ctx.Value(TestName), length, len(files)))
	}
}

func expectOutput(t *testing.T, ctx context.Context, fn func(), expected string) {
	rescueStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = rescueStdout
	expectEquals(t, ctx, string(out), expected)
}

// ** End Test Helpers **

// ** Setup/Teardown Helpers **
func setupInit() error {
	COMPRESS_OBJECTS = false

	initDirs := []string{
		filepath.Join(GOGIT_ROOT, "objects"),
		filepath.Join(GOGIT_ROOT, "refs", "heads"),
		filepath.Join(GOGIT_ROOT, "refs", "tags"),
	}

	for _, dir := range initDirs {
		err := os.MkdirAll(dir, FP)
		if err != nil {
			return err
		}
	}

	return setHEAD("main")
}

func setupCreateObject(oid string, content []byte) error {
	return os.WriteFile(filepath.Join(TEST_DIR, GOGIT_DIR, "objects", oid), content, FP)
}

func setupCreateFile(path string, content []byte, addToIndex bool) error {
	if err := os.MkdirAll(filepath.Join(TEST_DIR, filepath.Dir(path)), FP); err != nil {
		return err
	}

	// Write file to index and objects
	if addToIndex {
		oid := getOid(content, BLOB)
		content, _ := json.Marshal(map[string]string{path: oid})
		setupCreateObject(oid, []byte(fmt.Sprintf("blob\x00%s", content)))

		if err := os.WriteFile(GOGIT_INDEX, content, FP); err != nil {
			return err
		}
	}

	return os.WriteFile(filepath.Join(TEST_DIR, path), content, FP)
}

func setupCommit(branch string, commitOID string, parentOID string, blobs map[string][]byte) {
	// Create blobs
	blobOIDs := map[string]string{}
	for path, content := range blobs {
		setupCreateFile(path, content, true)

		blobOID := getOid(content, BLOB)
		setupCreateObject(blobOID, []byte(fmt.Sprintf("blob\x00%s", content)))
		blobOIDs[path] = blobOID
	}

	// Create tree
	var treeContent string
	for path, blobOID := range blobOIDs {
		treeContent += fmt.Sprintf("%s %s blob\n", path, blobOID)
	}

	treeOID := getOid([]byte(treeContent), TREE)
	setupCreateObject(treeOID, []byte(fmt.Sprintf("tree\x00%s", treeContent)))

	var parentString string
	if parentOID != "" {
		parentString = fmt.Sprintf("\nparent %s", parentOID)
	}

	// Create commit
	setupCreateObject(commitOID, []byte(fmt.Sprintf("commit\x00tree %s\ntime 12:00%s\nmessage XXX", treeOID, parentString)))

	os.MkdirAll(filepath.Join(GOGIT_DIR, "refs", "heads"), FP)
	os.WriteFile(filepath.Join(GOGIT_DIR, "refs", "heads", branch), []byte(commitOID), FP)
	setHEAD(branch)
}

func setupBranch(name string, branchOff string, numCommits int) {
	var parent string
	if branchOff != "" {
		parent = inspectRef(fmt.Sprintf("refs/heads/%s", branchOff))
	}

	for i := range numCommits {
		commitOID := fmt.Sprintf("%s-commit-%d", name, i+1)
		setupCommit(name, commitOID, parent, map[string][]byte{
			"test-1.txt": []byte(fmt.Sprintf("Testing branch \"%s\" commit %d!", name, i+1)),
			"test-2.txt": []byte(fmt.Sprintf("Testing branch \"%s\" commit %d!", name, i+1)),
			"test-3.txt": []byte(fmt.Sprintf("Testing branch \"%s\" commit %d!", name, i+1)),
		})
		parent = commitOID
	}

	os.MkdirAll(filepath.Join(GOGIT_DIR, "refs", "heads"), FP)
	os.WriteFile(filepath.Join(GOGIT_DIR, "refs", "heads", name), []byte(parent), FP)
	// Set HEAD back to main
	setHEAD("main")
}

func inspectRef(ref string) string {
	buf, err := os.ReadFile(filepath.Join(GOGIT_DIR, ref))
	if err != nil {
		return ""
	}
	return string(buf)
}

func inspectIndex() map[string]string {
	buf, err := os.ReadFile(GOGIT_INDEX)
	if err != nil {
		return nil
	}

	index := map[string]string{}
	if err = json.Unmarshal(buf, &index); err != nil {
		return nil
	}
	return index
}

func inspectFile(filePath string) []byte {
	content, err := os.ReadFile(filepath.Join(TEST_DIR, filePath))
	if err != nil {
		return []byte{}
	}
	return content
}

func setHEAD(refName string) error {
	return os.WriteFile(filepath.Join(GOGIT_DIR, HEAD), []byte(fmt.Sprintf("ref: refs/heads/%s", refName)), FP)
}

func getOid(data []byte, _type string) string {
	hasher := sha1.New()
	hasher.Write([]byte(fmt.Sprintf("%s\x00%s", _type, data)))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func cleanup(t *testing.T, err error) {
	os.RemoveAll(GOGIT_DIR)

	filepath.WalkDir(TEST_DIR, func(path string, _ fs.DirEntry, _ error) error {
		if path != TEST_DIR {
			return os.RemoveAll(path)
		}
		return nil
	})

	if err != nil {
		t.Error(err)
		t.FailNow()
	}
}

// ** End Setup/Teardown Helpers **

func init() {
	err := os.Chdir(TEST_DIR)
	if err != nil {
		panic(err)
	}
}

func Test_CLI(t *testing.T) {
	testcases := []struct {
		Name    string
		Args    CLIArgs
		Flags   CLIFlags
		Setup   func()
		Cleanup func()
		Run     func(args CLIArgs, flags CLIFlags)
	}{
		{
			Name:  "Init",
			Args:  CLIArgs{},
			Flags: CLIFlags{},
			Setup: func() {},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Init")
				if err := cli.Init(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, GOGIT_DIR, true)
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "objects"), true)
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "heads"), true)
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "tags"), true)
			},
		},
		{
			Name:  "Checkout - New Branch",
			Args:  CLIArgs{},
			Flags: CLIFlags{"branch": "new-branch"},
			Setup: func() {
				setupInit()
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Checkout - New Branch")
				if err := cli.Checkout(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/new-branch")
			},
		},
		{
			Name:  "Checkout - Existing Branch",
			Args:  CLIArgs{"existing-branch"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupBranch("existing-branch", "main", 1)
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Checkout - Existing Branch")

				// Change content of file
				os.WriteFile(filepath.Join(TEST_DIR, "test-1.txt"), []byte("Goodbye World!"), FP)
				expectEquals(t, ctx, string(inspectFile("test-1.txt")), "Goodbye World!")

				if err := cli.Checkout(args, flags); err != nil {
					cleanup(t, err)
				}

				expectEquals(t, ctx, string(inspectFile("test-1.txt")), "Testing branch \"existing-branch\" commit 1!")
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/existing-branch")
			},
		},
		{
			Name:  "Tag",
			Args:  CLIArgs{"new-tag", "commit-1"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "commit-1", "", map[string][]byte{"test.txt": []byte("Hello World!")})
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Tag")
				if err := cli.Tag(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "tags"), true)
				expectEquals(t, ctx, inspectRef("refs/tags/new-tag"), "commit-1")
			},
		},
		{
			Name:  "Branch - Lists Branches",
			Args:  CLIArgs{},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "commit-1", "", map[string][]byte{"test.txt": []byte("Hello World!")})
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Branch - Lists Branches")
				expectOutput(t, ctx, func() {
					cli.Branch(args, flags)
				}, "* main\n")
			},
		},
		{
			Name:  "Branch - New Branch",
			Args:  CLIArgs{"new-branch"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "commit-1", "", map[string][]byte{"test.txt": []byte("Hello World!")})
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Branch - New Branch")
				if err := cli.Branch(args, flags); err != nil {
					cleanup(t, err)
				}

				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/main")
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "heads", "new-branch"), true)
				expectEquals(t, ctx, inspectRef("refs/heads/new-branch"), "commit-1")
			},
		},
		{
			Name:  "Add - Individual File",
			Args:  CLIArgs{"test.txt"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCreateFile("test.txt", []byte("Hello World!"), false)
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Add - Individual File")
				if err := cli.Add(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "index"), true)
				expectEquals(t, ctx, inspectIndex()["test.txt"], getOid([]byte("Hello World!"), BLOB))
			},
		},
		{
			Name:  "Add - Directory",
			Args:  CLIArgs{"."},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCreateFile("test.txt", []byte("Hello World!"), false)
				setupCreateFile("./subdir1/test_subdir1_1.txt", []byte("Hello World Subdir1_1!"), false)
				setupCreateFile("./subdir1/test_subdir1_2.txt", []byte("Hello World Subdir1_2!"), false)
				setupCreateFile("./subdir2/test_subdir2_1.txt", []byte("Hello World Subdir2_1!"), false)
				setupCreateFile("./subdir2/subsubdir1/test_subdir2_subsubdir1_1.txt", []byte("Hello World Subdir2_SubSubdir1_1!"), false)
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Add - Directory")
				if err := cli.Add(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "index"), true)
				expectEquals(t, ctx, len(inspectIndex()), 5)
			},
		},
		{
			Name:  "Commit",
			Args:  CLIArgs{},
			Flags: CLIFlags{"message": "first commit"},
			Setup: func() {
				setupInit()
				setupCreateFile("test.txt", []byte("Hello World!"), true)
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Commit")
				if err := cli.Commit(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/main")
				expectDirLength(t, ctx, filepath.Join(GOGIT_DIR, "objects"), 3) // commit, tree, blob
			},
		},
		{
			Name:  "Merge and Commit",
			Args:  CLIArgs{"main"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "main-commit-1", "", map[string][]byte{"test-1.txt": []byte("Hello World!")})
				// Create branch off main with 2 commits
				setupBranch("first-branch", "main", 2)
				// Make another commit on main
				setupCommit("main", "main-commit-2", "main-commit-1", map[string][]byte{"test-1.txt": []byte("Hello World Again!")})
				// set HEAD to new branch
				setHEAD("first-branch")
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Merge and Commit")
				if err := cli.Merge(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, MERGE_HEAD), true)
				expectEquals(t, ctx, inspectRef(MERGE_HEAD), "main-commit-2")
				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/first-branch")

				if err := cli.Commit(CLIArgs{}, CLIFlags{"message": "merge commit"}); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, MERGE_HEAD), false)
				commit := inspectFile(filepath.Join(GOGIT_DIR, "objects", inspectRef("refs/heads/first-branch")))
				expectEquals(t, ctx, strings.Count(string(commit), "parent"), 2)
			},
		},
		{
			Name:  "Merge - Fast Forward",
			Args:  CLIArgs{"main"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "main-commit-1", "", map[string][]byte{"test-1.txt": []byte("Hello World!")})
				// Create branch off main with 0 commits
				setupBranch("first-branch", "main", 0)
				// Make another commit on main
				setupCommit("main", "main-commit-2", "main-commit-1", map[string][]byte{"test-1.txt": []byte("Hello World Again!")})
				// set HEAD to new branch
				setHEAD("first-branch")
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Merge - Fast Forward")
				if err := cli.Merge(args, flags); err != nil {
					cleanup(t, err)
				}

				expectExists(t, ctx, filepath.Join(GOGIT_DIR, MERGE_HEAD), false)
				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/first-branch")
				expectEquals(t, ctx, inspectRef("refs/heads/first-branch"), "main-commit-2")
			},
		},
		{
			Name:  "Rebase",
			Args:  CLIArgs{"main"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("main", "main-commit-1", "", map[string][]byte{"test-1.txt": []byte("Hello World!")})
				// Create branch off main with 2 commits
				setupBranch("first-branch", "main", 2)
				// Make another commit on main
				setupCommit("main", "main-commit-2", "main-commit-1", map[string][]byte{"test-1.txt": []byte("Hello World Again!")})
				// set HEAD to new branch
				setHEAD("first-branch")
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "Rebase")
				if err := cli.Rebase(args, flags); err != nil {
					cleanup(t, err)
				}

				expectEquals(t, ctx, inspectRef(HEAD), "ref: refs/heads/first-branch")

				// Rebase applies new commits so the commit ids of branch "first-branch" should be different
				currRef := inspectRef("refs/heads/first-branch")
				expectNotEquals(t, ctx, currRef, "first-branch-commit-2")

				commit, _ := base.GetCommit(currRef)
				currRef = commit.ParentOids[0]
				expectNotEquals(t, ctx, currRef, "first-branch-commit-1")

				commit, _ = base.GetCommit(currRef)
				currRef = commit.ParentOids[0]
				expectEquals(t, ctx, currRef, "main-commit-2")

				commit, _ = base.GetCommit(currRef)
				currRef = commit.ParentOids[0]
				expectEquals(t, ctx, currRef, "main-commit-1")
			},
		},
		{
			Name:  "GC",
			Args:  CLIArgs{},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()

				setupCommit("new-branch-1", "new-branch-1-commit-1", "", map[string][]byte{
					"test-branch-1.txt": []byte("Hello World 1!"),
				})

				setupCommit("new-branch-2", "new-branch-2-commit-1", "", map[string][]byte{
					"test-branch-2.txt": []byte("Hello World 2!"),
				})

				// Remove new-branch-1 ref (commit, tree, and blob are now unreachable)
				if err := os.Remove(filepath.Join(GOGIT_DIR, "refs", "heads", "new-branch-1")); err != nil {
					cleanup(t, err)
				}
			},
			Cleanup: func() {
				cleanup(t, nil)
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), TestName, "GC")
				if err := cli.GC(args, flags); err != nil {
					cleanup(t, err)
				}

				// commit, tree, and test.txt blob from new-branch-2 should not be deleted
				expectDirLength(t, ctx, filepath.Join(GOGIT_DIR, "objects"), 3)
				oid := inspectRef("refs/heads/new-branch-2")
				expectNotEquals(t, ctx, oid, "")
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "objects", oid), true)
			},
		},
	}

	for _, test := range testcases {
		t.Logf("Test: %s\n", test.Name)
		test.Setup()
		test.Run(test.Args, test.Flags)
		test.Cleanup()
	}
}
