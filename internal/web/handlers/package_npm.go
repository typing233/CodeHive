package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
)

var _ = sql.ErrNoRows

type NPMHandler struct {
	packageStore *store.PackageStore
	userStore    *store.UserStore
	tokenStore   *store.TokenStore
	auditStore   *store.AuditStore
	cfg          *config.Config
}

func NewNPMHandler(ps *store.PackageStore, us *store.UserStore, ts *store.TokenStore, as *store.AuditStore, cfg *config.Config) *NPMHandler {
	return &NPMHandler{
		packageStore: ps,
		userStore:    us,
		tokenStore:   ts,
		auditStore:   as,
		cfg:          cfg,
	}
}

func (h *NPMHandler) authenticateRequest(r *http.Request) (*models.User, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, fmt.Errorf("no auth")
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	user, err := h.tokenStore.GetUserByTokenHash(r.Context(), tokenHash)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	return user, nil
}

func (h *NPMHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	scope := chi.URLParam(r, "scope")
	pkgName := name
	if scope != "" {
		pkgName = scope + "/" + name
	}

	user, _ := h.authenticateRequest(r)

	var pkg *models.Package
	var err error

	if user != nil {
		pkg, err = h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "npm", pkgName)
	}
	if pkg == nil || err != nil {
		// Try to find a public package by name across all users
		pkg, err = h.findPublicPackage(r, "npm", pkgName)
		if err != nil {
			// Proxy to upstream if configured
			if h.cfg.Packages.NPMUpstream != "" {
				h.proxyNPMMetadata(w, r, pkgName)
				return
			}
			http.NotFound(w, r)
			return
		}
	}

	if pkg.IsPrivate && (user == nil || user.ID != pkg.OwnerID) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	versions, _ := h.packageStore.ListVersions(r.Context(), pkg.ID)

	// Build npm metadata response
	resp := map[string]interface{}{
		"name": pkg.Name,
		"versions": func() map[string]interface{} {
			vs := make(map[string]interface{})
			for _, v := range versions {
				var meta map[string]interface{}
				json.Unmarshal(v.Metadata, &meta)
				if meta == nil {
					meta = map[string]interface{}{}
				}
				meta["version"] = v.Version
				meta["dist"] = map[string]interface{}{
					"tarball": fmt.Sprintf("%s/api/packages/npm/%s/-/%s-%s.tgz",
						h.cfg.HTTP.BaseURL, pkg.Name, name, v.Version),
				}
				vs[v.Version] = meta
			}
			return vs
		}(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *NPMHandler) DownloadTarball(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	scope := chi.URLParam(r, "scope")
	tarball := chi.URLParam(r, "tarball")
	pkgName := name
	if scope != "" {
		pkgName = scope + "/" + name
	}

	// Extract version from tarball name (e.g., "foo-1.0.0.tgz")
	version := strings.TrimPrefix(tarball, name+"-")
	version = strings.TrimSuffix(version, ".tgz")

	user, _ := h.authenticateRequest(r)

	var pkg *models.Package
	if user != nil {
		pkg, _ = h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "npm", pkgName)
	}
	if pkg == nil {
		pkg, _ = h.findPublicPackage(r, "npm", pkgName)
	}
	if pkg == nil {
		http.NotFound(w, r)
		return
	}

	if pkg.IsPrivate && (user == nil || user.ID != pkg.OwnerID) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	ver, err := h.packageStore.GetVersion(r.Context(), pkg.ID, version)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(h.cfg.Packages.DataDir, ver.DiskPath)
	http.ServeFile(w, r, filePath)
}

func (h *NPMHandler) Publish(w http.ResponseWriter, r *http.Request) {
	user, err := h.authenticateRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	scope := chi.URLParam(r, "scope")
	pkgName := name
	if scope != "" {
		pkgName = scope + "/" + name
	}

	var body struct {
		Name     string                            `json:"name"`
		Versions map[string]map[string]interface{} `json:"versions"`
		Attachments map[string]struct {
			Data string `json:"data"`
		} `json:"_attachments"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 100<<20)).Decode(&body); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Get or create package
	pkg, err := h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "npm", pkgName)
	if err == sql.ErrNoRows || pkg == nil {
		pkg = &models.Package{
			OwnerID: user.ID,
			Name:    pkgName,
			Type:    "npm",
		}
		if err := h.packageStore.Create(r.Context(), pkg); err != nil {
			http.Error(w, "Failed to create package", http.StatusInternalServerError)
			return
		}
	}

	for version, meta := range body.Versions {
		// Check if version already exists
		if _, err := h.packageStore.GetVersion(r.Context(), pkg.ID, version); err == nil {
			http.Error(w, "Version already exists", http.StatusConflict)
			return
		}

		// Save tarball
		tarballName := fmt.Sprintf("%s-%s.tgz", name, version)
		attachment, ok := body.Attachments[tarballName]
		if !ok {
			http.Error(w, "Missing attachment", http.StatusBadRequest)
			return
		}

		// Decode base64 tarball data
		diskDir := filepath.Join("npm", user.Username, pkgName, version)
		absDir := filepath.Join(h.cfg.Packages.DataDir, diskDir)
		os.MkdirAll(absDir, 0755)

		diskPath := filepath.Join(diskDir, "package.tgz")
		absPath := filepath.Join(h.cfg.Packages.DataDir, diskPath)

		data, err := base64.StdEncoding.DecodeString(attachment.Data)
		if err != nil {
			http.Error(w, "Invalid attachment data", http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			http.Error(w, "Failed to save package", http.StatusInternalServerError)
			return
		}

		metaJSON, _ := json.Marshal(meta)
		pv := &models.PackageVersion{
			PackageID: pkg.ID,
			Version:   version,
			Metadata:  metaJSON,
			SizeBytes: int64(len(data)),
			DiskPath:  diskPath,
		}
		if err := h.packageStore.CreateVersion(r.Context(), pv); err != nil {
			http.Error(w, "Failed to save version", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (h *NPMHandler) Unpublish(w http.ResponseWriter, r *http.Request) {
	user, err := h.authenticateRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	scope := chi.URLParam(r, "scope")
	pkgName := name
	if scope != "" {
		pkgName = scope + "/" + name
	}

	pkg, err := h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "npm", pkgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if pkg.OwnerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete the specific version from tarball name
	tarball := chi.URLParam(r, "tarball")
	version := strings.TrimPrefix(tarball, name+"-")
	version = strings.TrimSuffix(version, ".tgz")

	ver, err := h.packageStore.GetVersion(r.Context(), pkg.ID, version)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Delete file
	absPath := filepath.Join(h.cfg.Packages.DataDir, ver.DiskPath)
	os.Remove(absPath)

	h.packageStore.DeleteVersion(r.Context(), ver.ID)

	h.auditStore.Log(r.Context(), &user.ID, "package.version.delete", "package", &pkg.ID,
		map[string]interface{}{"name": pkg.Name, "version": version}, r.RemoteAddr)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (h *NPMHandler) findPublicPackage(r *http.Request, pkgType, name string) (*models.Package, error) {
	// Search across all users for a public package with this name
	// This uses a separate query since GetByOwnerAndName requires an owner
	return h.packageStore.FindPublicByName(r.Context(), pkgType, name)
}

func (h *NPMHandler) proxyNPMMetadata(w http.ResponseWriter, r *http.Request, pkgName string) {
	upstream := h.cfg.Packages.NPMUpstream + "/" + pkgName
	resp, err := http.Get(upstream)
	if err != nil {
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
