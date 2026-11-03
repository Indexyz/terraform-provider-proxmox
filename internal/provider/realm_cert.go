// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Proxmox only accepts paths for realm client certificates and its upload
// endpoint only allows iso/vztmpl/import content, so PEM content is stored as
// `.img` files in an iso-capable directory storage and the realm is pointed at
// the derived path.
const realmClientCertKey = "realm-client-cert"

type realmClientCert struct {
	Node      string
	Storage   string
	CertPath  string
	KeyPath   string
	CertVolID string
	KeyVolID  string
}

func realmCertFileName(realm, suffix string) string {
	return fmt.Sprintf("terraform-realm-%s-%s.img", realm, suffix)
}

// uploadRealmClientCert resolves the upload target, stores both PEM files and
// returns their paths and volume ids.
func uploadRealmClientCert(ctx context.Context, client *Client, realm, storage string, cert, key []byte) (realmClientCert, error) {
	cfg, err := client.GetStorage(ctx, storage)
	if err != nil {
		return realmClientCert{}, fmt.Errorf("unable to read storage %q: %w", storage, err)
	}
	if cfg.Disable.Ptr() != nil && *cfg.Disable.Ptr() {
		return realmClientCert{}, fmt.Errorf("storage %q is disabled", storage)
	}
	if cfg.Path == "" {
		return realmClientCert{}, fmt.Errorf("storage %q has no filesystem path and cannot hold certificate files", storage)
	}
	if !slices.Contains(splitProxmoxList(cfg.Content), "iso") {
		return realmClientCert{}, fmt.Errorf("storage %q does not support iso content (supports %q)", storage, cfg.Content)
	}
	node, err := realmCertNode(ctx, client, cfg.Nodes)
	if err != nil {
		return realmClientCert{}, err
	}
	if err := ensureISOStorage(ctx, client, node, storage); err != nil {
		return realmClientCert{}, err
	}

	attachment := realmClientCert{Node: node, Storage: storage}
	attachment.CertPath = cfg.Path + "/template/iso/" + realmCertFileName(realm, "cert")
	attachment.KeyPath = cfg.Path + "/template/iso/" + realmCertFileName(realm, "key")
	attachment.CertVolID = storage + ":iso/" + realmCertFileName(realm, "cert")
	attachment.KeyVolID = storage + ":iso/" + realmCertFileName(realm, "key")

	for _, file := range []struct {
		data []byte
		name string
		vol  string
	}{
		{cert, realmCertFileName(realm, "cert"), attachment.CertVolID},
		{key, realmCertFileName(realm, "key"), attachment.KeyVolID},
	} {
		upid, owner, err := client.UploadStorageFile(ctx, UploadStorageFileRequest{
			Node: node, Storage: storage, Content: "iso", Filename: file.name, Data: file.data,
		})
		if err != nil {
			return realmClientCert{}, fmt.Errorf("unable to upload %q to storage %q: %w", file.name, storage, err)
		}
		if err := client.waitForNodeTask(ctx, owner, upid); err != nil {
			return realmClientCert{}, fmt.Errorf("upload task %q for %q did not finish: %w", upid, file.vol, err)
		}
	}
	return attachment, nil
}

// realmCertNode picks the upload node: the first storage-restricted node, else
// the alphabetically first cluster node.
func realmCertNode(ctx context.Context, client *Client, storageNodes string) (string, error) {
	if nodes := splitProxmoxList(storageNodes); len(nodes) > 0 {
		return nodes[0], nil
	}
	nodes, err := client.Nodes(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to list cluster nodes: %w", err)
	}
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}
	if len(names) == 0 {
		return "", errors.New("cluster has no nodes to upload to")
	}
	sort.Strings(names)
	return names[0], nil
}

// removeRealmClientCert deletes both stored files, ignoring already missing ones.
func removeRealmClientCert(ctx context.Context, client *Client, attachment realmClientCert) error {
	var firstErr error
	for _, volID := range []string{attachment.CertVolID, attachment.KeyVolID} {
		if volID == "" {
			continue
		}
		upid, err := client.deleteStorageFile(ctx, attachment.Node, attachment.Storage, volID)
		if err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("unable to delete %q: %w", volID, err)
			}
			continue
		}
		if upid == "" {
			continue
		}
		owner, err := taskOwnerNode(upid)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("delete of %q returned an invalid UPID: %w", volID, err)
			}
			continue
		}
		if err := client.waitForNodeTask(ctx, owner, upid); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("delete task %q for %q did not finish: %w", upid, volID, err)
		}
	}
	return firstErr
}

func readRealmClientCert(ctx context.Context, private nodeNetworkPrivateData) (realmClientCert, diag.Diagnostics) {
	var attachment realmClientCert
	value, diags := private.GetKey(ctx, realmClientCertKey)
	if diags.HasError() || len(value) == 0 {
		return attachment, diags
	}
	if err := json.Unmarshal(value, &attachment); err != nil {
		diags.AddError("Unable to Read Realm Client Certificate State", fmt.Sprintf("unable to decode client certificate state: %v", err))
	}
	return attachment, diags
}

func writeRealmClientCert(ctx context.Context, private nodeNetworkPrivateData, attachment realmClientCert) diag.Diagnostics {
	encoded, err := json.Marshal(attachment)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to Store Realm Client Certificate State", fmt.Sprintf("unable to encode client certificate state: %v", err))
		return diags
	}
	return private.SetKey(ctx, realmClientCertKey, encoded)
}

// realmCertFilesExist reports whether both stored files are still present.
func realmCertFilesExist(ctx context.Context, client *Client, attachment realmClientCert) (bool, error) {
	for _, volumeID := range []string{attachment.CertVolID, attachment.KeyVolID} {
		if volumeID == "" {
			return false, nil
		}
		exists, err := isoVolumeExists(ctx, client, attachment.Node, attachment.Storage, volumeID)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}
