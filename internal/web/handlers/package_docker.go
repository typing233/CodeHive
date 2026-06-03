package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/webhook"
	"github.com/go-chi/chi/v5"
)

type DockerHandler struct {
	packageStore *store.PackageStore
	userStore    *store.UserStore
	tokenStore   *store.TokenStore
	auditStore   *store.AuditStore
	webhookSvc   *webhook.Dispatcher
	cfg          *config.Config
	uploads      sync.Map
}

type uploadState struct {
	Path string
	Size int64
}

func NewDockerHandler(ps *store.PackageStore, us *store.UserStore, ts *store.TokenStore, as *store.AuditStore, wd *webhook.Dispatcher, cfg *config.Config) *DockerHandler {
	return &DockerHandler{
		packageStore: ps,
		userStore:    us,
		tokenStore:   ts,
		auditStore:   as,
		webhookSvc:   wd,
		cfg:          cfg,
	}
}

func (h *DockerHandler) authenticateDocker(r *http.Request) (*models.User, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, fmt.Errorf("no auth")
	}

	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		hash := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(hash[:])
		return h.tokenStore.GetUserByTokenHash(r.Context(), tokenHash)
	}

	return nil, fmt.Errorf("unsupported auth")
}

func (h *DockerHandler) VersionCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func (h *DockerHandler) GetManifest(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	reference := chi.URLParam(r, "reference")

	user, _ := h.authenticateDocker(r)

	pkg, err := h.findDockerPackage(r, name, user)
	if err != nil {
		h.writeError(w, "NAME_UNKNOWN", "repository not found", http.StatusNotFound)
		return
	}

	if pkg.IsPrivate && (user == nil || user.ID != pkg.OwnerID) {
		h.writeError(w, "UNAUTHORIZED", "authentication required", http.StatusUnauthorized)
		return
	}

	ver, err := h.packageStore.GetVersion(r.Context(), pkg.ID, reference)
	if err != nil {
		h.writeError(w, "MANIFEST_UNKNOWN", "manifest not found", http.StatusNotFound)
		return
	}

	manifestPath := filepath.Join(h.cfg.Packages.DataDir, ver.DiskPath)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		h.writeError(w, "MANIFEST_UNKNOWN", "manifest not found", http.StatusNotFound)
		return
	}

	contentType := "application/vnd.docker.distribution.manifest.v2+json"
	var meta map[string]interface{}
	if json.Unmarshal(ver.Metadata, &meta) == nil {
		if ct, ok := meta["mediaType"].(string); ok {
			contentType = ct
		}
	}

	digest := sha256.Sum256(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Docker-Content-Digest", "sha256:"+hex.EncodeToString(digest[:]))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func (h *DockerHandler) PutManifest(w http.ResponseWriter, r *http.Request) {
	user, err := h.authenticateDocker(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "authentication required", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	reference := chi.URLParam(r, "reference")

	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		h.writeError(w, "MANIFEST_INVALID", "cannot read body", http.StatusBadRequest)
		return
	}

	pkg, _ := h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "docker", name)
	if pkg == nil {
		pkg = &models.Package{
			OwnerID: user.ID,
			Name:    name,
			Type:    "docker",
		}
		if err := h.packageStore.Create(r.Context(), pkg); err != nil {
			h.writeError(w, "INTERNAL_ERROR", "failed to create package", http.StatusInternalServerError)
			return
		}
	}

	digest := sha256.Sum256(body)
	digestStr := "sha256:" + hex.EncodeToString(digest[:])

	diskDir := filepath.Join("docker", "manifests", user.Username, name)
	absDir := filepath.Join(h.cfg.Packages.DataDir, diskDir)
	os.MkdirAll(absDir, 0755)

	diskPath := filepath.Join(diskDir, reference+".json")
	absPath := filepath.Join(h.cfg.Packages.DataDir, diskPath)
	if err := os.WriteFile(absPath, body, 0644); err != nil {
		h.writeError(w, "INTERNAL_ERROR", "failed to save manifest", http.StatusInternalServerError)
		return
	}

	meta, _ := json.Marshal(map[string]interface{}{
		"mediaType": r.Header.Get("Content-Type"),
		"digest":    digestStr,
	})

	ver := &models.PackageVersion{
		PackageID: pkg.ID,
		Version:   reference,
		Metadata:  meta,
		SizeBytes: int64(len(body)),
		DiskPath:  diskPath,
	}

	if old, err := h.packageStore.GetVersion(r.Context(), pkg.ID, reference); err == nil {
		h.packageStore.DeleteVersion(r.Context(), old.ID)
	}
	h.packageStore.CreateVersion(r.Context(), ver)

	h.webhookSvc.Dispatch(r.Context(), 0, "package.published", map[string]interface{}{
		"type":      "docker",
		"name":      name,
		"reference": reference,
		"owner":     user.Username,
	})

	w.Header().Set("Docker-Content-Digest", digestStr)
	w.Header().Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", name, reference))
	w.WriteHeader(http.StatusCreated)
}

