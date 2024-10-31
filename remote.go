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

	remoteRefs := remote.getRemoteRefs(remotePath, "")
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

	data.ChangeRootDir(remotePath, func() {
		data.UpdateRef(refName, &RefValue{false, localRef.Value}, true)
	})

	return nil
}

func (Remote) Fetch(remotePath string) error {
	remoteRefs := remote.getRemoteRefs(remotePath, "heads")
	remoteOIDs := []string{}
	for _, refValue := range remoteRefs {
		remoteOIDs = append(remoteOIDs, refValue)
	}

	err := base.MapObjectsInCommits(
		remoteOIDs,
		func(oid string) error {
			data.fetchRemoteObject(oid, remotePath)
			return nil
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

func (Remote) getRemoteRefs(remotePath, prefix string) map[string]string {
	refMap := make(map[string]string)
	data.ChangeRootDir(remotePath, func() {
		for refName, ref := range data.iterRefs(prefix, true) {
			refMap[refName] = ref.Value
		}
	})
	return refMap
}
