package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-version"
)

func newVersion(v string) *version.Version {
	ver, _ := version.NewVersion(v)
	return ver
}

var testReleases = []*Release{
	&Release{
		Version: newVersion("0.1.0"),
		Product: "test",
		OS:      "windows",
		Arch:    "amd64",
	},
	&Release{
		Version: newVersion("0.1.1"),
		Product: "test",
		OS:      "linux",
		Arch:    "386",
	},
	&Release{
		Version: newVersion("0.2.1"),
		Product: "test",
		OS:      "openbsd",
		Arch:    "arm",
	},
	&Release{
		Version: newVersion("1.0.0"),
		Product: "other",
		OS:      "darwin",
		Arch:    "amd64",
	},
}

func TestInsertions(t *testing.T) {
	var db = make(ReleaseDB)
	for x := 0; x < len(testReleases); x++ {
		db.Insert(testReleases[x])
	}
	for k, v := range db {
		fmt.Printf("%s: %d\n", k, v.Len())
	}
	versions := db["test"]
	for _, v := range *versions {
		fmt.Printf("%#v\n", v)
	}
	if len(*versions) != 3 {
		t.Fatal("wrong number of versions")
	}
	releases := Releases{
		db: db,
	}
	fmt.Println(releases.LatestVersion("test"))
}
