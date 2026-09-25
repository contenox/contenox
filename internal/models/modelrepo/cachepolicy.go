package modelrepo

import "time"

// ModelCachePolicy is how long a backend's observed model list may be cached
// before it is enumerated again. It is a provider implementation detail: a
// catalog knows whether its list is a stable truth or a transient absence.
//
// A zero duration means the package default (ProviderCacheDuration in
// runtimestate). A negative duration means "do not cache" that outcome, so a
// catalog whose empty list is not yet meaningful (no license, not configured)
// refuses to let an empty result harden into an hour of "no models".
type ModelCachePolicy struct {
	Healthy time.Duration // non-empty list
	Empty   time.Duration // successful but empty list
	Error   time.Duration // failed enumeration (errors are not cached today)
}

// CachePolicyCatalog is the optional interface a CatalogProvider implements to
// declare its cache policy. Absent, runtimestate applies the package default.
type CachePolicyCatalog interface {
	CachePolicy() ModelCachePolicy
}
