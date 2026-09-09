// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/kdomanski/iso9660"
)

// noCloudVolumeLabel is the ISO9660 volume label required by the cloud-init
// NoCloud data source to recognize a seed image.
const noCloudVolumeLabel = "CIDATA"

// buildNoCloudISO renders user-data, meta-data, and network-config as the root
// files of an ISO9660 image labeled CIDATA. The writer stages the files in a
// private temporary directory that is removed on every path, with cleanup
// errors joined into the returned error.
func buildNoCloudISO(userData, metaData, networkConfig []byte) (data []byte, err error) {
	writer, err := iso9660.NewWriter()
	if err != nil {
		return nil, fmt.Errorf("unable to create ISO9660 writer: %w", err)
	}
	defer func() {
		if cleanupErr := writer.Cleanup(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("unable to clean up ISO9660 staging directory: %w", cleanupErr))
		}
	}()

	files := map[string][]byte{
		"user-data":      userData,
		"meta-data":      metaData,
		"network-config": networkConfig,
	}
	for name, content := range files {
		if err := writer.AddFile(bytes.NewReader(content), name); err != nil {
			return nil, fmt.Errorf("unable to add %s to NoCloud ISO: %w", name, err)
		}
	}

	var image bytes.Buffer
	if err := writer.WriteTo(&image, noCloudVolumeLabel); err != nil {
		return nil, fmt.Errorf("unable to write NoCloud ISO: %w", err)
	}
	return image.Bytes(), nil
}
