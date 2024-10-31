package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// If this changes, you must change the "tests" Make target as well
const TEST_DIR = "/tmp/gogit_test"

// ** Test Helpers **
func expect[T comparable](t *testing.T, ctx context.Context, received, target T) {
	if target != received {
		t.Errorf("%s: expected: %v, recieved: %v", ctx.Value("name"), target, received)
	}
}

func expectExists(t *testing.T, ctx context.Context, path string, shouldExist bool) {
	fp := filepath.Join(TEST_DIR, path)
	_, err := os.Stat(fp)
	if err != nil && shouldExist {
		t.Errorf("%s: %s does not exist", ctx.Value("name"), fp)
	}

	if err == nil && !shouldExist {
		t.Errorf("%s: %s exists", ctx.Value("name"), fp)
	}
}

func expectDirLength(t *testing.T, ctx context.Context, dirPath string, length int) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		t.Errorf("%s: %s", ctx.Value("name"), err)
	}
	if len(files) != length {
		t.Errorf("%s: expected %d files and received %d", ctx.Value("name"), length, len(files))
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
	expect(t, ctx, string(out), expected)
}

// ** End Test Helpers **

// ** Setup/Teardown Helpers **
func setupInit() error {
	initDirs := []string{
		filepath.Join(TEST_DIR, GOGIT_DIR, "objects"),
		filepath.Join(TEST_DIR, GOGIT_DIR, "refs", "heads"),
		filepath.Join(TEST_DIR, GOGIT_DIR, "refs", "tags"),
	}

	for _, dir := range initDirs {
		err := os.MkdirAll(dir, FP)
		if err != nil {
			return err
		}
	}

	return os.WriteFile(filepath.Join(TEST_DIR, GOGIT_DIR, HEAD), []byte("ref: refs/heads/main"), FP)
}

func setupCreateObject(oid string, content []byte) error {
	return os.WriteFile(filepath.Join(GOGIT_DIR, "objects", oid), content, FP)
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

func setupCommit(commitOID string) {
	filename := "test.txt"
	blobContent := []byte("Hello World!")
	setupCreateFile(filename, blobContent, true)

	// Create blob
	blobOID := getOid(blobContent, BLOB)
	setupCreateObject(blobOID, []byte(fmt.Sprintf("blob\x00%s", blobContent)))

	// Create tree
	treeContent := []byte(fmt.Sprintf("%s %s blob", filename, blobOID))
	treeOID := getOid(treeContent, TREE)
	setupCreateObject(treeOID, []byte(fmt.Sprintf("tree\x00%s", treeContent)))

	// Create commit
	setupCreateObject(commitOID, []byte(fmt.Sprintf("commit\x00tree %s\ntime XXX\nmessage XXX", treeOID)))

	os.MkdirAll(filepath.Join(GOGIT_DIR, "refs", "heads"), FP)
	os.WriteFile(filepath.Join(GOGIT_DIR, "refs", "heads", "main"), []byte(commitOID), FP)
	os.WriteFile(filepath.Join(GOGIT_DIR, HEAD), []byte("ref: refs/heads/main"), FP)
}

func setupBranch(name string) {
	commitOID := "commit-sha-123"
	setupCommit(commitOID)
	os.MkdirAll(filepath.Join(GOGIT_DIR, "refs", "heads"), FP)
	os.WriteFile(filepath.Join(GOGIT_DIR, "refs", "heads", name), []byte(commitOID), FP)
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

func getOid(data []byte, _type string) string {
	hasher := sha1.New()
	hasher.Write([]byte(fmt.Sprintf("%s\x00%s", _type, data)))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func teardownRemoveRoot() error {
	return os.RemoveAll(GOGIT_DIR)
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
		Name     string
		Args     CLIArgs
		Flags    CLIFlags
		Setup    func()
		TearDown func()
		Run      func(args CLIArgs, flags CLIFlags)
	}{
		{
			Name:  "Init",
			Args:  CLIArgs{},
			Flags: CLIFlags{},
			Setup: func() {},
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Init")
				err := cli.Init(args, flags)
				if err != nil {
					t.Error(err)
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
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Checkout - New Branch")
				err := cli.Checkout(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expect(t, ctx, inspectRef(HEAD), "ref: refs/heads/new-branch")
			},
		},
		{
			Name:  "Checkout - Existing Branch",
			Args:  CLIArgs{"existing-branch"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupBranch("existing-branch")
			},
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Checkout - Existing Branch")
				err := cli.Checkout(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expect(t, ctx, inspectRef(HEAD), "ref: refs/heads/existing-branch")
			},
		},
		{
			Name:  "Tag",
			Args:  CLIArgs{"new-tag", "commit-sha-123"},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("commit-sha-123")
			},
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Tag")
				err := cli.Tag(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "tags"), true)
				expect(t, ctx, inspectRef("refs/tags/new-tag"), "commit-sha-123")
			},
		},
		{
			Name:  "Branch - Lists Branches",
			Args:  CLIArgs{},
			Flags: CLIFlags{},
			Setup: func() {
				setupInit()
				setupCommit("commit-sha-123")
			},
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Branch - Lists Branches")
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
				setupCommit("commit-sha-123")
			},
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Branch - New Branch")
				err := cli.Branch(args, flags)
				if err != nil {
					t.Error(err)
				}
				expect(t, ctx, inspectRef(HEAD), "ref: refs/heads/main")
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "refs", "heads", "new-branch"), true)
				expect(t, ctx, inspectRef("refs/heads/new-branch"), "commit-sha-123")
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
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Add - Individual File")
				err := cli.Add(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "index"), true)
				expect(t, ctx, inspectIndex()["test.txt"], getOid([]byte("Hello World!"), BLOB))
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
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Add - Directory")
				err := cli.Add(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, "index"), true)
				expect(t, ctx, len(inspectIndex()), 5)
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
			TearDown: func() {
				teardownRemoveRoot()
			},
			Run: func(args CLIArgs, flags CLIFlags) {
				ctx := context.WithValue(context.Background(), "name", "Commit")
				err := cli.Commit(args, flags)
				if err != nil {
					t.Error(err)
				}
				expectExists(t, ctx, filepath.Join(GOGIT_DIR, HEAD), true)
				expect(t, ctx, inspectRef(HEAD), "ref: refs/heads/main")
				expectDirLength(t, ctx, filepath.Join(GOGIT_DIR, "objects"), 3) // commit, tree, blob
			},
		},
	}

	for _, test := range testcases {
		t.Logf("Test: %s", test.Name)
		test.Setup()
		test.Run(test.Args, test.Flags)
		test.TearDown()
	}
}
