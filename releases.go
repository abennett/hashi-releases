package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/gocolly/colly"
	"github.com/hashicorp/go-version"
)

const (
	ReleasesURL    = "https://releases.hashicorp.com"
	ReleasesDomain = "releases.hashicorp.com"
)

type Releases struct {
	scraper  *colly.Collector
	Releases []*Release
	db       DB
}

type DB map[string]*Versions

type Release struct {
	Product string
	Version *version.Version
	OS      string
	Arch    string
	Link    string
}

type Versions []*Version

func (v Versions) Len() int {
	return len(v)
}

func (v Versions) Less(i, j int) bool {
	return v[i].Version.LessThan(v[j].Version)
}

func (v Versions) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

type Version struct {
	Version *version.Version
	OS      map[string]map[string]*Release
}

func NewReleases() *Releases {
	c := colly.NewCollector(
		colly.AllowedDomains(ReleasesDomain),
		colly.Async(true),
	)
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 10,
	})
	return &Releases{
		scraper: c,
		db:      make(DB),
	}
}

func collectReleases(releaseChan <-chan *Release, out chan<- []*Release) {
	var count int
	var releases []*Release
	for release := range releaseChan {
		count++
		fmt.Printf("\r%d files located", count)
		releases = append(releases, release)
	}
	fmt.Println()
	out <- releases
}

func (r *Releases) FetchAllReleases() ([]*Release, error) {
	releaseChan := make(chan *Release, 25)
	outChan := make(chan []*Release)
	go collectReleases(releaseChan, outChan)
	r.scraper.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		if strings.HasSuffix(link, ".zip") || strings.HasSuffix(link, ".tgz") {
			releaseChan <- ParseBinaryLink(link)
		}
		if strings.HasSuffix(link, "/") {
			e.Request.Visit(link)
		}
	})
	r.scraper.OnError(func(r *colly.Response, err error) {
		log.Printf("[ERROR] on %s: %v", r.Request.URL, err)
	})
	r.scraper.Visit(ReleasesURL)
	r.scraper.Wait()
	close(releaseChan)
	r.Releases = <-outChan
	return r.Releases, nil
}

func (r *Releases) BuildReleaseDB() {
	for x := 0; x < len(r.Releases); x++ {
		r.db.Insert(r.Releases[x])
	}
}

func (rdb DB) Insert(release *Release) {
	product := strings.ToLower(release.Product)
	versions, ok := rdb[product]
	if !ok {
		version := &Version{
			Version: release.Version,
		}
		version.insert(release)
		rdb[product] = &Versions{version}
		return
	}
	i := sort.Search(len(*versions), func(i int) bool {
		return (*versions)[i].Version.GreaterThanOrEqual(release.Version)
	})
	if i == len(*versions) {
		version := &Version{
			Version: release.Version,
		}
		*versions = append(*versions, version)
	}
	if !(*versions)[i].Version.Equal(release.Version) {
		version := &Version{
			Version: release.Version,
		}
		*versions = append(*versions, &Version{})
		copy((*versions)[i+1:], (*versions)[i:])
		(*versions)[i] = version
	}
	(*versions)[i].insert(release)
}

func (v *Version) insert(release *Release) {
	if _, ok := v.OS[release.OS]; !ok {
		v.OS = make(map[string]map[string]*Release)
		archMap := make(map[string]*Release)
		v.OS[release.OS] = archMap
	}
	v.OS[release.OS][release.Arch] = release
}

func ParseBinaryLink(link string) *Release {
	spltLink := strings.Split(link, "/")
	binary := spltLink[len(spltLink)-1]
	spltBinary := strings.Split(binary, "_")
	version, _ := version.NewVersion(spltBinary[1])
	return &Release{
		Product: spltBinary[0],
		Version: version,
		OS:      spltBinary[2],
		Arch:    strings.TrimSuffix(spltBinary[3], ".zip"),
		Link:    ReleasesURL + link,
	}
}

func (r *Releases) ListProducts() []string {
	products := make([]string, len(r.db))
	var count int
	for k, _ := range r.db {
		products[count] = k
		count++
	}
	sort.Strings(products)
	return products
}

func (r *Releases) LatestVersion(product string) (string, bool) {
	product = strings.ToLower(product)
	versions, ok := r.db[product]
	if !ok {
		return "", false
	}
	return (*versions)[versions.Len()-1].Version.Original(), true
}

func (r *Releases) ListAllVersions(product string) ([]string, error) {
	product = strings.ToLower(product)
	versions, ok := r.db[product]
	if !ok {
		return nil, fmt.Errorf("%s not found", product)
	}
	versionList := make([]string, versions.Len())
	for x := 0; x < versions.Len(); x++ {
		versionList[x] = (*versions)[x].Version.Original()
	}
	return versionList, nil
}
