// Package models discovers and activates local model profiles.
package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const metadataRollbackTimeout = 5 * time.Second

type Role string

const (
	RoleKWS     Role = "kws"
	RoleVAD     Role = "vad"
	RoleSTT     Role = "stt"
	RoleSpeaker Role = "speaker"
	RoleTTS     Role = "tts"
	RoleIntent  Role = "intent"
)

type Profile struct {
	ID      string `json:"id"`
	Role    Role   `json:"role"`
	Runtime string `json:"runtime"`
	Path    string `json:"path"`
	Entry   string `json:"entry"`
	SHA256  string `json:"sha256"`
	Valid   bool   `json:"valid"`
	Error   string `json:"error"`
	Active  bool   `json:"-"`
}

type manifest struct {
	ID      string `json:"id"`
	Role    Role   `json:"role"`
	Runtime string `json:"runtime"`
	Entry   string `json:"entry"`
	SHA256  string `json:"sha256"`
}

type Snapshot struct {
	Profiles []Profile        `json:"profiles"`
	Active   map[Role]Profile `json:"active"`
}

type Store interface {
	ReplaceSnapshot(context.Context, []Profile) error
	List(context.Context) ([]Profile, error)
}

type Registry struct {
	root   string
	store  Store
	swap   func(context.Context, Snapshot) error
	scanMu sync.Mutex
	mu     sync.RWMutex
	active map[Role]Profile
}

func NewRegistry(root string, store Store, swap func(context.Context, Snapshot) error) *Registry {
	return &Registry{root: root, store: store, swap: swap, active: make(map[Role]Profile)}
}

func (r *Registry) Reload(ctx context.Context) (Snapshot, error) { return r.Scan(ctx) }

func (r *Registry) Scan(ctx context.Context) (Snapshot, error) {
	r.scanMu.Lock()
	defer r.scanMu.Unlock()
	profiles, err := scan(r.root)
	if err != nil {
		return Snapshot{}, err
	}
	var previous []Profile
	if r.store != nil {
		previous, err = r.store.List(ctx)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load stored model snapshot: %w", err)
		}
		previous = registryProfiles(previous)
	}
	r.mu.RLock()
	active := cloneActive(r.active)
	r.mu.RUnlock()
	delete(active, RoleIntent)
	normalizeProfiles(profiles, active)
	selected := selectProfiles(profiles)
	for role, profile := range selected {
		active[role] = profile
	}
	snapshot := Snapshot{Profiles: profiles, Active: active}
	if r.store != nil {
		if err := r.store.ReplaceSnapshot(ctx, persistedProfiles(profiles, active)); err != nil {
			return snapshot, fmt.Errorf("store model snapshot: %w", err)
		}
	}
	if r.swap != nil {
		if err := r.swap(ctx, snapshot); err != nil {
			if r.store != nil {
				rollbackCtx, cancel := context.WithTimeout(context.Background(), metadataRollbackTimeout)
				defer cancel()
				if rollbackErr := r.store.ReplaceSnapshot(rollbackCtx, previous); rollbackErr != nil {
					return snapshot, errors.Join(fmt.Errorf("swap model snapshot: %w", err), fmt.Errorf("restore model snapshot: %w", rollbackErr))
				}
			}
			return snapshot, fmt.Errorf("swap model snapshot: %w", err)
		}
	}
	r.mu.Lock()
	r.active = active
	r.mu.Unlock()
	return snapshot, nil
}

func (r *Registry) Active(role Role) (Profile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	profile, ok := r.active[role]
	return profile, ok
}

func scan(root string) ([]Profile, error) {
	roles, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan model root %q: %w", root, err)
	}
	var profiles []Profile
	for _, roleDirectory := range roles {
		if !roleDirectory.IsDir() || roleDirectory.Name() == string(RoleIntent) {
			continue
		}
		rolePath := filepath.Join(root, roleDirectory.Name())
		profileDirectories, err := os.ReadDir(rolePath)
		if err != nil {
			return nil, fmt.Errorf("scan model role %q: %w", roleDirectory.Name(), err)
		}
		for _, profileDirectory := range profileDirectories {
			if !profileDirectory.IsDir() {
				continue
			}
			profilePath := filepath.Join(rolePath, profileDirectory.Name())
			manifestPath := filepath.Join(profilePath, "model.json")
			if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
				continue
			}
			profile := readProfile(root, roleDirectory.Name(), profilePath, manifestPath)
			if profile.Role != RoleIntent {
				profiles = append(profiles, profile)
			}
		}
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Path < profiles[j].Path })
	return profiles, nil
}

