package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	AuthDir string
}

type Credential struct {
	AccessToken  interface{} `json:"access_token"`
	AccountID    string      `json:"account_id"`
	Disabled     interface{} `json:"disabled"`
	Email        string      `json:"email"`
	Expired      interface{} `json:"expired"`
	IDToken      interface{} `json:"id_token"`
	LastRefresh  interface{} `json:"last_refresh"`
	RefreshToken interface{} `json:"refresh_token"`
	Type         string      `json:"type"`
}

type UsageWindow struct {
	UsedPercent *float64 `json:"usedPercent"`
	ResetAt     string   `json:"resetAt"`
}

type AccountUsage struct {
	Window5H UsageWindow `json:"window5h"`
	Weekly   UsageWindow `json:"weekly"`
	PlanType string      `json:"planType"`
	Status   string      `json:"status"`
	Message  string      `json:"message"`
}

type AccountRecord struct {
	File             string       `json:"file"`
	Email            string       `json:"email"`
	Type             string       `json:"type"`
	AccountID        string       `json:"accountId"`
	Disabled         bool         `json:"disabled"`
	Expired          bool         `json:"expired"`
	LastRefresh      string       `json:"lastRefresh"`
	MTime            string       `json:"mtime"`
	Status           string       `json:"status"`
	LastProbeAt      string       `json:"lastProbeAt"`
	LastProbeMessage string       `json:"lastProbeMessage"`
	Usage            AccountUsage `json:"usage"`
}

type AccountDetail struct {
	AccountRecord
	AccessTokenPresent  bool `json:"accessTokenPresent"`
	RefreshTokenPresent bool `json:"refreshTokenPresent"`
	IDTokenPresent      bool `json:"idTokenPresent"`
}

type fileSnapshot struct {
	Size    int64
	ModUnix int64
}

func (s Store) List() ([]AccountRecord, error) {
	entries, err := os.ReadDir(s.AuthDir)
	if err != nil {
		return nil, fmt.Errorf("read auth dir: %w", err)
	}

	records := make([]AccountRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isCandidateFile(name) {
			continue
		}

		record, recErr := s.readRecordFromFile(name)
		if recErr != nil {
			continue
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Email == records[j].Email {
			return records[i].File < records[j].File
		}
		return records[i].Email < records[j].Email
	})

	return records, nil
}

func (s Store) ReadDetail(file string) (AccountDetail, error) {
	path, err := ResolveCredentialPath(s.AuthDir, file)
	if err != nil {
		return AccountDetail{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return AccountDetail{}, fmt.Errorf("read credential file: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return AccountDetail{}, fmt.Errorf("decode credential file: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return AccountDetail{}, fmt.Errorf("stat credential file: %w", err)
	}

	return AccountDetail{
		AccountRecord: AccountRecord{
			File:        file,
			Email:       strings.TrimSpace(cred.Email),
			Type:        deriveAccountType(file, strings.TrimSpace(cred.Type)),
			AccountID:   strings.TrimSpace(cred.AccountID),
			Disabled:    parseDisabled(cred.Disabled),
			Expired:     parseExpired(cred.Expired),
			LastRefresh: formatUnknown(cred.LastRefresh),
			MTime:       info.ModTime().UTC().Format(time.RFC3339),
			Status:      "unknown",
			Usage:       defaultUsage(),
		},
		AccessTokenPresent:  !isEmptyUnknown(cred.AccessToken),
		RefreshTokenPresent: !isEmptyUnknown(cred.RefreshToken),
		IDTokenPresent:      !isEmptyUnknown(cred.IDToken),
	}, nil
}

func (s Store) Delete(file string) error {
	path, err := ResolveCredentialPath(s.AuthDir, file)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func (s Store) Snapshot() (map[string]fileSnapshot, error) {
	entries, err := os.ReadDir(s.AuthDir)
	if err != nil {
		return nil, fmt.Errorf("read auth dir: %w", err)
	}

	snapshot := map[string]fileSnapshot{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isCandidateFile(name) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		snapshot[name] = fileSnapshot{Size: info.Size(), ModUnix: info.ModTime().UnixNano()}
	}
	return snapshot, nil
}

func DetectLatestChanged(before, after map[string]fileSnapshot) string {
	latest := ""
	latestMod := int64(-1)

	for file, snap := range after {
		prev, ok := before[file]
		if !ok || prev.ModUnix != snap.ModUnix || prev.Size != snap.Size {
			if snap.ModUnix > latestMod {
				latest = file
				latestMod = snap.ModUnix
			}
		}
	}
	return latest
}

func ResolveCredentialPath(authDir, file string) (string, error) {
	if !isCandidateFile(file) {
		return "", fmt.Errorf("invalid credential filename")
	}
	if filepath.Base(file) != file {
		return "", fmt.Errorf("invalid credential filename")
	}
	if strings.Contains(file, "..") || strings.ContainsAny(file, `/\\`) {
		return "", fmt.Errorf("invalid credential filename")
	}
	resolved := filepath.Join(authDir, file)
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve credential path: %w", err)
	}
	absBase, err := filepath.Abs(authDir)
	if err != nil {
		return "", fmt.Errorf("resolve auth dir: %w", err)
	}
	if absResolved != absBase && !strings.HasPrefix(absResolved, absBase+string(os.PathSeparator)) {
		return "", errors.New("credential path escaped auth dir")
	}
	return absResolved, nil
}

func (s Store) readRecordFromFile(file string) (AccountRecord, error) {
	path, err := ResolveCredentialPath(s.AuthDir, file)
	if err != nil {
		return AccountRecord{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return AccountRecord{}, err
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return AccountRecord{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return AccountRecord{}, err
	}

	return AccountRecord{
		File:        file,
		Email:       strings.TrimSpace(cred.Email),
		Type:        deriveAccountType(file, strings.TrimSpace(cred.Type)),
		AccountID:   strings.TrimSpace(cred.AccountID),
		Disabled:    parseDisabled(cred.Disabled),
		Expired:     parseExpired(cred.Expired),
		LastRefresh: formatUnknown(cred.LastRefresh),
		MTime:       info.ModTime().UTC().Format(time.RFC3339),
		Status:      "unknown",
		Usage:       defaultUsage(),
	}, nil
}

func isCandidateFile(name string) bool {
	return strings.HasPrefix(name, "codex-") && strings.HasSuffix(name, ".json")
}

func formatUnknown(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func isEmptyUnknown(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	default:
		return false
	}
}

func parseDisabled(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}

func parseExpired(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return false
		}
		if strings.EqualFold(raw, "true") {
			return true
		}
		if strings.EqualFold(raw, "false") {
			return false
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.Before(time.Now())
		}
		return false
	default:
		return false
	}
}

func deriveAccountType(file, fromCredential string) string {
	switch {
	case strings.HasSuffix(file, "-team.json"):
		return "team"
	case strings.HasSuffix(file, "-plus.json"):
		return "plus"
	case strings.HasSuffix(file, "-pro.json"):
		return "pro"
	default:
		if fromCredential != "" {
			return fromCredential
		}
		return "unknown"
	}
}

func defaultUsage() AccountUsage {
	return AccountUsage{
		Status:  "unavailable",
		Message: "Usage unavailable",
	}
}
