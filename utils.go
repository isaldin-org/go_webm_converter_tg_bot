package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
)

func isWebmUrl(url string) bool {
	match, err := regexp.Match("^http(s)?:\\/\\/[a-zA-Z0-9]{2,256}\\.[a-z]{2,6}\\/.*\\.webm$", []byte(url))

	if err != nil {
		return false
	}

	return match
}

func getFileHash() (string, error) {
	file, err := os.Open("temp.webm")
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	hashInBytes := hash.Sum(nil)[:16]
	return hex.EncodeToString(hashInBytes), nil
}

func isUrlSuitableForConvertation(url string) (bool, error) {
	resp, err := http.Head(url)
	if err != nil {
		return false, errors.New("failed to receive file headers from url")
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status code is %d", resp.StatusCode)
	}

	fileType := resp.Header.Get("Content-Type")
	if fileType != "video/webm" {
		return false, errors.New("content type is not suitable to webm")
	}

	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	if size > 50*1024*1024 {
		return false, errors.New("size should less than 50mb")
	}

	return true, nil
}
