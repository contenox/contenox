package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblicense"
)

// openingBoundary is how many leading messages of this request have already
// stopped changing, and the same boundary the cache assertion is stated
// against. It is one statement so the two cannot drift: a key derived from a
// different prefix than the one asserted stable is either a new session per
// turn or a cache breakpoint on a message that changed.
//
// The count is the conversation's own opening — the leading system instruction
// plus the first thing the caller said — and not a prefix of the request's
// length, because a request that happens to be short would otherwise be
// identified by its whole self and a session would change identity the moment
// it outgrew the boundary. A client resends its history, so the opening is the
// part that stays byte-identical; hashing the whole request would name a new
// session on every turn and pin nothing.
//
// Nothing is asserted on a first turn, which carries only the caller's own
// message: there is no prefix yet that a later turn repeats.
func openingBoundary(messages []modelrepo.Message) int {
	for i, message := range messages {
		if message.Role == "system" {
			continue
		}
		if i == 0 || messages[i-1].Role != "system" {
			return 0
		}
		return i + 1
	}
	return 0
}

// sessionKeyFor derives the cache-affinity key that pins one conversation to one
// provider and backend, so its prefix cache stays warm across turns.
//
// An explicit key is used as the client stated it. Otherwise the conversation's
// opening identifies it: the client's identity is part of the key because the
// same prompt from two clients is not one session, and the model is part of it
// because moving a conversation between models is a different cache.
//
// No opening and no explicit key returns "", which resolves randomly — the
// behaviour of a request that carries no session identity at all.
func sessionKeyFor(key *runtimetypes.ProxyKey, claims *liblicense.Claims, model, explicit string, messages []modelrepo.Message) string {
	if explicit != "" {
		return explicit
	}
	identity := clientIdentity(key, claims)
	if identity == "" {
		return ""
	}
	boundary := openingBoundary(messages)
	if boundary == 0 {
		return ""
	}
	h := sha256.New()
	part(h, "contenox-gateway-session:")
	part(h, identity)
	part(h, model)
	for _, message := range messages[:boundary] {
		part(h, message.Role)
		part(h, message.Content)
		part(h, strconv.Itoa(len(message.Images)))
		part(h, strconv.Itoa(len(message.Audio)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sessionHints is where a conversation's stable prefix ends: the opening, which
// is the only part of a resend the gateway has watched hold still. A later
// message is stable in the common case, but nothing here can tell a client that
// appends from one that rewrites its middle, so it is not asserted.
//
// The assertion is made for a request that named a session and no other, because
// stability is what a session is.
//
// The key is deliberately absent from the hints. A provider's prompt cache is
// keyed on the prefix itself, so scoping it per client would fragment a cache
// that two callers sending the same opening should share.
func sessionHints(sessionKey string, messages []modelrepo.Message) *llmrepo.CacheHints {
	if sessionKey == "" {
		return nil
	}
	boundary := openingBoundary(messages)
	if boundary == 0 {
		return nil
	}
	return &llmrepo.CacheHints{
		StableSystem:     true,
		StableTools:      true,
		StableHistoryLen: boundary,
	}
}

// clientIdentity is the caller this session belongs to: the key that was
// presented, so revoking and re-minting a key starts a new cache rather than
// inheriting a pinned backend.
func clientIdentity(key *runtimetypes.ProxyKey, claims *liblicense.Claims) string {
	if key != nil {
		if key.ID != "" {
			return key.ID
		}
		if key.KeyHash != "" {
			return key.KeyHash
		}
	}
	if claims != nil {
		return claims.Subject
	}
	return ""
}

// part writes one field of the digest with a NUL separator, so ("ab", "c") can
// never hash the same as ("a", "bc"). The value is written as it arrived: this
// digest names a conversation, and normalising it would make two different
// openings share one.
func part(h hash.Hash, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}
