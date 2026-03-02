package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type FileEntry struct {
	LocalPath  string `toml:"local_path"`
	RemoteName string `toml:"remote_name"`
}

type GoogleDriveConfig struct {
	CredentialsFile string `toml:"credentials_file"`
	TokenFile       string `toml:"token_file"`
	FolderID        string `toml:"folder_id"`
}

type SynologyConfig struct {
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	HTTPS     bool   `toml:"https"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	VerifySSL bool   `toml:"verify_ssl"`
	SharePath string `toml:"share_path"`
	DeviceID  string `toml:"device_id"`
}

type GistConfig struct {
	Token  string `toml:"token"`
	GistID string `toml:"gist_id"`
}

type Config struct {
	ActiveBackend  string            `toml:"active_backend"`
	RemoteDir      string            `toml:"remote_dir"`
	BackupExpiry   string            `toml:"backup_expiry"`
	SupportedFiles []string          `toml:"supported_files"`
	Files          []FileEntry       `toml:"files"`
	GoogleDrive    GoogleDriveConfig `toml:"google_drive"`
	Synology       SynologyConfig    `toml:"synology"`
	Gist           GistConfig        `toml:"gist"`

	path string `toml:"-"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "shync", "config.toml")
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ActiveBackend:  "google_drive",
		RemoteDir:      "/shync",
		BackupExpiry:   "3mo",
		SupportedFiles: []string{".zshrc", "config"},
		GoogleDrive: GoogleDriveConfig{
			CredentialsFile: filepath.Join(home, ".config", "shync", "credentials.json"),
			TokenFile:       filepath.Join(home, ".config", "shync", "token.json"),
		},
		Synology: SynologyConfig{
			Port:  5001,
			HTTPS: true,
		},
		path: DefaultPath(),
	}
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.path = path
	return &cfg, nil
}

func (c *Config) Save() error {
	return c.SaveTo(c.path)
}

func (c *Config) SaveTo(path string) error {
	if path == "" {
		path = DefaultPath()
	}
	c.path = path

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *Config) Path() string {
	return c.path
}

// Get returns the value at the given dot-notation key.
func (c *Config) Get(key string) (string, error) {
	parts := strings.Split(key, ".")
	v := reflect.ValueOf(c).Elem()
	t := v.Type()

	return getField(v, t, parts)
}

// Set sets the value at the given dot-notation key.
func (c *Config) Set(key, value string) error {
	parts := strings.Split(key, ".")
	v := reflect.ValueOf(c).Elem()
	t := v.Type()

	return setField(v, t, parts, value)
}

// Remove resets the value at the given dot-notation key to its zero value.
// For files array entries (files.<index>), it removes the entry.
func (c *Config) Remove(key string) error {
	parts := strings.Split(key, ".")

	// Special case: remove a file entry by index
	if len(parts) >= 2 && parts[0] == "files" {
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid file index: %s", parts[1])
		}
		if idx < 0 || idx >= len(c.Files) {
			return fmt.Errorf("file index %d out of range (0-%d)", idx, len(c.Files)-1)
		}
		c.Files = append(c.Files[:idx], c.Files[idx+1:]...)
		return nil
	}

	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	return resetField(v, t, parts)
}

// ListAll returns all config key-value pairs in dot notation.
func (c *Config) ListAll() []KeyValue {
	var result []KeyValue
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	listFields(v, t, "", &result)
	return result
}

type KeyValue struct {
	Key   string
	Value string
}

// AddFile adds a file entry if not already tracked (by local_path).
func (c *Config) AddFile(localPath, remoteName string) bool {
	for _, f := range c.Files {
		if f.LocalPath == localPath {
			return false
		}
	}
	c.Files = append(c.Files, FileEntry{
		LocalPath:  localPath,
		RemoteName: remoteName,
	})
	return true
}

// FindFileByLocalPath returns the file entry with the given local path.
func (c *Config) FindFileByLocalPath(localPath string) *FileEntry {
	for i := range c.Files {
		if c.Files[i].LocalPath == localPath {
			return &c.Files[i]
		}
	}
	return nil
}

// FindFileByRemoteName returns the file entry with the given remote name.
func (c *Config) FindFileByRemoteName(name string) *FileEntry {
	for i := range c.Files {
		if c.Files[i].RemoteName == name {
			return &c.Files[i]
		}
	}
	return nil
}

func findFieldByTag(t reflect.Type, tag string) (int, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tomlTag := f.Tag.Get("toml")
		if tomlTag == "-" {
			continue
		}
		// Strip options like ",omitempty"
		if idx := strings.Index(tomlTag, ","); idx != -1 {
			tomlTag = tomlTag[:idx]
		}
		if tomlTag == tag {
			return i, true
		}
	}
	return 0, false
}