func (h *DockerHandler) DeleteManifest(w http.ResponseWriter, r *http.Request) {
	user, err := h.authenticateDocker(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "authentication required", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	reference := chi.URLParam(r, "reference")

	pkg, _ := h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "docker", name)
	if pkg == nil || pkg.OwnerID != user.ID {
		h.writeError(w, "UNAUTHORIZED", "forbidden", http.StatusForbidden)
		return
	}

	ver, err := h.packageStore.GetVersion(r.Context(), pkg.ID, reference)
	if err != nil {
		h.writeError(w, "MANIFEST_UNKNOWN", "not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(h.cfg.Packages.DataDir, ver.DiskPath)
	os.Remove(absPath)
	h.packageStore.DeleteVersion(r.Context(), ver.ID)

	h.auditStore.Log(r.Context(), &user.ID, "package.manifest.delete", "package", &pkg.ID,
		map[string]interface{}{"name": name, "reference": reference}, r.RemoteAddr)

	h.webhookSvc.Dispatch(r.Context(), 0, "package.deleted", map[string]interface{}{
		"type":      "docker",
		"name":      name,
		"reference": reference,
		"owner":     user.Username,
	})

	w.WriteHeader(http.StatusAccepted)
}

func (h *DockerHandler) HeadBlob(w http.ResponseWriter, r *http.Request) {
	digest := chi.URLParam(r, "digest")

	blob, err := h.packageStore.GetBlob(r.Context(), digest)
	if err != nil {
		h.writeError(w, "BLOB_UNKNOWN", "blob not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(blob.SizeBytes, 10))
	w.Header().Set("Docker-Content-Digest", blob.Digest)
	w.WriteHeader(http.StatusOK)
}

func (h *DockerHandler) GetBlob(w http.ResponseWriter, r *http.Request) {
	digest := chi.URLParam(r, "digest")

	blob, err := h.packageStore.GetBlob(r.Context(), digest)
	if err != nil {
		h.writeError(w, "BLOB_UNKNOWN", "blob not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(h.cfg.Packages.DataDir, blob.DiskPath)
	http.ServeFile(w, r, absPath)
}

func (h *DockerHandler) InitBlobUpload(w http.ResponseWriter, r *http.Request) {
	_, err := h.authenticateDocker(r)
	if err != nil {
		h.writeError(w, "UNAUTHORIZED", "authentication required", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")

	uuid := generateUploadUUID()
	tmpDir := filepath.Join(h.cfg.Packages.DataDir, "docker", "uploads")
	os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, uuid)

	f, err := os.Create(tmpPath)
	if err != nil {
		h.writeError(w, "INTERNAL_ERROR", "cannot create upload", http.StatusInternalServerError)
		return
	}
	f.Close()

	h.uploads.Store(uuid, &uploadState{Path: tmpPath})

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, uuid))
	w.Header().Set("Docker-Upload-UUID", uuid)
	w.Header().Set("Range", "0-0")
	w.WriteHeader(http.StatusAccepted)
}

func (h *DockerHandler) ChunkBlobUpload(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	name := chi.URLParam(r, "name")

	val, ok := h.uploads.Load(uuid)
	if !ok {
		h.writeError(w, "BLOB_UPLOAD_UNKNOWN", "upload not found", http.StatusNotFound)
		return
	}
	state := val.(*uploadState)

	f, err := os.OpenFile(state.Path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		h.writeError(w, "INTERNAL_ERROR", "cannot write", http.StatusInternalServerError)
		return
	}
	n, _ := io.Copy(f, r.Body)
	f.Close()
	state.Size += n

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, uuid))
	w.Header().Set("Docker-Upload-UUID", uuid)
	w.Header().Set("Range", fmt.Sprintf("0-%d", state.Size-1))
	w.WriteHeader(http.StatusAccepted)
}

func (h *DockerHandler) CompleteBlobUpload(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	digest := r.URL.Query().Get("digest")

	val, ok := h.uploads.Load(uuid)
	if !ok {
		h.writeError(w, "BLOB_UPLOAD_UNKNOWN", "upload not found", http.StatusNotFound)
		return
	}
	state := val.(*uploadState)

	if r.Body != nil {
		f, _ := os.OpenFile(state.Path, os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			n, _ := io.Copy(f, r.Body)
			state.Size += n
			f.Close()
		}
	}

	blobDir := filepath.Join(h.cfg.Packages.DataDir, "docker", "blobs", "sha256")
	os.MkdirAll(blobDir, 0755)

	digestHex := strings.TrimPrefix(digest, "sha256:")
	finalPath := filepath.Join("docker", "blobs", "sha256", digestHex)
	absFinal := filepath.Join(h.cfg.Packages.DataDir, finalPath)

	os.Rename(state.Path, absFinal)
	h.uploads.Delete(uuid)

	h.packageStore.GetOrCreateBlob(r.Context(), digest, state.Size, finalPath)

	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", chi.URLParam(r, "name"), digest))
	w.WriteHeader(http.StatusCreated)
}

func (h *DockerHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	user, _ := h.authenticateDocker(r)

	pkg, err := h.findDockerPackage(r, name, user)
	if err != nil {
		h.writeError(w, "NAME_UNKNOWN", "repository not found", http.StatusNotFound)
		return
	}

	versions, _ := h.packageStore.ListVersions(r.Context(), pkg.ID)
	tags := make([]string, 0, len(versions))
	for _, v := range versions {
		tags = append(tags, v.Version)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name": name,
		"tags": tags,
	})
}

func (h *DockerHandler) findDockerPackage(r *http.Request, name string, user *models.User) (*models.Package, error) {
	if user != nil {
		pkg, err := h.packageStore.GetByOwnerAndName(r.Context(), user.ID, "docker", name)
		if err == nil {
			return pkg, nil
		}
	}
	return h.packageStore.FindPublicByName(r.Context(), "docker", name)
}

func (h *DockerHandler) writeError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]interface{}{
			{"code": code, "message": message},
		},
	})
}

func generateUploadUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
