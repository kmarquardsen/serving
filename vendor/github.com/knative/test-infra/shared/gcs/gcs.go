/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// gcs.go defines functions to use GCS

package gcs

import (
	"context"
	"io/ioutil"
	"log"
	"path"
	"os"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/api/iterator"
)

var client *storage.Client

// Authenticate explicitly sets up authentication for the rest of run
func Authenticate(ctx context.Context, serviceAccount string) error {
	var err error
	client, err = storage.NewClient(ctx, option.WithCredentialsFile(serviceAccount))
	return err
}

// Exist checks if path exist under gcs bucket
func Exist(ctx context.Context, bucketName, filePath string) bool {
	handle := createStorageObject(bucketName, filePath)
	_, err := handle.Attrs(ctx)
	return nil == err
}

// ListDirectChildren lists direct children paths (including files and directories).
func ListDirectChildren(ctx context.Context, bucketName, storagePath string) []string {
	// If there are 2 directories named "foo" and "foobar",
	// then given storagePath "foo" will get files both under "foo" and "foobar".
	// Add trailling slash to storagePath, so that only gets children under given directory.
	return list(ctx, bucketName, strings.TrimRight(storagePath, " /") + "/", "/")
}

// Copy file from within gcs
func Copy(ctx context.Context, srcBucketName, srcPath, dstBucketName, dstPath string) error {
	src := createStorageObject(srcBucketName, srcPath)
	dst := createStorageObject(dstBucketName, dstPath)

	_, err := dst.CopierFrom(src).Run(ctx)
	return err
}

// Download file from gcs
func Download(ctx context.Context, bucketName, srcPath, dstPath string) error {
	handle := createStorageObject(bucketName, srcPath)
	if _, err := handle.Attrs(ctx); nil != err {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	src, err := handle.NewReader(ctx)
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err = io.Copy(dst, src); nil != err {
		return err
	}
	return nil
}

// Upload file to gcs
func Upload(ctx context.Context, bucketName, dstPath, srcPath string) error {
	src, err := os.Open(srcPath)
	if nil != err {
		return err
	}
	dst := createStorageObject(bucketName, dstPath).NewWriter(ctx)
	defer dst.Close()
	if _, err = io.Copy(dst, src); nil != err {
		return err
	}
	return nil
}

// Read reads the specified file
func Read(ctx context.Context, bucketName, filePath string) ([]byte, error) {
	var contents []byte
	f, err := NewReader(ctx, bucketName, filePath)
	defer f.Close()
	if err != nil {
		return contents, err
	}
	contents, err = ioutil.ReadAll(f)
	if err != nil {
		return contents, err
	}
	return contents, nil
}

// NewReader creates a new Reader of a gcs file.
// Important: caller must call Close on the returned Reader when done reading
func NewReader(ctx context.Context, bucketName, filePath string) (*storage.Reader, error) {
	o := createStorageObject(bucketName, filePath)
	if _, err := o.Attrs(ctx); err != nil {
		return nil, err
	}
	return o.NewReader(ctx)
}

// create storage object handle, this step doesn't access internet
func createStorageObject(bucketName, filePath string) *storage.ObjectHandle {
	return client.Bucket(bucketName).Object(filePath)
}

// Query items under given gcs storagePath, use delim to eliminate some files.
// see https://godoc.org/cloud.google.com/go/storage#Query
func getObjectsAttrs(ctx context.Context, bucketName, storagePath, delim string) []*storage.ObjectAttrs {
	var allAttrs []*storage.ObjectAttrs
	bucketHandle := client.Bucket(bucketName)
	it := bucketHandle.Objects(ctx, &storage.Query{
		Prefix:	storagePath,
		Delimiter: delim,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error iterating: %v", err)
		}
		allAttrs = append(allAttrs, attrs)
	}
	return allAttrs
}

// list child under storagePath, use exclusionFilter for skipping some files.
// This function gets all child files recursively under given storagePath,
// then filter out filenames containing giving exclusionFilter.
// If exclusionFilter is empty string, returns all files but not directories,
// if exclusionFilter is "/", returns all direct children, including both files and directories.
// see https://godoc.org/cloud.google.com/go/storage#Query
func list(ctx context.Context, bucketName, storagePath, exclusionFilter string) []string {
	var filePaths []string
	objsAttrs := getObjectsAttrs(ctx, bucketName, storagePath, exclusionFilter)
	for _, attrs := range objsAttrs {
		filePaths = append(filePaths, path.Join(attrs.Prefix, attrs.Name))
	}
	return filePaths
}
