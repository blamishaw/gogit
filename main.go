package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CLI = Command Line Interface
type CLI struct{}

var cli CLI

type CLIArgs []string
type CLIFlags map[string]any

type Command struct {
	fn              func(args CLIArgs, flags CLIFlags) error
	requiredNumArgs int
	commandFlags    map[string]bool // Key: flag name, Value: required
}

func Exec(cmd Command, args CLIArgs, flags CLIFlags) error {
	if len(args) < cmd.requiredNumArgs {
		return GogitError{message: fmt.Sprintf("not enough specified args, expected %d and received %d\n", cmd.requiredNumArgs, len(args))}
	}

	for flagName, required := range cmd.commandFlags {
		if _, ok := flags[flagName]; !ok && required {
			return GogitError{message: fmt.Sprintf("missing required flag \"%s\"", flagName)}
		}
	}

	for flagName := range flags {
		if _, ok := cmd.commandFlags[flagName]; !ok {
			return GogitError{message: fmt.Sprintf("unknown flag \"%s\"", flagName)}
		}
	}

	if err := cmd.fn(args, flags); err != nil {
		return GogitError{message: err.Error()}
	}

	return nil
}

func (CLI) Init(_ CLIArgs, _ CLIFlags) error {
	if err := base.Init(); err != nil {
		return err
	}
	fmt.Printf("Initialized empty gogit repository in %s\n", GOGIT_ROOT)
	return nil
}

func (CLI) CatFile(args CLIArgs, _ CLIFlags) error {
	fp := args[0]
	buf, _, err := data.GetObject(fp)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", string(buf))
	return nil
}

func (CLI) Commit(_ CLIArgs, flags CLIFlags) error {
	message := flags["message"].(string)
	if message == "" {
		return fmt.Errorf("message must have length greater than 0")
	}

	oid, err := base.Commit(message, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("commit: %s\n", oid)
	return nil
}

func (CLI) Log(args CLIArgs, _ CLIFlags) error {
	var oid string
	if len(args) > 1 {
		oid = args[0]
	} else {
		oid = "@"
	}
	return base.Log(oid)
}

func (CLI) Checkout(args CLIArgs, flags CLIFlags) error {
	branchFlagName, branchFlagExists := flags["branch"].(string)
	if !branchFlagExists && len(args) == 0 {
		return fmt.Errorf("not enough args, require -b or branch name")
	}

	var branchName, msgPrefix string
	if branchFlagExists {
		branchName = branchFlagName
		msgPrefix = " a new"
	} else {
		branchName = args[0]
	}

	currentBranch, err := base.GetBranch()
	if err != nil {
		return err
	}

	if currentBranch == branchName {
		fmt.Printf("Already on %s\n", branchName)
		return nil
	}

	if err := base.Checkout(branchName, branchFlagExists); err != nil {
		return err
	}

	fmt.Printf("Switched to%s branch '%s'\n", msgPrefix, branchName)
	return nil
}

func (CLI) Tag(args CLIArgs, _ CLIFlags) error {
	name, oid := args[0], args[1]
	if err := base.CreateTag(name, oid); err != nil {
		return err
	}
	fmt.Println(name)
	return nil
}

func (CLI) K(_ CLIArgs, _ CLIFlags) error {
	return base.K()
}

func (CLI) Branch(args CLIArgs, _ CLIFlags) error {
	if len(args) == 0 {
		currentBranch, err := base.GetBranch()
		if err != nil {
			return err
		}

		branchIter, err := base.iterBranches()
		if err != nil {
			return err
		}

		for branchName, _ := range branchIter {
			if branchName == currentBranch {
				fmt.Printf("* %s\n", branchName)
			} else {
				fmt.Printf("%s\n", branchName)
			}
		}
		return nil
	}

	name := args[0]
	if err := base.CreateBranch(name, "@"); err != nil {
		return err
	}
	fmt.Printf("new branch %s created at HEAD\n", args[0])
	return nil
}

func (CLI) Status(_ CLIArgs, _ CLIFlags) error {
	headOID, err := base.GetOid("@")
	if err != nil {
		return err
	}
	branch, err := base.GetBranch()
	if err != nil {
		return err
	}

	if branch != "" {
		fmt.Printf("On branch %s\n", branch)
	} else {
		fmt.Printf("HEAD detatched at %s\n", headOID[:10])
	}

	mergeHeadRef, err := data.GetRef(MERGE_HEAD, true)
	if err != nil {
		return err
	}

	if mergeHeadRef.Value != "" {
		fmt.Printf("Merging with %s\n", mergeHeadRef.Value[:10])
	}

	headTreeOID := ""
	if headOID != "" {
		headCommit, err := base.GetCommit(headOID)
		if err != nil {
			return err
		}
		headTreeOID = headCommit.TreeOid
	}

	workingTree, err := base.GetWorkingTree()
	if err != nil {
		return err
	}

	indexTree, err := base.GetIndexTree()
	if err != nil {
		return err
	}

	fmt.Printf("\nChanges to be commited:\n")

	headTree, err := base.GetTree(headTreeOID, "")
	if err != nil {
		return err
	}

	for path, action := range diff.iterChangedFiles(headTree, indexTree) {
		diff.PrettyPrint(fmt.Sprintf("%s: %s", action, path))
	}

	fmt.Printf("\nChanges not staged for commit:\n")
	for path, action := range diff.iterChangedFiles(indexTree, workingTree) {
		diff.PrettyPrint(fmt.Sprintf("%s: %s", action, path))
	}
	return nil
}

func (CLI) Reset(args CLIArgs, _ CLIFlags) error {
	oid := args[0]
	if err := base.Reset(oid); err != nil {
		return err
	}
	fmt.Printf("%s\n", oid)
	return nil
}

func (CLI) Show(args CLIArgs, _ CLIFlags) error {
	oid := args[0]
	c, err := base.GetCommit(oid)
	if err != nil {
		return err
	}

	var parentTreeOid string
	if len(c.ParentOids) > 0 {
		parentCommit, err := base.GetCommit(c.ParentOids[0])
		if err != nil {
			return err
		}
		parentTreeOid = parentCommit.TreeOid
	}
	base.printCommit(oid, *c, []string{})

	parentTree, err := base.GetTree(parentTreeOid, "")
	if err != nil {
		return err
	}

	tree, err := base.GetTree(c.TreeOid, "")
	if err != nil {
		return err
	}

	out, err := diff.DiffTrees(parentTree, tree)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(out), "\n") {
		diff.PrettyPrint(line)
	}
	return nil
}