func normalizeProfiles(profiles []Profile, persisted map[Role]Profile) {
	byID := make(map[string][]int)
	for i, profile := range profiles {
		if profile.ID != "" {
			byID[profile.ID] = append(byID[profile.ID], i)
		}
	}
	for id, indexes := range byID {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			profiles[index].Valid = false
			profiles[index].Error = fmt.Sprintf("duplicate model id %q", id)
		}
	}
	for {
		selected := selectProfiles(profiles)
		retained := make(map[string]Profile)
		for role, profile := range persisted {
			if _, replaced := selected[role]; !replaced {
				retained[profile.ID] = profile
			}
		}
		changed := false
		for i := range profiles {
			if prior, collides := retained[profiles[i].ID]; profiles[i].Valid && collides && prior.Role != profiles[i].Role {
				profiles[i].Valid = false
				profiles[i].Error = fmt.Sprintf("model id %q conflicts with active role %q", profiles[i].ID, prior.Role)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	used := make(map[string]bool)
	for _, profile := range profiles {
		if profile.Valid {
			used[profile.ID] = true
		}
	}
	selected := selectProfiles(profiles)
	for role, profile := range persisted {
		if _, replaced := selected[role]; !replaced {
			used[profile.ID] = true
		}
	}
	for i := range profiles {
		if profiles[i].Valid {
			continue
		}
		base := "invalid:" + profiles[i].Path
		profiles[i].ID = base
		for suffix := 2; used[profiles[i].ID]; suffix++ {
			profiles[i].ID = fmt.Sprintf("%s#%d", base, suffix)
		}
		used[profiles[i].ID] = true
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
}

func selectProfiles(profiles []Profile) map[Role]Profile {
	selected := make(map[Role]Profile)
	for _, profile := range profiles {
		if _, exists := selected[profile.Role]; profile.Valid && !exists {
			selected[profile.Role] = profile
		}
	}
	return selected
}

func persistedProfiles(profiles []Profile, active map[Role]Profile) []Profile {
	persisted := append([]Profile(nil), profiles...)
	byID := make(map[string]int, len(persisted))
	for i := range persisted {
		persisted[i].Active = false
		byID[persisted[i].ID] = i
	}
	for _, role := range []Role{RoleKWS, RoleVAD, RoleSTT, RoleSpeaker, RoleTTS} {
		profile, ok := active[role]
		if !ok {
			continue
		}
		profile.Active = true
		if index, exists := byID[profile.ID]; exists {
			persisted[index].Active = true
		} else {
			byID[profile.ID] = len(persisted)
			persisted = append(persisted, profile)
		}
	}
	return persisted
}

func readProfile(root, directoryRole, profilePath, manifestPath string) Profile {
	relative, _ := filepath.Rel(root, profilePath)
	profile := Profile{Path: filepath.ToSlash(relative)}
	file, err := os.Open(manifestPath)
	if err != nil {
		profile.Error = err.Error()
		return profile
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var decoded manifest
	decodeErr := decoder.Decode(&decoded)
	profile.ID = decoded.ID
	profile.Role = decoded.Role
	profile.Runtime = decoded.Runtime
	profile.Entry = decoded.Entry
	profile.SHA256 = decoded.SHA256
	if decodeErr != nil {
		profile.Error = fmt.Sprintf("decode manifest: %v", decodeErr)
		return profile
	}
	if err := ensureEOF(decoder); err != nil {
		profile.Error = err.Error()
		return profile
	}
	if err := validateProfile(directoryRole, profilePath, &profile); err != nil {
		profile.Error = err.Error()
		return profile
	}
	profile.Valid = true
	return profile
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("manifest contains multiple JSON values")
		}
		return fmt.Errorf("decode manifest: %w", err)
	}
	return nil
}

func validateProfile(directoryRole, profilePath string, profile *Profile) error {
	if !validRole(profile.Role) || string(profile.Role) != directoryRole {
		return fmt.Errorf("invalid model role %q", profile.Role)
	}
	if profile.ID == "" || profile.Runtime == "" || profile.Entry == "" {
		return errors.New("id, runtime, and entry are required")
	}
	if filepath.IsAbs(profile.Entry) {
		return errors.New("entry path must be relative")
	}
	entryPath := filepath.Join(profilePath, filepath.FromSlash(profile.Entry))
	contained, err := pathContained(profilePath, entryPath)
	if err != nil || !contained {
		return errors.New("entry path escapes profile directory")
	}
	info, err := os.Stat(entryPath)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("entry %q must be a regular file", profile.Entry)
	}
	if profile.SHA256 != "" {
		file, err := os.Open(entryPath)
		if err != nil {
			return fmt.Errorf("open entry for checksum: %w", err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash entry: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close entry: %w", closeErr)
		}
		if !strings.EqualFold(profile.SHA256, hex.EncodeToString(hash.Sum(nil))) {
			return errors.New("sha256 mismatch")
		}
	}
	return nil
}

func pathContained(parent, child string) (bool, error) {
	absoluteParent, err := filepath.Abs(parent)
	if err != nil {
		return false, err
	}
	absoluteChild, err := filepath.Abs(child)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(absoluteParent, absoluteChild)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, err
	}
	current := absoluteParent
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, nil
		}
	}
	return true, nil
}

func validRole(role Role) bool {
	switch role {
	case RoleKWS, RoleVAD, RoleSTT, RoleSpeaker, RoleTTS:
		return true
	default:
		return false
	}
}

func registryProfiles(profiles []Profile) []Profile {
	filtered := profiles[:0]
	for _, profile := range profiles {
		if profile.Role != RoleIntent {
			filtered = append(filtered, profile)
		}
	}
	return filtered
}

func cloneActive(active map[Role]Profile) map[Role]Profile {
	clone := make(map[Role]Profile, len(active))
	for role, profile := range active {
		clone[role] = profile
	}
	return clone
}
