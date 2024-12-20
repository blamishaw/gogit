package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

// FP: File permission
const FP = 0777

// Var so can be overriden in tests
var COMPRESS_OBJECTS = true

type Data struct{}

// Namespacing
var data Data

func (Data) isValidSHA1(digest string) bool {
	if len(digest) != 40 {
		return false
	}

	// Regular expression to match only hex characters
	hexRegex := `^[0-9a-fA-F]{40}$`
	match, _ := regexp.MatchString(hexRegex, digest)
	return match
}

func (Data) isIgnored(path string) bool {
	data, _ := os.ReadFile(filepath.Join(".", ".gogitignore"))
	ignorable := append(strings.Split(string(data), "\n"), GOGIT_DIR)
	for _, fp := range ignorable {
		if fp != "" && strings.Contains(path, fp) {
			return true
		}
	}
	return false
}

func (Data) emptyCurrentDir() error {
	return filepath.WalkDir(".", func(path string, di fs.DirEntry, err error) error {
		if data.isIgnored(path) || strings.Contains(path, GOGIT_DIR) {
			return nil
		}

		if e := os.RemoveAll(path); e != nil {
			if _, ok := e.(*os.PathError); ok {
				return nil
			}
			return e
		}
		// Pass
		return nil
	})
}

// Iterates over all refs and returns a ref name, and pointer to a RefValue object
func (Data) iterRefs(prefix string, deref bool) (iter.Seq2[string, *RefValue], error) {
	refNames := []string{HEAD, MERGE_HEAD}
	refDir := filepath.Join("refs", prefix)
	err := filepath.WalkDir(filepath.Join(GOGIT_ROOT, refDir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			relP, err := filepath.Rel(GOGIT_ROOT, path)
			refNames = append(refNames, relP)
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return func(yield func(string, *RefValue) bool) {
		for _, refName := range refNames {
			if !strings.HasPrefix(refName, refDir) {
				continue
			}
			ref, refErr := data.GetRef(refName, deref)
			if ref.Value == "" {
				continue
			}
			if refErr != nil || !yield(refName, ref) {
				return
			}
		}
	}, nil
}

func (Data) Init() error {
	if err := os.Mkdir(GOGIT_ROOT, FP); err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(GOGIT_ROOT, "objects"),
		filepath.Join(GOGIT_ROOT, "refs", "heads"),
		filepath.Join(GOGIT_ROOT, "refs", "tags"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, FP); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// ChangeRootDir updates the gogit root directory, executes fn, and restores the gogit root
func (Data) ChangeRootDir(newDir string, fn func() error) error {
	prev := GOGIT_ROOT
	GOGIT_ROOT = newDir
	err := fn()
	GOGIT_ROOT = prev
	return err
}

func (Data) WithIndex(
	fn func(index map[string]string) (map[string]string, error),
) error {
	index := make(map[string]string)

	data, err := os.ReadFile(GOGIT_INDEX)
	indexCreated := !os.IsNotExist(err)

	if err != nil {
		if indexCreated {
			return err
		}
	}

	if err = json.Unmarshal(data, &index); len(data) > 0 && err != nil {
		return err
	}

	// Pass copy of index to fn
	// This is important for the reflect.DeepEqual call below
	newIndex, err := fn(maps.Clone(index))
	if err != nil {
		return err
	}

	// No need to rewrite index
	if indexCreated && reflect.DeepEqual(index, newIndex) {
		return nil
	}

	jsonData, err := json.Marshal(newIndex)
	if err != nil {
		return err
	}
	return os.WriteFile(GOGIT_INDEX, jsonData, FP)
}

// HashObject hashes a byte array and return the resulting MD5 hash ID
func (Data) HashObject(data []byte, _type string) (string, error) {
	hasher := sha1.New()
	// Type separated from data by NULL byte
	buf := []byte(fmt.Sprintf("%s\x00%s", _type, data))

	hasher.Write(buf)
	oid := hex.EncodeToString(hasher.Sum(nil))

	fp := filepath.Join(GOGIT_ROOT, "objects", oid)
	// Is this more efficient than just always writing the file?
	if _, err := os.Stat(fp); os.IsExist(err) {
		return oid, nil
	}

	// Zlib compress buffer
	var b bytes.Buffer
	if COMPRESS_OBJECTS {
		w := zlib.NewWriter(&b)
		_, _ = w.Write(buf)
		w.Close()
	} else {
		b = *bytes.NewBuffer(buf)
	}

	if err := os.WriteFile(fp, b.Bytes(), FP); err != nil {
		return "", err
	}
	return oid, nil
}

// GetObject takes an oid and returns the object content and type
func (Data) GetObject(oid string) ([]byte, string, error) {
	data, err := os.ReadFile(filepath.Join(GOGIT_ROOT, "objects", oid))
	if err != nil {
		return []byte{}, "", err
	}

	// Zlib uncompress buffer
	var b bytes.Buffer
	if COMPRESS_OBJECTS {
		compressedBuf := bytes.NewBuffer(data)
		r, err := zlib.NewReader(compressedBuf)
		if err != nil {
			return []byte{}, "", err
		}
		_, _ = io.Copy(&b, r)
		r.Close()
	} else {
		b = *bytes.NewBuffer(data)
	}

	buf := b.Bytes()
	typeIdx := bytes.IndexByte(buf, 0)
	return buf[typeIdx+1:], string(buf[:typeIdx]), nil
}

func (Data) DeleteObject(oid string) error {
	return os.Remove(filepath.Join(GOGIT_DIR, "objects", oid))
}

func (Data) DeleteRef(name string, deref bool) error {
	name, _, err := data.getRefInternal(name, deref)
	if err != nil {
		return err
	}

	if err = os.Remove(filepath.Join(GOGIT_ROOT, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// UpdateRef takes a ref name, RefValue object, and dereference boolean. If deref is true, we drill down
// symbolic refs until we reach an oid
func (Data) UpdateRef(name string, ref *RefValue, deref bool) error {
	name, _, err := data.getRefInternal(name, deref)
	if err != nil {
		return err
	}

	refValue := ref.Value
	if ref.Symbolic {
		refValue = fmt.Sprintf("ref: %s", refValue)
	}

	refPath := filepath.Join(GOGIT_ROOT, filepath.Dir(name))
	return os.WriteFile(filepath.Join(refPath, filepath.Base(name)), []byte(refValue), FP)
}

func (Data) GetRef(name string, deref bool) (*RefValue, error) {
	_, val, err := data.getRefInternal(name, deref)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// getRefInternal returns the ref name, a pointer to a RefValue object specified by the ref name, and an error.
// If the inspected ref is prepended with "ref: ", we recursively drill the symbolic references until we find an oid
func (Data) getRefInternal(name string, deref bool) (string, *RefValue, error) {
	buf, err := os.ReadFile(filepath.Join(GOGIT_ROOT, name))
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}

	value := string(buf)
	refIdx := strings.Index(value, "ref: ")
	symbolic := refIdx > -1
	if symbolic {
		value = value[len("refs: ")-1:]
		if deref {
			return data.getRefInternal(value, deref)
		}
	}
	return name, &RefValue{symbolic, value}, nil
}

// writeTempBlob writes a blob to a temporary file and stores the path in store
func (Data) writeTempBlob(blobOid string, store *[]string) error {
	f, err := os.CreateTemp("/tmp", blobOid)
	// Check if file exists already
	if err != nil {
		if os.IsExist(err) {
			*store = append(*store, f.Name())
			return nil
		}
		return err
	}
	defer f.Close()

	if blobOid != "" {
		blob, t, err := data.GetObject(blobOid)
		if err != nil {
			return err
		}
		if t != BLOB {
			return ObjectTypeError{received: t, expected: BLOB}
		}
		if _, err = f.Write(blob); err != nil {
			return err
		}
	}
	*store = append(*store, f.Name())
	return nil
}

func (Data) ObjectExists(oid string) bool {
	if _, err := os.Stat(filepath.Join(GOGIT_ROOT, "objects", oid)); err == nil {
		return true
	}
	return false
}

func (Data) fetchRemoteObject(oid, remotePath string) error {
	localObjectPath := filepath.Join(GOGIT_ROOT, "objects", oid)

	// If file exists, do not copy over from remote
	if data.ObjectExists(oid) {
		return nil
	}

	buf, err := os.ReadFile(filepath.Join(remotePath, "objects", oid))
	if err != nil {
		return err
	}
	return os.WriteFile(localObjectPath, buf, FP)
}

func (Data) pushRemoteObject(oid, remotePath string) error {
	buf, err := os.ReadFile(filepath.Join(GOGIT_ROOT, "objects", oid))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(remotePath, "objects", oid), buf, FP)
}