func (CLI) Diff(args CLIArgs, flags CLIFlags) error {
	commitProvided := len(args) > 0

	var treeFrom, treeTo Tree
	if commitProvided {
		commitOid := args[0]
		commit, err := base.GetCommit(commitOid)
		if err != nil {
			return err
		}

		treeFrom, err = base.GetTree(commit.TreeOid, "")
		if err != nil {
			return err
		}
	}

	var err error
	if value, ok := flags["cached"].(bool); ok && value {
		treeTo, err = base.GetIndexTree()
		if err != nil {
			return err
		}

		if !commitProvided {
			oid, err := base.GetOid("@")
			if err != nil {
				return err
			}
			if len(oid) > 0 {
				c, err := base.GetCommit(oid)
				if err != nil {
					return err
				}
				treeFrom, err = base.GetTree(oid, c.TreeOid)
				if err != nil {
					return err
				}
			}

		}
	} else {
		treeTo, err = base.GetWorkingTree()
		if err != nil {
			return err
		}

		if !commitProvided {
			treeFrom, err = base.GetIndexTree()
			if err != nil {
				return err
			}
		}
	}

	out, err := diff.DiffTrees(treeFrom, treeTo)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(out), "\n") {
		diff.PrettyPrint(line)
	}
	return nil
}

func (CLI) Merge(args CLIArgs, _ CLIFlags) error {
	oid := args[0]
	if err := base.Merge(oid); err != nil {
		return err
	}
	fmt.Println("Merged in working tree. Please commit")
	return nil
}

func (CLI) Rebase(args CLIArgs, _ CLIFlags) error {
	oid := args[0]
	if err := base.Rebase(oid); err != nil {
		return err
	}
	fmt.Printf("rebased off %s\n", oid)
	return nil
}

