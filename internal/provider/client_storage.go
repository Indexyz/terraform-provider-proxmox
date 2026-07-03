// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Storage struct {
	Storage     string
	Type        string
	Content     string
	Nodes       string
	Disable     proxmoxOptionalBool
	Shared      proxmoxOptionalBool
	Path        string
	Pool        string
	VGName      string
	ThinPool    string
	Server      string
	Export      string
	Share       string
	Username    string
	Password    string
	Monhost     string
	Datastore   string
	Namespace   string
	Fingerprint string
	SMBVersion  string
	Options     string
	Format      string
	Mkdir       proxmoxOptionalBool
	Sparse      proxmoxOptionalBool
	NoCOW       proxmoxOptionalBool
	KRBD        proxmoxOptionalBool
	Blocksize   string
	FSName      string
	ExtraConfig map[string]string
}

type storageConfigKnown struct {
	Storage     string              `json:"storage"`
	Type        string              `json:"type"`
	Content     string              `json:"content"`
	Nodes       string              `json:"nodes"`
	Disable     proxmoxOptionalBool `json:"disable"`
	Shared      proxmoxOptionalBool `json:"shared"`
	Path        string              `json:"path"`
	Pool        string              `json:"pool"`
	VGName      string              `json:"vgname"`
	ThinPool    string              `json:"thinpool"`
	Server      string              `json:"server"`
	Export      string              `json:"export"`
	Share       string              `json:"share"`
	Username    string              `json:"username"`
	Password    string              `json:"password"`
	Monhost     string              `json:"monhost"`
	Datastore   string              `json:"datastore"`
	Namespace   string              `json:"namespace"`
	Fingerprint string              `json:"fingerprint"`
	SMBVersion  string              `json:"smbversion"`
	Options     string              `json:"options"`
	Format      string              `json:"format"`
	Mkdir       proxmoxOptionalBool `json:"mkdir"`
	Sparse      proxmoxOptionalBool `json:"sparse"`
	NoCOW       proxmoxOptionalBool `json:"nocow"`
	KRBD        proxmoxOptionalBool `json:"krbd"`
	Blocksize   string              `json:"blocksize"`
	FSName      string              `json:"fs-name"`
}

type StorageRequest struct {
	Storage     string
	Type        string
	Content     *string
	Nodes       *string
	Disable     *bool
	Shared      *bool
	Path        *string
	Pool        *string
	VGName      *string
	ThinPool    *string
	Server      *string
	Export      *string
	Share       *string
	Username    *string
	Password    *string
	Monhost     *string
	Datastore   *string
	Namespace   *string
	Fingerprint *string
	SMBVersion  *string
	Options     *string
	Format      *string
	Mkdir       *bool
	Sparse      *bool
	NoCOW       *bool
	KRBD        *bool
	Blocksize   *string
	FSName      *string
	Delete      []string
	ExtraConfig map[string]string
}

func (r StorageRequest) IsEmpty() bool {
	return r.Content == nil && r.Nodes == nil && r.Disable == nil && r.Shared == nil &&
		r.Path == nil && r.Pool == nil && r.VGName == nil && r.ThinPool == nil &&
		r.Server == nil && r.Export == nil && r.Share == nil && r.Username == nil &&
		r.Password == nil && r.Monhost == nil && r.Datastore == nil && r.Namespace == nil &&
		r.Fingerprint == nil && r.SMBVersion == nil && r.Options == nil && r.Format == nil &&
		r.Mkdir == nil && r.Sparse == nil && r.NoCOW == nil && r.KRBD == nil &&
		r.Blocksize == nil && r.FSName == nil && len(r.ExtraConfig) == 0 && len(r.Delete) == 0
}

var storageKnownKeys = map[string]struct{}{
	"storage": {}, "type": {}, "content": {}, "nodes": {}, "disable": {}, "shared": {},
	"path": {}, "pool": {}, "vgname": {}, "thinpool": {}, "server": {}, "export": {},
	"share": {}, "username": {}, "password": {}, "monhost": {}, "datastore": {}, "namespace": {},
	"fingerprint": {}, "smbversion": {}, "options": {}, "format": {}, "mkdir": {}, "sparse": {},
	"nocow": {}, "krbd": {}, "blocksize": {}, "fs-name": {},
}

func (c *Client) GetStorage(ctx context.Context, id string) (Storage, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/storage/%s", url.PathEscape(id)), nil, nil, &raw); err != nil {
		return Storage{}, err
	}
	return decodeStorageConfig(raw)
}

