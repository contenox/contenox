package capacity

import "testing"

func TestUnit_KVBytesPerToken(t *testing.T) {
	// 28 layers, 2 KV heads, 128 head dim, f16 (2 bytes), K+V:
	// 2 * 28 * 2 * 128 * 2 = 28672 bytes/token.
	if got := KVBytesPerToken(28, 2, 128, "f16"); got != 28672 {
		t.Fatalf("KVBytesPerToken = %d, want 28672", got)
	}
	if got := KVBytesPerToken(28, 2, 128, "q8_0"); got != 14336 {
		t.Fatalf("q8_0 KVBytesPerToken = %d, want 14336", got)
	}
	if got := KVBytesPerToken(0, 2, 128, "f16"); got != 0 {
		t.Fatalf("unknown layers should yield 0, got %d", got)
	}
}

func TestUnit_Resolve_AmpleMemoryUsesModelMax(t *testing.T) {
	c := Resolve(Params{
		ModelMaxCtx:     32768,
		KVBytesPerToken: 28672,
		WeightsBytes:    1 << 30,  // 1 GiB weights
		FreeBytes:       64 << 30, // 64 GiB free
		HeadroomFrac:    0.1,
	})
	if c.EffectiveContext != 32768 {
		t.Fatalf("ample memory should give model max 32768, got %d", c.EffectiveContext)
	}
}

func TestUnit_Resolve_ScarceMemoryClampsBelowMax(t *testing.T) {
	// ~512 MiB free, 28672 B/token → budget ≈ 0.9*512MiB ≈ 483MiB → ~17.6k tokens.
	c := Resolve(Params{
		ModelMaxCtx:     32768,
		KVBytesPerToken: 28672,
		WeightsBytes:    0,
		FreeBytes:       512 << 20,
		HeadroomFrac:    0.1,
	})
	if c.EffectiveContext <= 0 || c.EffectiveContext >= 32768 {
		t.Fatalf("scarce memory should clamp below model max, got %d", c.EffectiveContext)
	}
	free := int64(512 << 20)
	want := int(int64(float64(free)*0.9) / 28672)
	if c.EffectiveContext != want {
		t.Fatalf("EffectiveContext = %d, want %d", c.EffectiveContext, want)
	}
}

func TestUnit_Resolve_RequestCaps(t *testing.T) {
	c := Resolve(Params{ModelMaxCtx: 32768, KVBytesPerToken: 28672, FreeBytes: 64 << 30, Request: 8192})
	if c.EffectiveContext != 8192 {
		t.Fatalf("request should cap to 8192, got %d", c.EffectiveContext)
	}
}

func TestUnit_Resolve_UnknownKVFallsBackToModelMax(t *testing.T) {
	c := Resolve(Params{ModelMaxCtx: 32768, KVBytesPerToken: 0, FreeBytes: 1 << 20})
	if c.EffectiveContext != 32768 {
		t.Fatalf("unknown KV cost should fall back to model max, got %d", c.EffectiveContext)
	}
}

func TestUnit_Resolve_WeightsExceedMemoryYieldsZero(t *testing.T) {
	c := Resolve(Params{ModelMaxCtx: 32768, KVBytesPerToken: 28672, WeightsBytes: 2 << 30, FreeBytes: 1 << 30})
	if c.EffectiveContext != 0 {
		t.Fatalf("weights exceeding memory should yield 0 window, got %d", c.EffectiveContext)
	}
}

func TestUnit_SystemRAM_ReportsPositive(t *testing.T) {
	got, err := SystemRAM{}.FreeBytes()
	if err != nil {
		t.Fatalf("SystemRAM: %v", err)
	}
	if got <= 0 {
		t.Fatalf("SystemRAM free = %d, want > 0", got)
	}
}
