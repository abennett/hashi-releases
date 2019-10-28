package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/mitchellh/cli"
)

const (
	ReleasesURL    = "https://releases.hashicorp.com"
	ReleasesDomain = "releases.hashicorp.com"
)

var (
	localOS   = runtime.GOOS
	localArch = runtime.GOARCH

	tmpDir = path.Join(os.TempDir(), "hashi-releases")
)

type Releases struct {
	Index Index
}

type Index map[string]*Product

type Product struct {
	Name     string              `json:"name"`
	Versions map[string]*Version `json:"versions"`
	Sorted   version.Collection
	isSorted bool
}

func (p *Product) sortVersions() error {
	collection := make(version.Collection, len(p.Versions))
	var idx int
	for k, _ := range p.Versions {
		v, err := version.NewVersion(k)
		if err != nil {
			return err
		}
		collection[idx] = v
		idx++
	}
	p.Sorted = collection
	sort.Sort(p.Sorted)
	p.isSorted = true
	return nil
}

type Version struct {
	Product    string `json:"name"`
	Version    string `json:"version"`
	SHASums    string `json:"shasums"`
	SHASumsSig string `json:"shasums_signature"`
	shaMap     map[string][]byte
	Builds     []*Build `json:"builds"`
}

type Build struct {
	Product  string `json:"name"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

func (v *Version) GetBuildForLocal() *Build {
	for idx, b := range v.Builds {
		if b.OS == localOS && b.Arch == localArch {
			return v.Builds[idx]
		}
	}
	return nil
}

func (v *Version) initSHAMap() error {
	resp, err := http.Get(strings.Join([]string{ReleasesURL, v.Product, v.Version, v.SHASums}, "/"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *Build) Download() error {
	resp, err := http.Get(b.URL)
	if err != nil {
		return err
	}
	f, err := os.Create(b.Filename)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func NewIndex() Index {
	resp, err := http.Get(ReleasesURL + "/index.json")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	etag := resp.Header.Get("Etag")
	etag = strings.Trim(etag, "\"")
	if etag == "" {
		panic("no etag found")
	}
	cacheFilePath := path.Join(tmpDir, etag)

	b, err := ioutil.ReadFile(cacheFilePath)
	if err != nil {
		b, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		if err = os.MkdirAll(path.Dir(cacheFilePath), 0700); err != nil {
			panic(err)
		}
		if err = ioutil.WriteFile(cacheFilePath, b, 0600); err != nil {
			panic(err)
		}
	}
	var index Index
	if err = json.Unmarshal(b, &index); err != nil {
		panic(err)
	}
	for _, v := range index {
		if err = v.sortVersions(); err != nil {
			panic(err)
		}
	}
	return index
}

func (i Index) LatestVersion(product string) string {
	p, ok := i[product]
	if !ok {
		return ""
	}
	return p.Sorted[len(p.Sorted)-1].Original()
}

func (i Index) LatestBuild(product, os, arch string) *Build {
	p, ok := i[product]
	if !ok {
		return nil
	}
	v, ok := p.Versions[p.Sorted[len(p.Sorted)-1].Original()]
	if !ok {
		return nil
	}
	return v.GetBuildForLocal()
}

func (i Index) ListVersions(product string) []string {
	p, ok := i[product]
	if !ok {
		return nil
	}
	versions := make([]string, len(p.Sorted))
	for idx, v := range p.Sorted {
		versions[idx] = v.Original()
	}
	return versions
}

func (i Index) ListProducts() []string {
	products := make([]string, len(i))
	var idx int
	for k, _ := range i {
		products[idx] = k
		idx++
	}
	return products
}

type IndexCommand struct {
	synopsis string
	help     string
	version  *Version
}

func (ic *IndexCommand) Help() string {
	return ic.help
}

func (ic *IndexCommand) Synopsis() string {
	return ic.synopsis
}

func (ic *IndexCommand) Run(args []string) int {
	build := ic.version.GetBuildForLocal()
	if build == nil {
		fmt.Printf("%s %s not found for this os and/or architecture\n", build.Product, build.Version)
		return 1
	}
	if err := build.Download(); err != nil {
		return 1
	}
	return 0
}

func (i Index) Commands() map[string]cli.CommandFactory {
	commands := make(map[string]cli.CommandFactory)
	for _, product := range i {
		for idx, _ := range product.Sorted {
			versionNum := product.Sorted[idx].Original()
			versionInfo, ok := product.Versions[versionNum]
			if !ok {
				panic(product.Name + " " + versionNum)
			}
			if versionInfo.GetBuildForLocal() == nil {
				continue
			}
			commands[product.Name+" "+versionNum] = func() (cli.Command, error) {
				return &IndexCommand{
					version: versionInfo,
				}, nil
			}
		}
	}
	return commands
}
