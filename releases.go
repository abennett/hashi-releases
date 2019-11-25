package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-version"
	"github.com/mitchellh/cli"
)

const (
	ReleasesURL    = "https://releases.hashicorp.com"
	ReleasesDomain = "releases.hashicorp.com"

	indexSuffix = ".index"
)

var (
	localOS   = runtime.GOOS
	localArch = runtime.GOARCH

	tmpDir = path.Join(os.TempDir(), "hashi-releases")

	logger hclog.Logger
)

type Releases struct {
	Index Index
}

type Index struct {
	Products map[string]*Product
}

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
	Product    string   `json:"name"`
	Version    string   `json:"version"`
	SHASums    string   `json:"shasums"`
	SHASumsSig string   `json:"shasums_signature"`
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

func (i Index) DownloadBuildForLocal(product, version string) error {
	p, ok := i.Products[product]
	if !ok {
		return fmt.Errorf("product %s not found", product)
	}
	v, ok := p.Versions[version]
	if !ok {
		return fmt.Errorf("version %s of %s not found", version, product)
	}
	build := v.GetBuildForLocal()
	if build == nil {
		return errors.New("no such build")
	}
	bts, err := build.Download()
	if err != nil {
		return err
	}
	if err = CheckBytes(build.Filename, bts); err != nil {
		return err
	}
	_, err = ExtractZip(product, "", bts)
	return err
}

func ExtractZip(product, parentDir string, bts []byte) (string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(bts), int64(len(bts)))
	if err != nil {
		return "", err
	}
	finalPath := path.Join(parentDir, product)
	outFile, err := os.OpenFile(finalPath, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		return "", err
	}
	var content io.ReadCloser
	for _, f := range zipReader.File {
		if f.Name == product {
			zipFile, err := f.Open()
			if err != nil {
				return "", err
			}
			content = zipFile
		}
	}
	_, err = io.Copy(outFile, content)
	if err != nil {
		return "", nil
	}
	return finalPath, nil
}

func (b *Build) Download() ([]byte, error) {
	resp, err := http.Get(b.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bts, nil
}

func (b *Build) DownloadAndSave() error {
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
	cacheFilePath := path.Join(tmpDir, etag, etag+indexSuffix)

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
	if err = json.Unmarshal(b, &index.Products); err != nil {
		panic(err)
	}
	for _, v := range index.Products {
		if err = v.sortVersions(); err != nil {
			panic(err)
		}
	}
	return index
}

func (i *Index) LatestVersion(product string) string {
	p, ok := i.Products[product]
	if !ok {
		return ""
	}
	return p.Sorted[len(p.Sorted)-1].Original()
}

func (i *Index) LatestBuild(product, os, arch string) *Build {
	p, ok := i.Products[product]
	if !ok {
		return nil
	}
	v, ok := p.Versions[p.Sorted[len(p.Sorted)-1].Original()]
	if !ok {
		return nil
	}
	return v.GetBuildForLocal()
}

func (i *Index) ListVersions(product string) []string {
	p, ok := i.Products[product]
	if !ok {
		return nil
	}
	versions := make([]string, len(p.Sorted))
	for idx, v := range p.Sorted {
		versions[idx] = v.Original()
	}
	return versions
}

func (i *Index) ListProducts() []string {
	products := make([]string, len(i.Products))
	var idx int
	for k, _ := range i.Products {
		products[idx] = k
		idx++
	}
	return products
}

type IndexCommand struct {
	synopsis string
	help     string
	product  string
	index    *Index
}

func (ic *IndexCommand) Help() string {
	return ic.help
}

func (ic *IndexCommand) Synopsis() string {
	return ic.synopsis
}

func (ic *IndexCommand) Run(args []string) int {
	if len(args) != 1 {
		fmt.Println("invalid number of arguments")
		return 1
	}
	version, ok := ic.index.Products[ic.product].Versions[args[0]]
	if !ok {
		fmt.Println(args[0] + " not found for " + ic.product)
	}
	if err := ic.index.DownloadBuildForLocal(ic.product, version.Version); err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}

type ListCommand struct {
	list version.Collection
}

func (lc *ListCommand) Help() string {
	return ""
}

func (lc *ListCommand) Synopsis() string {
	return ""
}

func (lc *ListCommand) Run(args []string) int {
	for _, v := range lc.list {
		fmt.Println(v)
	}
	return 0
}

func (i *Index) Commands() map[string]cli.CommandFactory {
	commands := make(map[string]cli.CommandFactory)
	for _, product := range i.Products {
		name := product.Name
		commands[name] = func() (cli.Command, error) {
			return &IndexCommand{
				product: name,
				index:   i,
			}, nil
		}
		list := product.Sorted
		commands[name+" list"] = func() (cli.Command, error) {
			return &ListCommand{
				list: list,
			}, nil
		}
	}
	return commands
}