func getField(v reflect.Value, t reflect.Type, parts []string) (string, error) {
	if len(parts) == 0 {
		return formatValue(v), nil
	}

	// Handle files array
	if parts[0] == "files" {
		if len(parts) < 2 {
			return fmt.Sprintf("%d tracked files", v.FieldByName("Files").Len()), nil
		}
		filesField := v.FieldByName("Files")
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", fmt.Errorf("invalid file index: %s", parts[1])
		}
		if idx < 0 || idx >= filesField.Len() {
			return "", fmt.Errorf("file index %d out of range (0-%d)", idx, filesField.Len()-1)
		}
		elem := filesField.Index(idx)
		if len(parts) == 2 {
			return formatValue(elem), nil
		}
		return getField(elem, elem.Type(), parts[2:])
	}

	fieldIdx, ok := findFieldByTag(t, parts[0])
	if !ok {
		return "", fmt.Errorf("unknown config key: %s", parts[0])
	}
	fv := v.Field(fieldIdx)
	ft := t.Field(fieldIdx)

	if fv.Kind() == reflect.Struct {
		return getField(fv, ft.Type, parts[1:])
	}
	if fv.Kind() == reflect.Slice {
		if len(parts) == 1 {
			return formatValue(fv), nil
		}
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", fmt.Errorf("invalid index: %s", parts[1])
		}
		if idx < 0 || idx >= fv.Len() {
			return "", fmt.Errorf("index %d out of range (0-%d)", idx, fv.Len()-1)
		}
		elem := fv.Index(idx)
		if len(parts) == 2 {
			return formatValue(elem), nil
		}
		if elem.Kind() == reflect.Struct {
			return getField(elem, elem.Type(), parts[2:])
		}
		return "", fmt.Errorf("key %s.%s is not a section", parts[0], parts[1])
	}
	if len(parts) > 1 {
		return "", fmt.Errorf("key %s is not a section", parts[0])
	}
	return formatValue(fv), nil
}

func setField(v reflect.Value, t reflect.Type, parts []string, value string) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}

	// Handle files array
	if parts[0] == "files" {
		if len(parts) < 3 {
			return fmt.Errorf("use 'shync push' to add files, or set files.<index>.<field>")
		}
		filesField := v.FieldByName("Files")
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid file index: %s", parts[1])
		}
		if idx < 0 || idx >= filesField.Len() {
			return fmt.Errorf("file index %d out of range (0-%d)", idx, filesField.Len()-1)
		}
		elem := filesField.Index(idx)
		return setField(elem, elem.Type(), parts[2:], value)
	}

	fieldIdx, ok := findFieldByTag(t, parts[0])
	if !ok {
		return fmt.Errorf("unknown config key: %s", parts[0])
	}
	fv := v.Field(fieldIdx)
	ft := t.Field(fieldIdx)

	if fv.Kind() == reflect.Struct {
		return setField(fv, ft.Type, parts[1:], value)
	}
	if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
		if len(parts) == 1 {
			// Set entire list: comma-separated values.
			var vals []string
			for _, s := range strings.Split(value, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					vals = append(vals, s)
				}
			}
			fv.Set(reflect.ValueOf(vals))
			return nil
		}
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid index: %s", parts[1])
		}
		if idx < 0 || idx >= fv.Len() {
			return fmt.Errorf("index %d out of range (0-%d)", idx, fv.Len()-1)
		}
		fv.Index(idx).SetString(value)
		return nil
	}
	if len(parts) > 1 {
		return fmt.Errorf("key %s is not a section", parts[0])
	}
	return setValue(fv, value)
}

func resetField(v reflect.Value, t reflect.Type, parts []string) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}

	fieldIdx, ok := findFieldByTag(t, parts[0])
	if !ok {
		return fmt.Errorf("unknown config key: %s", parts[0])
	}
	fv := v.Field(fieldIdx)
	ft := t.Field(fieldIdx)

	if fv.Kind() == reflect.Struct && len(parts) > 1 {
		return resetField(fv, ft.Type, parts[1:])
	}
	if len(parts) > 1 {
		return fmt.Errorf("key %s is not a section", parts[0])
	}
	fv.Set(reflect.Zero(fv.Type()))
	return nil
}

func setValue(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer: %s", s)
		}
		v.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("invalid boolean: %s", s)
		}
		v.SetBool(b)
	default:
		return fmt.Errorf("unsupported field type: %s", v.Kind())
	}
	return nil
}

func formatValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Slice:
		var parts []string
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, formatValue(v.Index(i)))
		}
		return strings.Join(parts, ", ")
	case reflect.Struct:
		var parts []string
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			tag := t.Field(i).Tag.Get("toml")
			if tag == "-" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", tag, formatValue(v.Field(i))))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

func listFields(v reflect.Value, t reflect.Type, prefix string, result *[]KeyValue) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("toml")
		if tag == "-" || tag == "" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}

		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}

		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.Struct:
			listFields(fv, f.Type, key, result)
		case reflect.Slice:
			if fv.Type().Elem().Kind() == reflect.Struct {
				for j := 0; j < fv.Len(); j++ {
					elem := fv.Index(j)
					elemKey := fmt.Sprintf("%s.%d", key, j)
					listFields(elem, elem.Type(), elemKey, result)
				}
			} else {
				*result = append(*result, KeyValue{Key: key, Value: formatValue(fv)})
			}
		default:
			*result = append(*result, KeyValue{Key: key, Value: formatValue(fv)})
		}
	}
}

// ParseExpiry parses a human-friendly duration string like "12h", "7d", "2w", "3mo", "1y".
func ParseExpiry(s string) (time.Duration, error) {
	// Try "mo" suffix first (before single-char units) since it's two characters.
	if strings.HasSuffix(s, "mo") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "mo"))
		if err != nil {
			return 0, fmt.Errorf("invalid expiry value: %s", s)
		}
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	}

	if len(s) < 2 {
		return 0, fmt.Errorf("invalid expiry format: %s (expected <number><unit>, e.g. 3mo, 7d, 1y)", s)
	}

	unit := s[len(s)-1:]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid expiry value: %s", s)
	}

	switch unit {
	case "h":
		return time.Duration(n) * time.Hour, nil
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	case "w":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case "y":
		return time.Duration(n) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown expiry unit %q (use h, d, w, mo, or y)", unit)
	}
}
