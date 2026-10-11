package console

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

const (
	aboutProduct  = "Trajectory console"
	licenseName   = "Apache License 2.0"
	licenseSPDX   = "Apache-2.0"
	licenseURL    = "https://www.apache.org/licenses/LICENSE-2.0"
	licenseSource = "https://github.com/Coder-s-OG-s/Trajectory-IR"
	licenseNotice = "Apache License 2.0. No product key. No expiry. Not a paid support contract."
)

// LicenseInfo is the Apache 2.0 notice for this console. There is no product key.
type LicenseInfo struct {
	Name    string  `json:"name"`
	SPDX    string  `json:"spdx"`
	URL     string  `json:"url"`
	Source  string  `json:"source_url"`
	Notice  string  `json:"notice"`
	Expires *string `json:"expires"`
	Paid    bool    `json:"paid"`
}

// AboutView is the operator About page: license plus facts about this machine.
type AboutView struct {
	Product       string      `json:"product"`
	License       LicenseInfo `json:"license"`
	DataDir       string      `json:"data_dir"`
	DataBytes     int64       `json:"data_bytes"`
	RunCount      int         `json:"run_count"`
	EventCount    int         `json:"event_count"`
	PackageCount  int         `json:"package_count"`
	AuthRequired  bool        `json:"auth_required"`
	LoopbackTools bool        `json:"loopback_tools"`
	Health        string      `json:"health"`
}

func staticLicense() LicenseInfo {
	return LicenseInfo{
		Name:    licenseName,
		SPDX:    licenseSPDX,
		URL:     licenseURL,
		Source:  licenseSource,
		Notice:  licenseNotice,
		Expires: nil,
		Paid:    false,
	}
}

// About scans the local data dir once for the About overlay.
func (s *Store) About() (AboutView, error) {
	sums, err := s.summarizeAll()
	if err != nil {
		return AboutView{}, err
	}
	events := 0
	for _, sum := range sums {
		events += sum.EventCount
	}
	pkgs, err := s.countPackages()
	if err != nil {
		return AboutView{}, err
	}
	bytes, err := dirBytes(s.root)
	if err != nil {
		return AboutView{}, err
	}
	return AboutView{
		Product:       aboutProduct,
		License:       staticLicense(),
		DataDir:       s.root,
		DataBytes:     bytes,
		RunCount:      len(sums),
		EventCount:    events,
		PackageCount:  pkgs,
		LoopbackTools: true,
		Health:        "ok",
	}, nil
}

func (s *Store) countPackages() (int, error) {
	root := filepath.Join(s.root, packagesDir)
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, bad := emit.SafeTirName(d.Name()); bad != nil {
			return nil
		}
		n++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return n, nil
}

func dirBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return total, nil
}