func (c *Client) Storages(ctx context.Context) ([]Storage, error) {
	var rawList []map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/storage", nil, nil, &rawList); err != nil {
		return nil, err
	}
	result := make([]Storage, 0, len(rawList))
	for _, raw := range rawList {
		config, err := decodeStorageConfig(raw)
		if err != nil {
			return nil, err
		}
		result = append(result, config)
	}
	return result, nil
}

func (c *Client) CreateStorage(ctx context.Context, req StorageRequest) error {
	form := url.Values{}
	form.Set("storage", req.Storage)
	form.Set("type", req.Type)
	encodeStorageFields(form, req)
	return c.do(ctx, http.MethodPost, "/storage", nil, form, nil)
}

func (c *Client) UpdateStorage(ctx context.Context, id string, req StorageRequest) error {
	form := url.Values{}
	encodeStorageFields(form, req)
	if len(req.Delete) > 0 {
		form.Set("delete", strings.Join(sortedStrings(req.Delete), ","))
	}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/storage/%s", url.PathEscape(id)), nil, form, nil)
}

func (c *Client) DeleteStorage(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/storage/%s", url.PathEscape(id)), nil, nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

func decodeStorageConfig(raw map[string]json.RawMessage) (Storage, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return Storage{}, fmt.Errorf("unable to marshal raw storage config: %w", err)
	}
	var known storageConfigKnown
	if err := json.Unmarshal(payload, &known); err != nil {
		return Storage{}, fmt.Errorf("unable to decode storage config: %w", err)
	}
	config := Storage{
		Storage:     known.Storage,
		Type:        known.Type,
		Content:     known.Content,
		Nodes:       known.Nodes,
		Disable:     known.Disable,
		Shared:      known.Shared,
		Path:        known.Path,
		Pool:        known.Pool,
		VGName:      known.VGName,
		ThinPool:    known.ThinPool,
		Server:      known.Server,
		Export:      known.Export,
		Share:       known.Share,
		Username:    known.Username,
		Password:    known.Password,
		Monhost:     known.Monhost,
		Datastore:   known.Datastore,
		Namespace:   known.Namespace,
		Fingerprint: known.Fingerprint,
		SMBVersion:  known.SMBVersion,
		Options:     known.Options,
		Format:      known.Format,
		Mkdir:       known.Mkdir,
		Sparse:      known.Sparse,
		NoCOW:       known.NoCOW,
		KRBD:        known.KRBD,
		Blocksize:   known.Blocksize,
		FSName:      known.FSName,
		ExtraConfig: map[string]string{},
	}
	for key, value := range raw {
		if _, ok := storageKnownKeys[key]; ok {
			continue
		}
		decoded, ok := decodeQemuConfigStringValue(value)
		if !ok || decoded == "" {
			continue
		}
		config.ExtraConfig[key] = decoded
	}
	if len(config.ExtraConfig) == 0 {
		config.ExtraConfig = nil
	}
	return config, nil
}

func encodeStorageFields(form url.Values, req StorageRequest) {
	setOptionalString(form, "content", req.Content)
	setOptionalString(form, "nodes", req.Nodes)
	setOptionalString(form, "path", req.Path)
	setOptionalString(form, "pool", req.Pool)
	setOptionalString(form, "vgname", req.VGName)
	setOptionalString(form, "thinpool", req.ThinPool)
	setOptionalString(form, "server", req.Server)
	setOptionalString(form, "export", req.Export)
	setOptionalString(form, "share", req.Share)
	setOptionalString(form, "username", req.Username)
	setOptionalString(form, "password", req.Password)
	setOptionalString(form, "monhost", req.Monhost)
	setOptionalString(form, "datastore", req.Datastore)
	setOptionalString(form, "namespace", req.Namespace)
	setOptionalString(form, "fingerprint", req.Fingerprint)
	setOptionalString(form, "smbversion", req.SMBVersion)
	setOptionalString(form, "options", req.Options)
	setOptionalString(form, "format", req.Format)
	setOptionalString(form, "blocksize", req.Blocksize)
	setOptionalString(form, "fs-name", req.FSName)
	setOptionalBool(form, "disable", req.Disable)
	setOptionalBool(form, "shared", req.Shared)
	setOptionalBool(form, "mkdir", req.Mkdir)
	setOptionalBool(form, "sparse", req.Sparse)
	setOptionalBool(form, "nocow", req.NoCOW)
	setOptionalBool(form, "krbd", req.KRBD)
	setSortedStringMap(form, req.ExtraConfig)
}
