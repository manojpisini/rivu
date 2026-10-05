package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrNotFound reports a dotted key that does not exist in the
// configuration (config get/set, P2.22). The CLI maps it to exit 3.
var ErrNotFound = errors.New("unknown config key")

// Get returns the effective value at a dotted key such as
// "workspace.root" or "flow.stale_threshold_days". List values join
// with commas; sections are not values.
func Get(c Config, key string) (string, error) {
	m, err := toMap(c)
	if err != nil {
		return "", err
	}
	v, err := lookup(m, key)
	if err != nil {
		return "", err
	}
	return formatValue(key, v)
}

// Set assigns value at a dotted key on c, converting against the key's
// current type: strings as-is, ints and bools parsed, lists
// comma-separated. Unknown keys return ErrNotFound so a typo never
// writes a stray entry; new entries inside per-language or weights
// maps are added with config edit.
func Set(c *Config, key, value string) error {
	m, err := toMap(*c)
	if err != nil {
		return err
	}
	parts := strings.Split(key, ".")
	var cur any = m
	for _, part := range parts[:len(parts)-1] {
		mm, ok := cur.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		v, ok := mm[part]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		cur = v
	}
	pm, ok := cur.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	leaf := parts[len(parts)-1]
	old, ok := pm[leaf]
	if !ok || old == nil {
		return fmt.Errorf("%w: %s (for new map entries, use `rivu config edit`)", ErrNotFound, key)
	}
	nv, err := convert(old, value, key)
	if err != nil {
		return err
	}
	pm[leaf] = nv
	out, err := fromMap(m)
	if err != nil {
		return err
	}
	*c = out
	return nil
}

// toMap round-trips c through TOML into a mutable tree; Load guarantees
// defaults for every key, so the tree is complete.
func toMap(c Config) (map[string]any, error) {
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	var m map[string]any
	if _, err := toml.Decode(buf.String(), &m); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return m, nil
}

func fromMap(m map[string]any) (Config, error) {
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return Config{}, fmt.Errorf("encode config: %w", err)
	}
	var c Config
	if _, err := toml.Decode(buf.String(), &c); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	return c, nil
}

func lookup(m map[string]any, key string) (any, error) {
	if key == "" {
		return nil, fmt.Errorf("%w: empty key", ErrNotFound)
	}
	var cur any = m
	parts := strings.Split(key, ".")
	for i, part := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		v, ok := mm[part]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		if i == len(parts)-1 {
			return v, nil
		}
		cur = v
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
}

func formatValue(key string, v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = fmt.Sprintf("%v", x)
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("%s is a section, not a value — use a leaf key like %s.something", key, key)
	}
}

func convert(old any, value, key string) (any, error) {
	switch old.(type) {
	case string:
		return value, nil
	case bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		return b, nil
	case int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be an integer, got %q", key, value)
		}
		return n, nil
	case float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number, got %q", key, value)
		}
		return f, nil
	case []any:
		if value == "" {
			return []any{}, nil
		}
		items := strings.Split(value, ",")
		out := make([]any, len(items))
		for i, s := range items {
			out[i] = strings.TrimSpace(s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s is a section, not a value — use a leaf key", key)
	}
}
