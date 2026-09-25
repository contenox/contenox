package liblicense

import (
	"strconv"
	"strings"
	"time"
)

// Claims represents the full payload of a license, combining standard metadata
// with an arbitrary key-value mapping (KV) for feature flags, token quotas, tier, etc.
type Claims struct {
	// ID is the unique identifier for this license.
	ID string `json:"id"`

	// Issuer identifies who issued the license.
	Issuer string `json:"iss,omitempty"`

	// Subject identifies the licensee / user / customer / tenant.
	Subject string `json:"sub,omitempty"`

	// IssuedAt is the UTC timestamp when the license was created.
	IssuedAt time.Time `json:"iat"`

	// NotBefore is the optional UTC timestamp before which the license is not active.
	NotBefore *time.Time `json:"nbf,omitempty"`

	// ExpiresAt is the optional UTC timestamp after which the license is expired.
	ExpiresAt *time.Time `json:"exp,omitempty"`

	// KV holds arbitrary key-value pairs (e.g., "tokens": "500000", "tier": "enterprise").
	KV map[string]string `json:"kv,omitempty"`
}

// NewClaims initializes a Claims instance with a generated ID and IssuedAt set to current time.
func NewClaims(id, issuer, subject string) Claims {
	return Claims{
		ID:       id,
		Issuer:   issuer,
		Subject:  subject,
		IssuedAt: time.Now().UTC().Truncate(time.Second),
		KV:       make(map[string]string),
	}
}

// Get returns the value associated with key, and whether the key was present.
func (c *Claims) Get(key string) (string, bool) {
	if c.KV == nil {
		return "", false
	}
	val, ok := c.KV[key]
	return val, ok
}

// GetString returns the value associated with key, or defaultVal if absent.
func (c *Claims) GetString(key, defaultVal string) string {
	if val, ok := c.Get(key); ok {
		return val
	}
	return defaultVal
}

// GetInt returns the integer value for key, or defaultVal if absent or unparseable.
func (c *Claims) GetInt(key string, defaultVal int) int {
	val, ok := c.Get(key)
	if !ok {
		return defaultVal
	}
	i, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return defaultVal
	}
	return i
}

// GetInt64 returns the int64 value for key, or defaultVal if absent or unparseable.
func (c *Claims) GetInt64(key string, defaultVal int64) int64 {
	val, ok := c.Get(key)
	if !ok {
		return defaultVal
	}
	i, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil {
		return defaultVal
	}
	return i
}

// GetBool returns the boolean value for key, or defaultVal if absent or unparseable.
func (c *Claims) GetBool(key string, defaultVal bool) bool {
	val, ok := c.Get(key)
	if !ok {
		return defaultVal
	}
	b, err := strconv.ParseBool(strings.TrimSpace(val))
	if err != nil {
		return defaultVal
	}
	return b
}

// HasFeature returns true if feature is enabled, checking both:
// 1. A comma/space-separated list of features in the "features" or "capabilities" key
// 2. A specific boolean key matching the feature name (e.g., "feature:ee_ai_tools" or "ee_ai_tools")
func (c *Claims) HasFeature(feature string) bool {
	if c.KV == nil || feature == "" {
		return false
	}

	if c.GetBool(feature, false) || c.GetBool("feature:"+feature, false) {
		return true
	}

	for _, listKey := range []string{"features", "capabilities"} {
		if raw, ok := c.Get(listKey); ok {
			items := strings.FieldsFunc(raw, func(r rune) bool {
				return r == ',' || r == ';' || r == ' ' || r == '\t'
			})
			for _, item := range items {
				if strings.EqualFold(strings.TrimSpace(item), feature) {
					return true
				}
			}
		}
	}

	return false
}

// Set stores a string key-value pair.
func (c *Claims) Set(key, val string) {
	if c.KV == nil {
		c.KV = make(map[string]string)
	}
	c.KV[key] = val
}

// SetInt stores an integer value under key.
func (c *Claims) SetInt(key string, val int) {
	c.Set(key, strconv.Itoa(val))
}

// SetInt64 stores an int64 value under key.
func (c *Claims) SetInt64(key string, val int64) {
	c.Set(key, strconv.FormatInt(val, 10))
}

// SetBool stores a boolean value under key.
func (c *Claims) SetBool(key string, val bool) {
	c.Set(key, strconv.FormatBool(val))
}

// ValidateTimestamps checks whether the license is currently valid at time `now`.
func (c *Claims) ValidateTimestamps(now time.Time) error {
	if c.NotBefore != nil && now.Before(*c.NotBefore) {
		return ErrLicenseNotYetValid
	}
	if c.ExpiresAt != nil && !now.Before(*c.ExpiresAt) {
		return ErrLicenseExpired
	}
	return nil
}
