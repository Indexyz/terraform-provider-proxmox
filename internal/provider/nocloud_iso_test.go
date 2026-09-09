// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/kdomanski/iso9660"
)

const noCloudISOSectorSize = 2048

// readISORootRecords walks the produced image bytes independently of the
// writer: it parses the root directory records from the primary volume
// descriptor and returns the raw extent content of every root file keyed by
// its on-media identifier.
func readISORootRecords(t *testing.T, image []byte) map[string][]byte {
	t.Helper()

	pvd := image[16*noCloudISOSectorSize:]
	rootRecord := pvd[156 : 156+34]
	rootExtent := int64(binary.LittleEndian.Uint32(rootRecord[2:6]))
	rootSize := int64(binary.LittleEndian.Uint32(rootRecord[10:14]))

	files := map[string][]byte{}
	offset := rootExtent * noCloudISOSectorSize
	end := offset + rootSize
	for offset < end {
		recordLength := int64(image[offset])
		if recordLength == 0 {
			// Directory records do not cross sector boundaries.
			offset += noCloudISOSectorSize - (offset % noCloudISOSectorSize)
			continue
		}
		record := image[offset : offset+recordLength]
		identifier := string(record[33 : 33+int(record[32])])
		if identifier[0] != 0 && identifier[0] != 1 {
			extent := int64(binary.LittleEndian.Uint32(record[2:6]))
			size := int64(binary.LittleEndian.Uint32(record[10:14]))
			files[identifier] = image[extent*noCloudISOSectorSize : extent*noCloudISOSectorSize+size]
		}
		offset += recordLength
	}
	return files
}

func TestBuildNoCloudISOActualBytes(t *testing.T) {
	userData := "#cloud-config\nruncmd:\n  - echo seeded\n"
	metaData := "instance-id: iid-20260214\nlocal-hostname: shaula-runner\n"
	networkConfig := "version: 2\nethernets:\n  nic0:\n    match:\n      name: en*\n    dhcp4: true\n"

	image, err := buildNoCloudISO([]byte(userData), []byte(metaData), []byte(networkConfig))
	if err != nil {
		t.Fatalf("buildNoCloudISO() unexpected error: %v", err)
	}
	if len(image) == 0 || len(image)%noCloudISOSectorSize != 0 {
		t.Fatalf("image size %d is not sector aligned", len(image))
	}

	// Sector 16 must be a primary volume descriptor carrying the CIDATA label
	// that the NoCloud data source matches, followed by a terminator.
	pvd := image[16*noCloudISOSectorSize:]
	if pvd[0] != 1 || string(pvd[1:6]) != "CD001" || pvd[6] != 1 {
		t.Fatalf("sector 16 is not a primary volume descriptor: %#v", pvd[:8])
	}
	if label := strings.TrimRight(string(pvd[40:72]), " "); label != noCloudVolumeLabel {
		t.Fatalf("unexpected volume label %q", label)
	}
	terminator := image[17*noCloudISOSectorSize:]
	if terminator[0] != 255 || string(terminator[1:6]) != "CD001" {
		t.Fatalf("sector 17 is not a volume descriptor terminator: %#v", terminator[:8])
	}

	// The root records must expose the three NoCloud files under the on-media
	// identifiers (mangled ISO9660 form of user-data etc.) with byte-exact
	// contents at their extents.
	files := readISORootRecords(t, image)
	want := map[string]string{
		"user-data;1":      userData,
		"meta-data;1":      metaData,
		"network-config;1": networkConfig,
	}
	if len(files) != len(want) {
		t.Fatalf("unexpected root files %v", files)
	}
	for identifier, content := range want {
		got, ok := files[identifier]
		if !ok {
			t.Fatalf("missing root file %q in %v", identifier, files)
		}
		if string(got) != content {
			t.Fatalf("content mismatch for %q: got %q want %q", identifier, got, content)
		}
	}

	// The ISO reader must resolve the same files by their unmangled NoCloud
	// names, mirroring what the Linux isofs mount exposes after dropping the
	// ;1 version suffix from identifiers.
	readBack, err := iso9660.OpenImage(bytes.NewReader(image))
	if err != nil {
		t.Fatalf("OpenImage() unexpected error: %v", err)
	}
	label, err := readBack.Label()
	if err != nil || label != noCloudVolumeLabel {
		t.Fatalf("reader label %q err %v", label, err)
	}
	root, err := readBack.RootDir()
	if err != nil {
		t.Fatalf("RootDir() unexpected error: %v", err)
	}
	children, err := root.GetChildren()
	if err != nil {
		t.Fatalf("GetChildren() unexpected error: %v", err)
	}
	byName := map[string]string{}
	for _, child := range children {
		content, err := io.ReadAll(child.Reader())
		if err != nil {
			t.Fatalf("read %q: %v", child.Name(), err)
		}
		byName[child.Name()] = string(content)
	}
	for name, content := range map[string]string{"user-data": userData, "meta-data": metaData, "network-config": networkConfig} {
		if byName[name] != content {
			t.Fatalf("reader content mismatch for %q: got %q want %q", name, byName[name], content)
		}
	}
}
