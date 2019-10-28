package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
)

const (
	SHASumSuffix = "SHA256SUMS"
)

type SHAMap map[string][]byte

func (sm SHAMap) CheckFile(fpath string) error {
	b, err := ioutil.ReadFile(fpath)
	if err != nil {
		return err
	}
	mapSum, ok := sm[path.Base(fpath)]
	if !ok {
		return errors.New("sum for file not found")
	}
	hash := sha256.New()
	hash.Write(b)
	if !bytes.Equal(mapSum, hash.Sum(nil)) {
		return errors.New("hashes do not match")
	}
	return nil
}

func (sm SHAMap) GetSHASums(product, version string) error {
	resp, err := http.Get(SHASumLink(product, version))
	if err != nil {
		return err
	}
	shaMap := make(SHAMap)
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		splitLine := strings.Split(scanner.Text(), " ")
		if len(splitLine) != 2 {
			return errors.New("inaccurate shasum file")
		}
		b, err := hex.DecodeString(splitLine[0])
		if err != nil {
			return err
		}
		shaMap[splitLine[0]] = b
	}
	return nil
}

func SHASumLink(product, version string) string {
	fileName := strings.Join([]string{product, version, SHASumSuffix}, "_")
	return strings.Join([]string{ReleasesURL, product, version, fileName}, "/")
}
