// Package style is beam's only StyleID→terminal-attributes table: the
// tier ladder (truecolor → 256 → 16 → mono), the capability snapshot that
// selects a tier, the brand ladder, and the glyph set that carries meaning
// when color is unavailable. No other package may construct an escape
// sequence or invent a color; Styles is the only path from role to attribute.
package style

import "github.com/contenox/contenox/internal/surfaces/beam/frame"

// resetSuffix ends any non-empty prefix. SGR 0 clears every attribute the
// prefix set, so spans never bleed into the text that follows them.
const resetSuffix = "\x1b[0m"

// Attribute-only SGR codes. These carry the same meaning at every color
// tier, so they need no per-profile variant the way colors do.
const (
	attrBold   = "\x1b[1m"
	attrDim    = "\x1b[2m"
	attrItalic = "\x1b[3m"
)

// ANSI16 foreground codes. The tier is the aixterm 16-color set (8
// standard + 8 bright); bright variants are used throughout for the
// "16-color readable" half of the tier doctrine.
const (
	fg16Red     = "\x1b[91m"
	fg16Green   = "\x1b[92m"
	fg16Yellow  = "\x1b[93m"
	fg16Blue    = "\x1b[94m"
	fg16Magenta = "\x1b[95m"
	fg16Cyan    = "\x1b[96m"
	fg16Gray    = "\x1b[90m"

	fg16BrandBright = "\x1b[93m"
	fg16BrandCore   = "\x1b[33m"
)

// ANSI256 foreground codes, indexed into the standard xterm 256-color
// palette (\x1b[38;5;Nm).
const (
	fg256Red        = "\x1b[38;5;203m"
	fg256Yellow     = "\x1b[38;5;221m"
	fg256Green      = "\x1b[38;5;114m"
	fg256Cyan       = "\x1b[38;5;116m"
	fg256Magenta    = "\x1b[38;5;176m"
	fg256Gray       = "\x1b[38;5;244m"
	fg256Code       = "\x1b[38;5;110m"
	fg256BrandDark  = "\x1b[38;5;221m"
	fg256BrandLight = "\x1b[38;5;136m"

	fg256Ramp1Dark  = "\x1b[38;5;221m"
	fg256Ramp2Dark  = "\x1b[38;5;178m"
	fg256Ramp3Dark  = "\x1b[38;5;228m"
	fg256Ramp1Light = "\x1b[38;5;178m"
	fg256Ramp2Light = "\x1b[38;5;136m"
	fg256Ramp3Light = "\x1b[38;5;221m"
)

// TrueColor foreground codes (\x1b[38;2;R;G;Bm).
const (
	fgTCRed        = "\x1b[38;2;248;113;113m"
	fgTCYellow     = "\x1b[38;2;250;204;21m"
	fgTCGreen      = "\x1b[38;2;74;222;128m"
	fgTCCyan       = "\x1b[38;2;34;211;238m"
	fgTCMagenta    = "\x1b[38;2;192;132;252m"
	fgTCGray       = "\x1b[38;2;107;114;128m"
	fgTCCode       = "\x1b[38;2;125;211;252m"
	fgTCBrandDark  = "\x1b[38;2;242;201;76m" // #F2C94C
	fgTCBrandLight = "\x1b[38;2;159;121;0m"  // #9F7900

	fgTCRamp1Dark  = "\x1b[38;2;242;201;76m"  // #F2C94C
	fgTCRamp2Dark  = "\x1b[38;2;220;174;24m"  // #DCAE18
	fgTCRamp3Dark  = "\x1b[38;2;255;232;120m" // #FFE878
	fgTCRamp1Light = "\x1b[38;2;220;174;24m"  // #DCAE18
	fgTCRamp2Light = "\x1b[38;2;159;121;0m"   // #9F7900
	fgTCRamp3Light = "\x1b[38;2;242;201;76m"  // #F2C94C
)

// Styles is the process-lifetime StyleID→SGR table for one Caps snapshot.
// It satisfies term.StyleResolver; construct exactly one per process via
// New and hand it to the terminal engine.
type Styles struct {
	table map[frame.StyleID]string
}

// New builds the resolver for caps. The role table is fixed at
// construction time — Styles never re-reads the environment or re-probes
// the terminal.
func New(caps Caps) *Styles {
	return &Styles{table: buildTable(caps)}
}

