package main

import (
	"fmt"
	ds "local/gogit/data-structures"
	"os"
	"path/filepath"
	"strings"
)

type Remote struct{}

// Namespacing
var remote Remote

func (Remote) Push(remotePath, refName string) error {
	if !strings.HasPrefix(refName, "refs/heads/") {
		refName = "refs/heads/" + refName
	}

	remoteRefs, err := remote.getRemoteRefs(remotePath, "")
	if err != nil {
		return err
	}

	localRef, err := data.GetRef(refName, true)
	if err != nil {
		return err
	}

	// Prevent overwriting unsynced remote changes
	if remoteRefValue, ok := remoteRefs[refName]; ok {
		if !base.isAncestorOf(remoteRefValue, localRef.Value) {
			return fmt.Errorf("remote branch is not an ancestor of local")
		}
	}

	knownRemoteRefs := []string{}
	for _, refValue := range remoteRefs {
		if data.ObjectExists(refValue) {
			knownRemoteRefs = append(knownRemoteRefs, refValue)
		}
	}

	remoteObjects := ds.NewSet([]string{})
	err = base.MapObjectsInCommits(
		knownRemoteRefs,
		func(oid string) error {
			remoteObjects.Add(oid)
			return nil
		},
	)
	if err != nil {
		return err
	}

	objectsToPush := ds.NewSet([]string{})
	err = base.MapObjectsInCommits(
		[]string{localRef.Value},
		func(oid string) error {
			if !remoteObjects.Includes(oid) {
				objectsToPush.Add(oid)
			}
			return nil
		},
	)

	for _, oid := range objectsToPush.ToArray() {
		err = data.pushRemoteObject(oid, remotePath)
	}

	if err != nil {
		return err
	}

	return data.ChangeRootDir(remotePath, func() error {
		return data.UpdateRef(refName, &RefValue{false, localRef.Value}, true)
	})
}

func (Remote) Fetch(remotePath string) error {
	remoteRefs, err := remote.getRemoteRefs(remotePath, "heads")
	if err != nil {
		return err
	}
	remoteOIDs := []string{}
	for _, refValue := range remoteRefs {
		remoteOIDs = append(remoteOIDs, refValue)
	}

	err = base.MapObjectsInCommits(
		remoteOIDs,
		func(oid string) error {
			return data.fetchRemoteObject(oid, remotePath)
		},
	)
	if err != nil {
		return err
	}

	if err = os.Mkdir(remoteRefDir, FP); err != nil && !os.IsExist(err) {
		return err
	}

	for refName, refValue := range remoteRefs {
		err := data.UpdateRef(
			filepath.Join(remoteRefDir, filepath.Base(refName)),
			&RefValue{false, refValue},
			false,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (Remote) getRemoteRefs(remotePath, prefix string) (map[string]string, error) {
	refMap := make(map[string]string)

	err := data.ChangeRootDir(remotePath, func() error {
		refIter, err := data.iterRefs(prefix, true)
		if err != nil {
			return err
		}

		for refName, ref := range refIter {
			refMap[refName] = ref.Value
		}
		return nil
	})
	return refMap, err
}
