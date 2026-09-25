package liblicense

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnit_ThinkingDiscountMultiplier_DefaultAndExplicitZero(t *testing.T) {
	claims := NewClaims("license", "issuer", "subject")
	require.Equal(t, 1.0, ThinkingDiscountMultiplierFor(&claims, "reasoner"))

	claims.SetInt64(ThinkingDiscountMultiplierKey("reasoner"), 2500)
	require.Equal(t, 0.25, ThinkingDiscountMultiplierFor(&claims, "reasoner"))

	claims.SetInt64(ThinkingDiscountMultiplierKey("reasoner"), 0)
	require.Zero(t, ThinkingDiscountMultiplierFor(&claims, "reasoner"))
}

func TestUnit_CacheDiscountMultiplier_PreservesExplicitZero(t *testing.T) {
	claims := NewClaims("license", "issuer", "subject")
	claims.SetInt64(CacheDiscountMultiplierKey("reasoner"), 0)
	require.Zero(t, CacheDiscountMultiplierFor(&claims, "reasoner"))
}