// SGR returns the SGR prefix/suffix pair for id. prefix is a single SGR
// sequence or empty; suffix is the reset sequence whenever prefix is
// non-empty, and empty otherwise. Every frame.StyleID resolves — an id
// missing from the table (impossible for the closed set in frame.All,
// but SGR must never panic on one) degrades to the empty pair, same as
// Mono.
func (s *Styles) SGR(id frame.StyleID) (prefix, suffix string) {
	prefix = s.table[id]
	if prefix == "" {
		return "", ""
	}
	return prefix, resetSuffix
}

// buildTable returns the role table for caps. Mono returns nil: every
// lookup on a nil map yields the zero value, so SGR strips all styling
// without a second code path — this is the doctrine, not an optimization.
//
// Role values (the same across dark/light except the brand family):
//
//	none, assistant, shell            empty (default foreground)
//	user, heading, strong, active     bold
//	em                                italic
//	thought, muted, skipped           dim
//	error, failed                     red
//	warn                              yellow
//	done                              green
//	pending                           cyan
//	code                              soft cyan/blue
//	hitl                              brand signal yellow
//	border, inactive, tool            bright-black (chrome)
//	brand                             signal yellow; ANSI16 = yellow
//	brand-ramp1/2/3                   logo-mark yellow ramp
//
// Every prefix here is foreground/attribute-only: no role ever emits a
// background or reverse-video code, in content or chrome.
func buildTable(caps Caps) map[frame.StyleID]string {
	if caps.Profile == ProfileMono {
		return nil
	}

	t := map[frame.StyleID]string{
		frame.StyleNone:      "",
		frame.StyleAssistant: "",
		frame.StyleShell:     "",
		frame.StyleUser:      attrBold,
		frame.StyleHeading:   attrBold,
		frame.StyleStrong:    attrBold,
		frame.StyleActive:    attrBold,
		frame.StyleEmphasis:  attrItalic,
		frame.StyleThought:   attrDim,
		frame.StyleMuted:     attrDim,
		frame.StyleSkipped:   attrDim,
	}

	var red, yellow, green, cyan, gray, code, brand string
	var ramp1, ramp2, ramp3 string
	switch caps.Profile {
	case ProfileANSI16:
		red, yellow, green, cyan, gray, code = fg16Red, fg16Yellow, fg16Green, fg16Cyan, fg16Gray, fg16Blue
		if caps.Dark {
			brand = fg16BrandBright
			ramp1, ramp2, ramp3 = fg16BrandBright, fg16BrandCore, fg16BrandBright
		} else {
			brand = fg16BrandCore
			ramp1, ramp2, ramp3 = fg16BrandCore, fg16BrandCore, fg16BrandBright
		}
	case ProfileANSI256:
		red, yellow, green, cyan, gray, code = fg256Red, fg256Yellow, fg256Green, fg256Cyan, fg256Gray, fg256Code
		if caps.Dark {
			brand = fg256BrandDark
			ramp1, ramp2, ramp3 = fg256Ramp1Dark, fg256Ramp2Dark, fg256Ramp3Dark
		} else {
			brand = fg256BrandLight
			ramp1, ramp2, ramp3 = fg256Ramp1Light, fg256Ramp2Light, fg256Ramp3Light
		}
	case ProfileTrueColor:
		red, yellow, green, cyan, gray, code = fgTCRed, fgTCYellow, fgTCGreen, fgTCCyan, fgTCGray, fgTCCode
		if caps.Dark {
			brand = fgTCBrandDark
			ramp1, ramp2, ramp3 = fgTCRamp1Dark, fgTCRamp2Dark, fgTCRamp3Dark
		} else {
			brand = fgTCBrandLight
			ramp1, ramp2, ramp3 = fgTCRamp1Light, fgTCRamp2Light, fgTCRamp3Light
		}
	}

	t[frame.StyleError] = red
	t[frame.StyleFailed] = red
	t[frame.StyleWarn] = yellow
	t[frame.StyleDone] = green
	t[frame.StylePending] = cyan
	t[frame.StyleHITL] = brand
	t[frame.StyleBorder] = gray
	t[frame.StyleInactive] = gray
	t[frame.StyleTool] = gray
	t[frame.StyleCode] = code
	t[frame.StyleBrand] = brand
	t[frame.StyleBrandRamp1] = ramp1
	t[frame.StyleBrandRamp2] = ramp2
	t[frame.StyleBrandRamp3] = ramp3

	return t
}