func (CLI) Fetch(args CLIArgs, _ CLIFlags) error {
	remotePath := args[0]

	if filepath.Base(remotePath) != GOGIT_DIR {
		remotePath = filepath.Join(remotePath, GOGIT_DIR)
		if _, err := os.Stat(remotePath); err != nil {
			return err
		}
	}

	return remote.Fetch(remotePath)
}

func (CLI) Push(args CLIArgs, _ CLIFlags) error {
	remotePath, refName := args[0], args[1]

	if filepath.Base(remotePath) != GOGIT_DIR {
		remotePath = filepath.Join(remotePath, GOGIT_DIR)
		if _, err := os.Stat(remotePath); err != nil {
			return err
		}
	}

	return remote.Push(remotePath, refName)
}

func (CLI) Add(args CLIArgs, _ CLIFlags) error {
	return base.Add(args...)
}

func (CLI) ReadIndex(_ CLIArgs, _ CLIFlags) error {
	idx, err := base.getStructuredIndex()
	if err != nil {
		return err
	}
	base.printStructuredIndex(idx, 0)
	return nil
}

func (CLI) GC(_ CLIArgs, _ CLIFlags) error {
	unreachable, err := base.GC()
	if err != nil {
		return err
	}
	fmt.Printf("Removed %d unreachable objects\n", unreachable)
	return nil
}

func parseFlags(flags CLIFlags, args CLIArgs, flagIdx int) (CLIFlags, CLIArgs, error) {
	if flagIdx < 0 {
		return CLIFlags{}, args, nil
	}

	_ = flag.CommandLine.Parse(args[flagIdx:])

	f := CLIFlags{}
	for name, value := range flags {
		switch v := value.(type) {
		case *string:
			if len(*v) > 0 {
				f[name] = *v
			}
		case *bool:
			if *v {
				f[name] = *v
			}
		default:
			return CLIFlags{}, CLIArgs{}, fmt.Errorf("fatal: unknown flag type %T", v)
		}
	}

	return f, args[:flagIdx], nil
}

func main() {
	var err error
	input := os.Args

	if len(input) < 2 {
		fmt.Println(GogitError{message: "must specify a command"})
		os.Exit(1)
	}

	cmd, args := input[1], input[2:]
	// Parse args up to first flag
	firstArgWithDash := -1
	for i := 0; i < len(args); i++ {
		if len(args[i]) > 0 && args[i][0] == '-' {
			firstArgWithDash = i
			break
		}
	}

	flags := CLIFlags{
		"message": flag.String("m", "", "commit message"),
		"branch":  flag.String("b", "", "branch name"),
		"cached":  flag.Bool("cached", false, "diff using index"),
	}

	flags, args, err = parseFlags(flags, args, firstArgWithDash)
	if err != nil {
		fmt.Println(err)
		return
	}

	var none map[string]bool
	commands := map[string]Command{
		"init":       {cli.Init, 0, none},
		"cat-file":   {cli.CatFile, 1, none},
		"commit":     {cli.Commit, 0, map[string]bool{"message": true}},
		"log":        {cli.Log, 0, none},
		"checkout":   {cli.Checkout, 0, map[string]bool{"branch": false}},
		"tag":        {cli.Tag, 2, none},
		"k":          {cli.K, 0, none},
		"branch":     {cli.Branch, 0, none},
		"status":     {cli.Status, 0, none},
		"reset":      {cli.Reset, 1, none},
		"show":       {cli.Show, 1, none},
		"diff":       {cli.Diff, 0, map[string]bool{"cached": false}},
		"merge":      {cli.Merge, 1, none},
		"rebase":     {cli.Rebase, 1, none},
		"fetch":      {cli.Fetch, 1, none},
		"push":       {cli.Push, 2, none},
		"add":        {cli.Add, 1, none},
		"read-index": {cli.ReadIndex, 0, none},
		"gc":         {cli.GC, 0, none},
	}

	if fn, ok := commands[cmd]; ok {
		err := Exec(fn, args, flags)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		fmt.Println(GogitError{message: fmt.Sprintf("unknown command \"%s\"", cmd)})
		os.Exit(1)
	}
}
