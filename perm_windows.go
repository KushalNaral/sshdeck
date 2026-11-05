package main

import "io/fs"

// ownedByAnotherUser: files under %AppData% are protected by the folder's access rules, which this check does not read.
func ownedByAnotherUser(fs.FileInfo) (string, bool) { return "", false }
