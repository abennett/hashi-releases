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

func CheckFile(fpath string) error {
	b, err := ioutil.ReadFile(fpath)
	if err != nil {
		return err
	}
	return CheckBytes(path.Base(fpath), b)
}

func CheckBytes(fileName string, b []byte) error {
	product, version := ProductVersionFromName(fileName)
	if product == "" || version == "" {
		return errors.New("invalid file name")
	}
	shas, err := GetSHASums(product, version)
	if err != nil {
		return err
	}
	mapSum, ok := shas[fileName]
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

func GetSHASums(product, version string) (map[string][]byte, error) {
	resp, err := http.Get(SHASumLink(product, version))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := make(map[string][]byte)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			return nil, errors.New("inaccurate shasum line " + scanner.Text())
		}
		b, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, err
		}
		out[fields[1]] = b
	}
	return out, nil
}

func SHASumLink(product, version string) string {
	fileName := strings.Join([]string{product, version, SHASumSuffix}, "_")
	return strings.Join([]string{ReleasesURL, product, version, fileName}, "/")
}

func ProductVersionFromName(fileName string) (string, string) {
	split := strings.Split(fileName, "_")
	if len(split) != 4 {
		return "", ""
	}
	return split[0], split[1]
}
